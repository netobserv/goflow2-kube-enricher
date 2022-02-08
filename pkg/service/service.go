// Package service glues together all the application component and provides a unified
// build of the service, for both production and tests
package service

import (
	"context"
	"log"
	"os"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	fjson "github.com/netobserv/goflow2-kube-enricher/pkg/format/json"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	"github.com/netobserv/goflow2-kube-enricher/pkg/health"
	"github.com/netobserv/goflow2-kube-enricher/pkg/ingest/ipfix"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
	"github.com/netobserv/goflow2-kube-enricher/pkg/pipe"
	"github.com/netobserv/goflow2-kube-enricher/pkg/transform"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

var klog = logrus.WithField("module", "service/Service")

type Service struct {
	config    *config.Config
	pipe      *pipe.Pipeline
	health    *health.Reporter
	informers *meta.Informers
}

// Build a Service instance that interacts with the external components passed as arguments
func Build(cfg *config.Config,
	clientset kubernetes.Interface,
	loki *export.Loki,
	health *health.Reporter,
) *Service {
	// mount pipeline
	var in pipe.Ingester
	if cfg.Listen == "" {
		switch cfg.StdinFormat {
		case config.JSONFlagName:
			in = fjson.Ingester(os.Stdin)
		case config.PBFlagName:
			in = pb.Ingester(os.Stdin)
		default:
			klog.WithField("format", cfg.StdinFormat).Fatal("Unknown source format")
		}
	} else {
		in = ipfix.Ingester(cfg)
	}

	infrms := meta.NewInformers(clientset)
	enricher := transform.Enricher{
		Config:    cfg,
		Informers: &infrms,
		Health:    health,
	}
	return &Service{
		config:    cfg,
		health:    health,
		pipe:      pipe.New(in, loki.Submitter(), enricher.Enrich),
		informers: &infrms,
	}
}

func (r *Service) Start(ctx context.Context) {
	r.pipe.Start(ctx)
	r.startInformers(ctx)
	r.health.Status = health.Ready
}

func (r *Service) startInformers(ctx context.Context) {
	if err := r.informers.Start(ctx.Done()); err != nil {
		klog.WithError(err).Fatal("can't start informers")
	}
	klog.Info("waiting for informers to be synchronized")
	r.informers.WaitForCacheSync(ctx.Done())
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		r.informers.DebugInfo(log.Writer())
	}
}

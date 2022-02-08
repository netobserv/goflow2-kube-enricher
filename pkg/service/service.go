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
	config *config.Config
	pipe   *pipe.Pipeline
	health *health.Reporter
}

// Build a Service instance that interacts with the external components passed as arguments
func Build(cfg *config.Config,
	clientset kubernetes.Interface,
	loki *export.Loki,
	health *health.Reporter,
) *Service {
	// create and sync informers
	informers := meta.NewInformers(clientset)
	stopCh := make(chan struct{})
	defer close(stopCh)
	if err := informers.Start(stopCh); err != nil {
		klog.WithError(err).Fatal("can't start informers")
	}
	klog.Info("waiting for informers to be synchronized")
	informers.WaitForCacheSync(stopCh)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		informers.DebugInfo(log.Writer())
	}

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

	enricher := transform.Enricher{
		Config:    cfg,
		Informers: &informers,
		Health:    health,
	}
	return &Service{
		config: cfg,
		health: health,
		pipe:   pipe.New(in, loki.Submitter(), enricher.Enrich),
	}
}

func (r *Service) Start(ctx context.Context) {
	r.pipe.Start(ctx)
	r.health.Status = health.Ready
}

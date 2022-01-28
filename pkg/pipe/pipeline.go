// Package pipe groups the flow listening and enrichement functionalities into a single
// pipe
package pipe

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
)

const netflowScheme = "netflow"
const legacyScheme = "nfl"

type Status int

const (
	Stopped Status = iota
	Starting
	Started
)

func (s Status) String() string {
	switch s {
	case Stopped:
		return "Stopped"
	case Started:
		return "Started"
	case Starting:
		return "Starting"
	default:
		return "Unknown"
	}
}

type Pipeline struct {
	cfg       *config.Config
	status    Status
	informers *meta.Informers
	loki      *Loki
	enricher  KubeEnricher
}

func NewPipeline(cfg *config.Config, clientset kubernetes.Interface) (*Pipeline, error) {
	loki, err := NewLoki(&cfg.Loki)
	if err != nil {
		return nil, errors.Wrap(err, "can't create Loki exporter")
	}

	informers := meta.NewInformers(clientset)
	return &Pipeline{
		cfg:       cfg,
		status:    Stopped,
		informers: &informers,
		loki:      &loki,
		enricher: KubeEnricher{
			informers: &informers,
			config:    cfg,
		},
	}, nil
}

func (fe *Pipeline) Start(ctx context.Context) {
	fe.status = Starting

	ipfixMessages := StartListening(fe.cfg.Listen)
	fe.startInformers()
	// Starts informers to get updated information about kubernetes entities
	// Start format-enrich loop

	transformedFlows := Transform(ipfixMessages)
	enrichedFlows := fe.enricher.Enrich(transformedFlows)

	fe.status = Started
	for msg := range enrichedFlows {
		fe.loki.ProcessRecord(msg)
	}
}

func (fe *Pipeline) startInformers() {
	log := logrus.WithField("component", "Pipeline")
	log.Info("starting Kubernetes informers")
	stopCh := make(chan struct{}) // TODO: channel leak. Properly stop
	if err := fe.informers.Start(stopCh); err != nil {
		log.WithError(err).Fatal("can't start informers")
	}
	log.Info("waiting for informers to be synchronized")
	fe.informers.WaitForCacheSync(stopCh)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		fe.informers.DebugInfo(log.Writer())
	}
}

func (fe *Pipeline) Status() Status {
	return fe.status
}

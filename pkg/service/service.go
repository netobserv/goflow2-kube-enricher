// Package service groups the flow listening and enrichement functionalities into a single
// service
package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	jsonFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/json"
	nfFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/netflow"
	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
	reader "github.com/netobserv/goflow2-kube-enricher/pkg/reader"
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

type FlowEnricher struct {
	status    Status
	informers *meta.Informers
	reader    reader.Reader
	loki      *export.Loki
}

func NewFlowEnricher(cfg *config.Config, clientset kubernetes.Interface) (*FlowEnricher, error) {
	var in format.Format
	if cfg.Listen == "" {
		switch cfg.StdinFormat {
		case config.JSONFlagName:
			in = jsonFormat.NewScanner(os.Stdin)
		case config.PBFlagName:
			in = pbFormat.NewScanner(os.Stdin)
		default:
			return nil, fmt.Errorf("unknown source format: %s", cfg.StdinFormat)
		}
	} else {
		listenAddrURL, err := url.Parse(cfg.Listen)
		if err != nil {
			return nil, errors.Wrap(err, "can't parse the listen address URL")
		}
		if listenAddrURL.Scheme == netflowScheme || listenAddrURL.Scheme == legacyScheme {
			hostname := listenAddrURL.Hostname()
			port, err := strconv.ParseUint(listenAddrURL.Port(), 10, 64)
			if err != nil {
				return nil, errors.Wrap(err, "failed reading listening port")
			}
			in = nfFormat.NewDriver(hostname, int(port), listenAddrURL.Scheme == legacyScheme)
		} else {
			return nil, fmt.Errorf("unknown listening protocol %q", listenAddrURL.Scheme)
		}
	}

	loki, err := export.NewLoki(&cfg.Loki)
	if err != nil {
		return nil, errors.Wrap(err, "can't create Loki exporter")
	}

	informers := meta.NewInformers(clientset)
	flowReader, err := reader.NewReader(cfg, in, &informers)
	if err != nil {
		return nil, errors.Wrap(err, "can't instantiate reader instance")
	}
	return &FlowEnricher{
		status:    Stopped,
		informers: &informers,
		reader:    flowReader,
		loki:      &loki,
	}, nil
}

func (fe *FlowEnricher) Start(ctx context.Context) {
	fe.status = Starting

	// Starts informers to get updated information about kubernetes entities
	log := logrus.WithField("component", "reader.Reader")
	log.Info("starting Kubernetes informers")
	stopCh := make(chan struct{})
	defer close(stopCh)
	if err := fe.informers.Start(stopCh); err != nil {
		log.WithError(err).Fatal("can't start informers")
	}
	log.Info("waiting for informers to be synchronized")
	fe.informers.WaitForCacheSync(stopCh)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		fe.informers.DebugInfo(log.Writer())
	}
	// Start format-enrich loop
	fe.status = Started
	fe.reader.Start(ctx)
}

func (fe *FlowEnricher) Status() Status {
	return fe.status
}

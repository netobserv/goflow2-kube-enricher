// Package ipfix provides functionality for IPFIX ingestion
package ipfix

import (
	"context"
	"net/url"
	"strconv"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	nfFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/netflow"
	"github.com/netobserv/goflow2-kube-enricher/pkg/pipe"
	"github.com/sirupsen/logrus"
)

const (
	netflowScheme = "netflow"
	legacyScheme  = "nfl"
)

var ilog = logrus.WithField("module", "ipfix/Ingester")

// Ingester creates a new Goflow IPFIX ingester over UDP.
// TODO: move format/netflow contents to this package
// or remove and replace them by go-vmware/ipfix library
func Ingester(cfg *config.Config) pipe.Ingester {
	listenAddrURL, err := url.Parse(cfg.Listen)
	if err != nil {
		ilog.Fatal(err)
	}
	return func(ctx context.Context) <-chan flow.Record {
		if listenAddrURL.Scheme != netflowScheme && listenAddrURL.Scheme != legacyScheme {
			ilog.Fatalf("Unknown listening protocol")
		}
		hostname := listenAddrURL.Hostname()
		port, err := strconv.ParseUint(listenAddrURL.Port(), 10, 64)
		if err != nil {
			ilog.Fatal("Failed reading listening port: ", err)
		}
		ilog.Infof("Start listening on %s", cfg.Listen)

		return nfFormat.StartDriver(ctx, hostname, int(port),
			listenAddrURL.Scheme == legacyScheme)
	}
}

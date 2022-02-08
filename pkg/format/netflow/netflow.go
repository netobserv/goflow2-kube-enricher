// Package netflow implements the Format interface for raw netflow input (not reencoded)
package netflow

import (
	"context"
	"log"

	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	goflow2Format "github.com/netsampler/goflow2/format"

	// this blank import triggers pb registration via init
	_ "github.com/netsampler/goflow2/format/protobuf"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
)

const channelSize = 5

// StartDriver starts a new go routine to handle netflow connections
func StartDriver(ctx context.Context, hostname string, port int, legacy bool) <-chan flow.Record {
	out := make(chan flow.Record, channelSize)

	go func() {
		transporter := NewWrapper(out)

		formatter, err := goflow2Format.FindFormat(ctx, "pb")
		if err != nil {
			log.Fatal(err)
		}

		if legacy {
			sNFL := &utils.StateNFLegacy{
				Format:    formatter,
				Transport: transporter,
				Logger:    logrus.StandardLogger(),
			}
			err = sNFL.FlowRoutine(1, hostname, port, false)
			go func() {
				<-ctx.Done()
				sNFL.Shutdown()
				close(out)
			}()
		} else {
			sNF := &utils.StateNetFlow{
				Format:    formatter,
				Transport: transporter,
				Logger:    logrus.StandardLogger(),
			}
			err = sNF.FlowRoutine(1, hostname, port, false)
			go func() {
				<-ctx.Done()
				sNF.Shutdown()
				close(out)
			}()
		}
		log.Fatal(err)

	}()

	return out
}

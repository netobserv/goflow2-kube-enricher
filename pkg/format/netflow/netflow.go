// Package netflow implements the Format interface for raw netflow input (not reencoded)
package netflow

import (
	"context"
	"log"

	goflow2Format "github.com/netsampler/goflow2/format"
	// this blank import triggers pb registration via init
	_ "github.com/netsampler/goflow2/format/protobuf"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
)

const channelSize = 5

type Driver struct {
	in       chan map[string]interface{}
	legacy   bool
	hostname string
	port     int
	sdn      func()
}

func NewDriver(hostname string, port int, legacy bool) *Driver {
	return &Driver{
		legacy:   legacy,
		hostname: hostname,
		port:     port,
		in:       make(chan map[string]interface{}, channelSize),
	}
}

// Start flow listening
func (gf *Driver) Start(ctx context.Context) {
	logrus.WithFields(logrus.Fields{
		"component": "netflow.Driver",
		"port":      gf.port,
	}).Infof("Start listening for new flows")

	transporter := NewWrapper(gf.in)

	formatter, err := goflow2Format.FindFormat(ctx, "pb")
	if err != nil {
		log.Fatal(err)
	}

	if gf.legacy {
		sNFL := &utils.StateNFLegacy{
			Format:    formatter,
			Transport: transporter,
			Logger:    logrus.StandardLogger(),
		}
		err = sNFL.FlowRoutine(1, gf.hostname, gf.port, false)
		gf.sdn = sNFL.Shutdown
	} else {
		sNF := &utils.StateNetFlow{
			Format:    formatter,
			Transport: transporter,
			Logger:    logrus.StandardLogger(),
		}
		err = sNF.FlowRoutine(1, gf.hostname, gf.port, false)
		gf.sdn = sNF.Shutdown
	}
	log.Fatal(err)
}

func (gf *Driver) Next() (map[string]interface{}, error) {
	msg := <-gf.in
	return msg, nil
}

func (gf *Driver) Shutdown() {
	gf.sdn()
}

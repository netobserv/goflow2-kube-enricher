package netflow

import (
	"context"
	"log"

	goflow2Format "github.com/netsampler/goflow2/format"
	_ "github.com/netsampler/goflow2/format/protobuf"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
)

const channelSize = 5

type Driver struct {
	in chan map[string]interface{}
}

// Start a new go routine to handle netflow connections
func StartDriver(ctx context.Context, hostname string, port int) *Driver {
	gf := Driver{}
	gf.in = make(chan map[string]interface{}, channelSize)

	go func() {
		transporter := NewWrapper(gf.in)

		formatter, err := goflow2Format.FindFormat(ctx, "pb")
		if err != nil {
			log.Fatal(err)
		}

		sNF := &utils.StateNetFlow{
			Format:    formatter,
			Transport: transporter,
			Logger:    logrus.StandardLogger(),
		}
		err = sNF.FlowRoutine(1, hostname, port, false)
		log.Fatal(err)

	}()

	return &gf
}

func (gf *Driver) Next() (map[string]interface{}, error) {
	msg := <-gf.in
	return msg, nil
}

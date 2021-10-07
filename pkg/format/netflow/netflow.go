package netflow

import (
	"context"
	"log"

	goflow2Format "github.com/netsampler/goflow2/format"
	_ "github.com/netsampler/goflow2/format/protobuf"
	"github.com/netsampler/goflow2/utils"
	"github.com/sirupsen/logrus"
)

type GoflowDriver struct {
	in chan map[string]interface{}
}

func NewDriver(hostname string, port int) *GoflowDriver {
	gf := GoflowDriver{}
	gf.in = make(chan map[string]interface{})

	go func(in chan map[string]interface{}) {
		ctx := context.Background()

		transporter := NewWrapper(in)

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

	}(gf.in)

	return &gf
}

func (gf *GoflowDriver) Next() (map[string]interface{}, error) {
	msg := <-gf.in
	return msg, nil
}

package pipe

import (
	"github.com/vmware/go-ipfix/pkg/collector"
	"github.com/vmware/go-ipfix/pkg/entities"
)

func panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func StartListening(address string) <-chan *entities.Message {
	collect, err := collector.InitCollectingProcess(collector.CollectorInput{
		Address:       address,
		Protocol:      "udp",
		MaxBufferSize: 65535,
		TemplateTTL:   0,
	})
	panicOnErr(err)

	go collect.Start()
	out := make(chan *entities.Message)
	go func() {
		for msg := range collect.GetMsgChan() {
			// discard templates
			if msg.GetSet().GetSetType() == entities.Data {
				out <- msg
			}
		}
	}()
	return out
}

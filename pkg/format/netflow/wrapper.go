package netflow

import (
	"github.com/golang/protobuf/proto"
	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	goflowpb "github.com/netsampler/goflow2/pb"
)

// This is en implementation of the goflow2 transport interface
type TransportWrapper struct {
	c chan map[string]interface{}
}

func NewWrapper(c chan map[string]interface{}) *TransportWrapper {
	tw := TransportWrapper{c: c}
	return &tw
}

func (w *TransportWrapper) Send(key, data []byte) error {
	message := goflowpb.FlowMessage{}
	err := proto.Unmarshal(data, &message)
	if err != nil {
		return err
	}
	renderedMsg, err := pbFormat.RenderMessage(message)
	if err == nil {
		w.c <- renderedMsg
	}
	return err
}

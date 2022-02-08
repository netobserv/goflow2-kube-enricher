package netflow

import (
	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	goflowpb "github.com/netsampler/goflow2/pb"
	"google.golang.org/protobuf/proto"

	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
)

// TransportWrapper is an implementation of the goflow2 transport interface
type TransportWrapper struct {
	c chan flow.Record
}

func NewWrapper(c chan flow.Record) *TransportWrapper {
	tw := TransportWrapper{c: c}
	return &tw
}

func (w *TransportWrapper) Send(_, data []byte) error {
	message := goflowpb.FlowMessage{}
	err := proto.Unmarshal(data, &message)
	if err != nil {
		return err
	}
	renderedMsg, err := pbFormat.RenderMessage(&message)
	if err == nil {
		w.c <- renderedMsg
	}
	return err
}

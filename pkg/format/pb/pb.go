// Package pb defines a Protobuf Format, implementing Format interface for input
package pb

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"

	ms "github.com/mitchellh/mapstructure"
	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	"github.com/netobserv/goflow2-kube-enricher/pkg/pipe"
	goflowFormat "github.com/netsampler/goflow2/format/common"
	goflowpb "github.com/netsampler/goflow2/pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var plog = logrus.WithField("module", "pb/Ingester")

// Ingester returns a pipe.Ingester instance that parses the protobuf messages from the
// input stream and forwards them as flow.Record instances to an output channel
func Ingester(in io.Reader) pipe.Ingester {
	return func(ctx context.Context) <-chan flow.Record {
		out := make(chan flow.Record, pipe.ChannelsBuffer)
		go scanMessages(ctx, in, out)
		return out
	}
}

func scanMessages(ctx context.Context, in io.Reader, out chan<- flow.Record) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if record, err := next(in); err != nil {
				if errors.Is(err, io.EOF) {
					plog.Info("reached end of input. Stopping")
					return
				}
				plog.WithError(err).Warn("can't read record")
			} else {
				out <- record
			}
		}
	}
}

func next(in io.Reader) (flow.Record, error) {
	lenBuf := make([]byte, binary.MaxVarintLen64)

	// Message is prefixed by its length, we read that length
	_, err := io.ReadAtLeast(in, lenBuf, binary.MaxVarintLen64)
	if err != nil {
		return nil, err
	}
	len, lenSize := protowire.ConsumeVarint(lenBuf)
	if lenSize < 0 {
		return nil, errors.New("protobuf: Could not parse message length")
	}

	// Now we can allocate the message buffer with the appropriate size
	msgBuf := make([]byte, len)
	if lenSize < binary.MaxVarintLen64 {
		// If we read too much, we copy remaining bytes to the message buffer
		copy(msgBuf[0:binary.MaxVarintLen64-lenSize], lenBuf[lenSize:binary.MaxVarintLen64])
	}
	_, err = io.ReadFull(in, msgBuf[binary.MaxVarintLen64-lenSize:])
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	msgBuf = bytes.TrimSuffix(msgBuf, []byte("\n"))

	message := goflowpb.FlowMessage{}
	err = proto.Unmarshal(msgBuf, &message)
	if err != nil {
		return nil, err
	}
	return RenderMessage(&message)
}

func RenderMessage(message *goflowpb.FlowMessage) (map[string]interface{}, error) {
	outputMap := make(map[string]interface{})
	err := ms.Decode(message, &outputMap)
	if err != nil {
		return nil, err
	}
	outputMap["DstAddr"] = goflowFormat.RenderIP(message.DstAddr)
	outputMap["SrcAddr"] = goflowFormat.RenderIP(message.SrcAddr)
	outputMap["DstMac"] = renderMac(message.DstMac)
	outputMap["SrcMac"] = renderMac(message.SrcMac)
	return outputMap, nil
}

func renderMac(macValue uint64) string {
	mac := make([]byte, 8)
	binary.BigEndian.PutUint64(mac, macValue)
	return net.HardwareAddr(mac[2:]).String()
}

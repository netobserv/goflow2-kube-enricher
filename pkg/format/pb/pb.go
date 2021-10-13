package pb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"

	ms "github.com/mitchellh/mapstructure"
	goflowFormat "github.com/netsampler/goflow2/format/common"
	goflowpb "github.com/netsampler/goflow2/pb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type PbFormat struct {
	in io.Reader
}

func NewScanner(in io.Reader) *PbFormat {
	input := PbFormat{}
	input.in = in
	return &input
}

func (pbFormat *PbFormat) Next() (map[string]interface{}, error) {
	lenBuf := make([]byte, binary.MaxVarintLen64)

	// Message is prefixed by its length, we read that length
	_, err := io.ReadAtLeast(pbFormat.in, lenBuf, binary.MaxVarintLen64)
	if err != nil && err != io.EOF {
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
	_, err = io.ReadFull(pbFormat.in, msgBuf[binary.MaxVarintLen64-lenSize:])
	if err != nil && err != io.EOF {
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

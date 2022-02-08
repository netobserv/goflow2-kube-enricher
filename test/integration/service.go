// Package integration contains tools and tests for integration testing
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang/snappy"
	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/health"
	"github.com/netobserv/goflow2-kube-enricher/pkg/service"
	"github.com/netobserv/loki-client-go/pkg/logproto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vmware/go-ipfix/pkg/entities"
	"k8s.io/client-go/kubernetes"
)

const (
	templateID          = 256
	observationDomainID = 1
	startTimeout        = 10 * time.Second
)

// Values taken from https://www.iana.org/assignments/ipfix/ipfix.xhtml
var (
	sourceIPv4Address = entities.NewInfoElement("sourceIPv4Address", 8, entities.Ipv4Address, 0, 4)
	flowStartSeconds  = entities.NewInfoElement("flowStartSeconds", 150, entities.DateTimeSeconds, 0, 4)
	flowEndSeconds    = entities.NewInfoElement("flowEndSeconds", 151, entities.DateTimeSeconds, 0, 4)
)

// TestKubeEnricher setup that provides an extra layer to submit test flows and check what
// has been submitted to loki. To reproduce all the steps of a usual Kube Enricher instance,
// this test instance also does all the encoding/decoding steps, like IPFIX for the input data
// and Protocol Buffers for the output data.
type TestKubeEnricher struct {
	// Port (UDP) where the kube enricher listens to
	Port int
	// LokiFlows is a buffered channel containing maps with the Flow's data as received by Loki
	LokiFlows <-chan map[string]interface{}
	ctxCancel func()
	fakeLoki  *httptest.Server
	conn      net.Conn
	clock     func() time.Time
}

// StartKubeEnricher given a kubernetes.Interface implementation (usually a fake.NewSimpleClientset)
// and a mocked clock
func StartKubeEnricher(clientset kubernetes.Interface, clock func() time.Time) (TestKubeEnricher, error) {
	log.SetLevel(log.DebugLevel)
	startTime := time.Now()
	listenPort, err := freeUDPPort()
	if err != nil {
		return TestKubeEnricher{}, errors.Wrap(err, "opening free UDP port")
	}
	log.WithField("port", listenPort).Info("Starting integration tests")
	lokiFlows := make(chan map[string]interface{}, 256)
	fakeLoki := httptest.NewServer(lokiHandler(lokiFlows))

	cfg := &config.Config{
		PrintInput:  true,
		PrintOutput: true,
		Listen:      fmt.Sprintf("netflow://0.0.0.0:%d", listenPort),
		IPFields: map[string]string{
			"SrcAddr": "",
		},
		Loki: config.LokiConfig{
			URL:            fakeLoki.URL,
			TenantID:       "foo",
			BatchSize:      1024,
			BatchWait:      time.Second,
			Timeout:        time.Second,
			TimestampScale: time.Second,
		},
	}

	loki, err := export.NewLoki(&cfg.Loki)
	if err != nil {
		fakeLoki.Close()
		return TestKubeEnricher{}, errors.Wrap(err, "creating loki exporter")
	}
	healthReport := health.NewReporter(health.Starting)
	svc := service.Build(cfg, clientset, &loki, healthReport)
	ctx, ctxCancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// wait until the service has started
	for healthReport.Status != health.Ready {
		if time.Since(startTime) > startTimeout {
			log.Fatal("timeout while waiting for TestKubeEnricher to start")
		}
	}

	conn, err := net.Dial("udp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		ctxCancel()
		fakeLoki.Close()
		return TestKubeEnricher{}, errors.Wrapf(err, "can't open UDP connection :%d", listenPort)
	}
	return TestKubeEnricher{
		Port:      listenPort,
		LokiFlows: lokiFlows,
		ctxCancel: ctxCancel,
		conn:      conn,
		fakeLoki:  fakeLoki,
		clock:     clock,
	}, nil
}

// Close the resources associated to this running instance
func (ke *TestKubeEnricher) Close() {
	ke.ctxCancel()
	ke.fakeLoki.Close()
	ke.conn.Close()
}

// SendTemplate must be executed before sending any flow
func (ke *TestKubeEnricher) SendTemplate() error {
	// TODO: add more fields
	templateElements := []entities.InfoElementWithValue{
		entities.NewIPAddressInfoElement(sourceIPv4Address, nil),
		entities.NewDateTimeSecondsInfoElement(flowStartSeconds, 0),
		entities.NewDateTimeSecondsInfoElement(flowEndSeconds, 0),
	}
	set := entities.NewSet(false)
	if err := set.PrepareSet(entities.Template, templateID); err != nil {
		return err
	}
	if err := set.AddRecord(templateElements, templateID); err != nil {
		return nil
	}
	set.UpdateLenInHeader()
	return ke.sendMessage(set)
}

// SendFlow containing the information passed as an argument
func (ke *TestKubeEnricher) SendFlow(srcIP string) error {
	now := ke.clock().Unix()
	// TODO: add more fields
	templateElements := []entities.InfoElementWithValue{
		entities.NewIPAddressInfoElement(sourceIPv4Address, net.ParseIP(srcIP)),
		entities.NewDateTimeSecondsInfoElement(flowStartSeconds, uint32(now)),
		entities.NewDateTimeSecondsInfoElement(flowEndSeconds, uint32(now)),
	}
	set := entities.NewSet(false)
	if err := set.PrepareSet(entities.Data, templateID); err != nil {
		return err
	}
	if err := set.AddRecord(templateElements, templateID); err != nil {
		return nil
	}
	set.UpdateLenInHeader()
	return ke.sendMessage(set)
}

func (ke *TestKubeEnricher) sendMessage(set entities.Set) error {
	msg := entities.NewMessage(false)
	msg.SetVersion(10)
	msg.AddSet(set)
	msgLen := entities.MsgHeaderLength + set.GetSetLength()
	msg.SetVersion(10)
	msg.SetObsDomainID(observationDomainID)
	msg.SetMessageLen(uint16(msgLen))
	msg.SetExportTime(uint32(time.Now().Unix()))
	msg.SetSequenceNum(0)
	bytesSlice := make([]byte, msgLen)
	copy(bytesSlice[:entities.MsgHeaderLength], msg.GetMsgHeader())
	copy(bytesSlice[entities.MsgHeaderLength:entities.MsgHeaderLength+entities.SetHeaderLen], set.GetHeaderBuffer())
	index := entities.MsgHeaderLength + entities.SetHeaderLen
	for _, record := range set.GetRecords() {
		len := record.GetRecordLength()
		copy(bytesSlice[index:index+len], record.GetBuffer())
		index += len
	}
	_, err := ke.conn.Write(bytesSlice)
	return err
}

// lokiHandler is a fake loki HTTP service that decodes the snappy/protobuf messages
// and forwards them for later assertions
func lokiHandler(flowsData chan<- map[string]interface{}) http.HandlerFunc {
	hlog := log.WithField("component", "LokiHandler")
	return func(rw http.ResponseWriter, req *http.Request) {
		hlog.WithFields(log.Fields{
			"method": req.Method,
			"url":    req.URL,
			"header": req.Header,
		}).Info("new request")
		if req.Method != http.MethodPost && req.Method != http.MethodPut {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			hlog.WithError(err).Error("can't read request body")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		decodedBody, err := snappy.Decode([]byte{}, body)
		if err != nil {
			hlog.WithError(err).Error("can't decode snappy body")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		pr := logproto.PushRequest{}
		if err := pr.Unmarshal(decodedBody); err != nil {
			hlog.WithError(err).Error("can't decode protobuf body")
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, stream := range pr.Streams {
			for _, entry := range stream.Entries {
				flowData := map[string]interface{}{}
				if err := json.Unmarshal([]byte(entry.Line), &flowData); err != nil {
					hlog.WithError(err).Error("expecting JSON line")
					rw.WriteHeader(http.StatusBadRequest)
					return
				}
				// TODO: decorate the flow map with extra metadata from the stream entry
				flowsData <- flowData
			}
		}
		rw.WriteHeader(http.StatusOK)
	}
}

// freeUDPPort asks the kernel for a free open port that is ready to use.
func freeUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port, nil
}

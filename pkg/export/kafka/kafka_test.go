package kafka

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

var now = time.Now()

type fakeEmitter struct {
	mock.Mock
}

func (f *fakeEmitter) SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	a := f.Mock.Called(msg.Timestamp, msg.Topic, msg.Key, msg.Value)
	return 0, 0, a.Error(0)
}

func TestKafla_BuildClientConfig(t *testing.T) {
	// GIVEN a file that overrides some fields in the default configuration
	cfgFile, err := ioutil.TempFile("", "testload_")
	require.NoError(t, err)
	_, err = cfgFile.WriteString(`
kafka:
  version: 3.0.0
  export:
    brokers:
      - kafka-broker:9092
    maxMsgBytes: 1024
    flushBytes: 2048
    flushFrequency: 3000000000
printInput: true
`)
	require.NoError(t, err)
	require.NoError(t, cfgFile.Close())

	// WHEN it is loaded and transformed into a Kafka Config
	cfg, err := config.Load(cfgFile.Name())
	require.NoError(t, err)

	// THEN the generated kafka config is consistent with the parsed data
	scfg, err := buildKafkaConfig(cfg.Kafka)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", scfg.Version.String())
	assert.Equal(t, 1024, scfg.Producer.MaxMessageBytes)
	assert.Equal(t, 2048, scfg.Producer.Flush.Bytes)
	assert.Equal(t, time.Duration(3000000000), scfg.Producer.Flush.Frequency)
}

func TestKafka_ProcessRecord(t *testing.T) {
	// GIVEN a Loki exporter
	fe := fakeEmitter{}
	fe.On("SendMessage", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	cfg, err := config.Read(strings.NewReader(`
kafka:
  version: 3.0.0
  export:
    brokers:
      - kafka-broker:9092
    hashKeys:
      - ts
      - foo
      - bar
printInput: true
printOutput: true
`))
	require.NoError(t, err)
	kafka, err := NewKafka(cfg.Kafka)
	require.NoError(t, err)
	kafka.emitter = &fe

	// WHEN it processes input records
	record := map[string]interface{}{
		"ts": 123456, "ignored": "ignored!", "foo": "fooValue", "bar": "barValue", "value": 1234}
	js, err := json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, kafka.ProcessRecord(now, record))

	// THEN it forwards the records sending encoded timestamp as key and record as json value
	fe.AssertCalled(t, "SendMessage",
		now,
		`goflow-kube`,
		sarama.StringEncoder(fmt.Sprintf("%v-%v-%v", 123456, "fooValue", "barValue")),
		sarama.ByteEncoder(js))
}

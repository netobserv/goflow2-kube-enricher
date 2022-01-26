package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

var now = time.Now()

type fakeEmitter struct {
	mock.Mock
}

func (f *fakeEmitter) WriteMessages(ctx context.Context, msgs ...kafkago.Message) error {
	a := f.Mock.Called(string(msgs[0].Key), string(msgs[0].Value))
	return a.Error(0)
}
func TestKafla_BuildClientConfig(t *testing.T) {
	// GIVEN a file that overrides some fields in the default configuration
	cfgFile, err := ioutil.TempFile("", "testload_")
	require.NoError(t, err)
	_, err = cfgFile.WriteString(`
kafka:
  topic: testTopic
  writer:
    brokers:
      - kafka-broker:9092
    balancer: hash
printInput: true
`)
	require.NoError(t, err)
	require.NoError(t, cfgFile.Close())

	// WHEN it is loaded and transformed into a Kafka Config
	cfg, err := config.Load(cfgFile.Name())
	require.NoError(t, err)

	// THEN the generated kafka config is consistent with the parsed data
	wc, err := buildKafkaConfig(cfg.Kafka)
	require.NoError(t, err)
	assert.Equal(t, []string{"kafka-broker:9092"}, wc.Brokers)
	assert.Equal(t, "testTopic", wc.Topic)
	assert.Equal(t, &kafkago.Hash{}, wc.Balancer)
}

func TestKafka_ProcessRecord(t *testing.T) {
	// GIVEN a Loki exporter
	fe := fakeEmitter{}
	fe.On("WriteMessages", mock.Anything, mock.Anything).Return(nil)
	cfg, err := config.Read(strings.NewReader(`
kafka:
  topic: testTopic
  writer:
    brokers:
      - kafka-broker:9092
    balancer: hash
    hashKeys:
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
	fe.AssertCalled(t, "WriteMessages",
		fmt.Sprintf("%v-%v", "fooValue", "barValue"),
		string(js))
}

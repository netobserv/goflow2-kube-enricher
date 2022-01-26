// Package kafka enables data exporting to Kafka
package kafka

import (
	"fmt"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	jsoniter "github.com/json-iterator/go"
	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithField("module", "export/kafka")
)

// Emitter abstracts the Sarama Producer
type emitter interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}

// Kafka record exporter
type Kafka struct {
	config      config.KafkaConfig
	kafkaConfig sarama.Config
	emitter     emitter
	timeNow     func() time.Time
}

// NewKafka creates a Kafka flow exporter from a given configuration
func NewKafka(cfg *config.KafkaConfig) (*Kafka, error) {
	kcfg, err := buildKafkaConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Kafka{
		config:      *cfg,
		kafkaConfig: *kcfg,
		emitter:     nil,
		timeNow:     time.Now,
	}, nil
}

func buildKafkaConfig(cfg *config.KafkaConfig) (*sarama.Config, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka config is not valid: %w", err)
	}
	if err := cfg.Export.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka export config is not valid: %w", err)
	}

	sConfig := config.NewSaramaClientConfig(cfg)
	sConfig.Producer.MaxMessageBytes = cfg.Export.MaxMsgBytes
	sConfig.Producer.Flush.Bytes = cfg.Export.FlushBytes
	sConfig.Producer.Flush.Frequency = cfg.Export.FlushFrequency
	sConfig.Producer.RequiredAcks = sarama.WaitForAll
	sConfig.Producer.Return.Successes = true
	sConfig.Producer.Return.Errors = true
	return sConfig, nil
}

func (k *Kafka) ProcessRecord(timestamp time.Time, record map[string]interface{}) error {
	js, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(record)
	if err != nil {
		return err
	}

	if k.emitter == nil {
		p, err := sarama.NewSyncProducer(k.config.Export.Brokers, &k.kafkaConfig)
		if err != nil {
			return err
		}
		k.emitter = p
	}

	key := hashFields(k.config.Export.Keys, record, timestamp)
	var message *sarama.ProducerMessage
	if key != nil {
		message = &sarama.ProducerMessage{
			Topic:     k.config.Topic,
			Timestamp: timestamp,
			Key:       sarama.StringEncoder(*key),
			Value:     sarama.ByteEncoder(js),
		}
		log.Debugf("Kafka export message topic %s key %s", message.Topic, *key)
	} else {
		message = &sarama.ProducerMessage{
			Topic:     k.config.Topic,
			Timestamp: timestamp,
			Value:     sarama.ByteEncoder(js),
		}
		log.Debugf("Kafka export message topic %s with nil key", message.Topic)
	}

	p, o, err := k.emitter.SendMessage(message)
	log.Debugf("Kafka message sent on partition %d offset %d", p, o)

	return err
}

// Create Key string from selected fields. Else return nill for random partition
func hashFields(fields []string, record map[string]interface{}, timestamp time.Time) *string {
	var v []string

	for _, field := range fields {
		value, ok := record[field]
		if ok {
			v = append(v, fmt.Sprintf("%v", value))
		}
	}

	if len(v) > 0 {
		result := strings.Join(v, "-")
		return &result
	}
	return nil
}

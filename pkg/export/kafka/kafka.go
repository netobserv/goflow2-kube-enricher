// Package kafka enables data exporting to Kafka
package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithField("module", "export/kafka")
)

// Emitter abstracts the kafkago Writer
type emitter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
}

// Kafka record exporter
type Kafka struct {
	config  config.KafkaConfig
	emitter emitter
	timeNow func() time.Time
}

func buildKafkaConfig(cfg *config.KafkaConfig) (*kafkago.WriterConfig, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka config is not valid: %w", err)
	}
	if err := cfg.Writer.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka export config is not valid: %w", err)
	}

	var b kafkago.Balancer
	switch cfg.Writer.Balancer {
	case "roundRobin":
		b = &kafkago.RoundRobin{}
	case "leastBytes":
		b = &kafkago.LeastBytes{}
	case "hash":
		b = &kafkago.Hash{}
	case "crc32":
		b = &kafkago.CRC32Balancer{}
	case "murmur2":
		b = &kafkago.Murmur2Balancer{}
	default:
		return nil, fmt.Errorf("the provided kafka balancer is not valid: %s", cfg.Writer.Balancer)
	}

	return &kafkago.WriterConfig{
		Dialer:       config.NewKafkaDialer(cfg),
		Brokers:      cfg.Writer.Brokers,
		Topic:        cfg.Topic,
		Balancer:     b,
		MaxAttempts:  cfg.Writer.MaxAttempts,
		BatchSize:    cfg.Writer.MaxBatchSize,
		BatchBytes:   cfg.Writer.MaxBatchBytes,
		BatchTimeout: cfg.Writer.BatchTimeout,
		ReadTimeout:  cfg.Writer.ReadTimeout,
		WriteTimeout: cfg.Writer.WriteTimeout,
	}, nil
}

// NewKafka creates a Kafka flow exporter from a given configuration
func NewKafka(cfg *config.KafkaConfig) (*Kafka, error) {
	wc, err := buildKafkaConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Kafka{
		config: *cfg,
		//kafkaConfig: *kcfg,
		emitter: kafkago.NewWriter(*wc),
		timeNow: time.Now,
	}, nil
}

func (k *Kafka) ProcessRecord(timestamp time.Time, record map[string]interface{}) error {
	js, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(record)
	if err != nil {
		return err
	}
	key := hashFields(k.config.Writer.Keys, record, timestamp)
	err = k.emitter.WriteMessages(context.Background(),
		kafkago.Message{
			Key:   []byte(*key),
			Value: js,
		},
	)
	log.Debugf("Kafka WriteMessages topic %s key %s", k.config.Topic, *key)
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

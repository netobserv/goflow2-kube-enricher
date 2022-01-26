package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type KafkaConfig struct {
	TLS     bool          `yaml:"tls"`
	Timeout time.Duration `yaml:"timeout"`
	Topic   string        `yaml:"topic"`
	Reader  *KafkaReader  `yaml:"reader"`
	Writer  *KafkaWriter  `yaml:"writer"`
}
type KafkaReader struct {
	GroupID                string        `yaml:"groupID"`
	QueueCapacity          int           `yaml:"queueCapacity"`
	MinBytes               int           `yaml:"minBytes"`
	MaxBytes               int           `yaml:"MaxBytes"`
	MaxWait                time.Duration `yaml:"maxWait"`
	ReadLagInterval        time.Duration `yaml:"readLagInterval"`
	GroupBalancers         []string      `yaml:"groupBalancers"`
	HeartbeatInterval      time.Duration `yaml:"heartbeatInterval"`
	CommitInterval         time.Duration `yaml:"commitInterval"`
	PartitionWatchInterval time.Duration `yaml:"partitionWatchInterval"`
	WatchPartitionChanges  bool          `yaml:"WatchPartitionChanges"`
	SessionTimeout         time.Duration `yaml:"sessionTimeout"`
	RebalanceTimeout       time.Duration `yaml:"rebalanceTimeout"`
	JoinGroupBackoff       time.Duration `yaml:"joinGroupBackoff"`
	RetentionTime          time.Duration `yaml:"retentionTime"`
	StartFirstOffset       bool          `yaml:"startFirstOffset"`
	ReadBackoffMin         time.Duration `yaml:"readBackoffMin"`
	ReadBackoffMax         time.Duration `yaml:"readBackoffMax"`
}

type KafkaWriter struct {
	Keys          []string      `yaml:"hashKeys"`
	Brokers       []string      `yaml:"brokers"`
	Balancer      string        `yaml:"balancer"`
	MaxAttempts   int           `yaml:"maxAttempts"`
	MaxBatchSize  int           `yaml:"maxBatchSize"`
	MaxBatchBytes int           `yaml:"maxBatchBytes"`
	BatchTimeout  time.Duration `yaml:"batchTimeout"`
	ReadTimeout   time.Duration `yaml:"readTimeout"`
	WriteTimeout  time.Duration `yaml:"writeTimeout"`
}

func NewKafkaDialer(cfg *KafkaConfig) *kafkago.Dialer {
	var tc *tls.Config
	if cfg.TLS {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			log.Fatalf(fmt.Sprintf("Error initializing TLS: %v", err))
		}
		tc = &tls.Config{
			RootCAs: rootCAs,
		}
	}
	return &kafkago.Dialer{
		Timeout:   cfg.Timeout,
		DualStack: true,
		TLS:       tc,
	}
}

func (c *KafkaConfig) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.Topic == "" {
		return errors.New("topic can't be empty")
	}
	return nil
}

func (c *KafkaReader) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.GroupID == "" {
		return errors.New("kafka group consumer can't be empty")
	}
	return nil
}

func (c *KafkaWriter) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.Brokers == nil || len(c.Brokers) == 0 {
		return errors.New("you must provide kafka brokers")
	}
	if c.Balancer == "" {
		return errors.New("you must provide kafka balancer")
	}
	return nil
}

func DefaultKafka() *KafkaConfig {
	return &KafkaConfig{
		TLS:    false,
		Topic:  "goflow-kube",
		Reader: DefaultKafkaReader(),
		Writer: DefaultKafkaWriter(),
	}
}

func DefaultKafkaReader() *KafkaReader {
	return &KafkaReader{
		GroupID: "goflow-kube",
	}
}

func DefaultKafkaWriter() *KafkaWriter {
	return &KafkaWriter{
		Keys:     []string{},
		Brokers:  []string{},
		Balancer: "roundRobin",
	}
}

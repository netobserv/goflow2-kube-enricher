package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Shopify/sarama"
)

type KafkaConfig struct {
	Version  string        `yaml:"version"`
	ClientID string        `yaml:"clientID"`
	TLS      bool          `yaml:"tls"`
	SASL     *KafkaSASL    `yaml:"sasl"`
	Topic    string        `yaml:"topic"`
	Consume  *KafkaConsume `yaml:"consume"`
	Export   *KafkaExport  `yaml:"export"`
}

type KafkaSASL struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type KafkaConsume struct {
	Group               string        `yaml:"group"`
	BalanceStrategy     string        `yaml:"balanceStrategy"`
	InitialOffsetOldest bool          `yaml:"initialOffsetOldest"`
	Backoff             time.Duration `yaml:"backoff"`
	MaxWaitTime         time.Duration `yaml:"maxWaitTime"`
	MaxProcessingTime   time.Duration `yaml:"maxProcessingTime"`
}

type KafkaExport struct {
	Keys           []string      `yaml:"hashKeys"`
	Brokers        []string      `yaml:"brokers"`
	MaxMsgBytes    int           `yaml:"maxMsgBytes"`
	FlushBytes     int           `yaml:"flushBytes"`
	FlushFrequency time.Duration `yaml:"flushFrequency"`
}

func NewSaramaClientConfig(cfg *KafkaConfig) *sarama.Config {
	sConfig := sarama.NewConfig()
	sConfig.ClientID = cfg.ClientID
	kafkaConfigVersion, err := sarama.ParseKafkaVersion(cfg.Version)
	if err != nil {
		log.Fatal(err)
	}
	sConfig.Version = kafkaConfigVersion
	if cfg.TLS {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			log.Fatalf(fmt.Sprintf("Error initializing TLS: %v", err))
		}
		sConfig.Net.TLS.Enable = true
		sConfig.Net.TLS.Config = &tls.Config{RootCAs: rootCAs}
	}
	if cfg.SASL != nil {
		if !cfg.TLS {
			log.Println("Warning: Using SASL without TLS will transmit the authentication in plaintext")
		}
		sConfig.Net.SASL.Enable = true
		sConfig.Net.SASL.User = cfg.SASL.User
		sConfig.Net.SASL.Password = cfg.SASL.Password
		log.Printf("Authenticating as user %s", sConfig.Net.SASL.User)
	}

	return sConfig
}

func (c *KafkaConfig) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.Topic == "" {
		return errors.New("topic can't be empty")
	}
	if c.SASL != nil && (c.SASL.User == "" || c.SASL.Password == "") {
		return errors.New("using sasl require user and password to be set")
	}
	return nil
}

func (c *KafkaConsume) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.Group == "" {
		return errors.New("kafka group consumer can't be empty")
	}
	return nil
}

func (c *KafkaExport) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.Brokers == nil || len(c.Brokers) == 0 {
		return errors.New("you must provide kafka brokers")
	}
	return nil
}

func DefaultKafka() *KafkaConfig {
	return &KafkaConfig{
		Version:  "2.8.0",
		ClientID: "goflow-kube",
		TLS:      false,
		SASL:     nil,
		Topic:    "goflow-kube",
		Consume:  DefaultKafkaConsume(),
		Export:   DefaultKafkaExport(),
	}
}

func DefaultKafkaConsume() *KafkaConsume {
	return &KafkaConsume{
		Group:               "goflow-kube",
		BalanceStrategy:     "range",
		InitialOffsetOldest: true,
		Backoff:             2 * time.Second,
		MaxWaitTime:         5 * time.Second,
		MaxProcessingTime:   1 * time.Second,
	}
}

func DefaultKafkaExport() *KafkaExport {
	return &KafkaExport{
		Keys:           []string{},
		Brokers:        []string{},
		MaxMsgBytes:    1024 * 1024,      // 1mb
		FlushBytes:     10 * 1024 * 1024, // 10 mb
		FlushFrequency: 5 * time.Second,
	}
}

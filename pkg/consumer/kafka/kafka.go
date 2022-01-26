// Package kafka implements the consumer for kafka
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	sarama "github.com/Shopify/sarama"
	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/enricher"
	"github.com/sirupsen/logrus"
)

type Consumer struct {
	log           *logrus.Entry
	enricher      enricher.Enricher
	ready         chan bool
	topic         string
	consumerGroup sarama.ConsumerGroup
}

// NewKafkaConsumer create kafka consumer
func NewKafkaConsumer(hostname string, port uint64, log *logrus.Entry, cfg *config.KafkaConfig, enricher enricher.Enricher) *Consumer {
	sConfig, err := buildKafkaConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}

	/**
	 * Setup a new Sarama consumer group
	 */
	cs := Consumer{}
	cs.log = log
	cs.enricher = enricher
	cs.ready = make(chan bool)
	cs.topic = cfg.Topic
	consumerGroup, err := sarama.NewConsumerGroup([]string{fmt.Sprintf("%s:%s", hostname, strconv.FormatUint(port, 10))}, cfg.Consume.Group, sConfig)
	if err != nil {
		log.Fatal(err)
	}
	cs.consumerGroup = consumerGroup

	return &cs
}

func buildKafkaConfig(cfg *config.KafkaConfig) (*sarama.Config, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka config is not valid: %w", err)
	}
	if err := cfg.Consume.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka consume config is not valid: %w", err)
	}

	sConfig := config.NewSaramaClientConfig(cfg)
	switch cfg.Consume.BalanceStrategy {
	case "sticky":
		sConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	case "roundrobin":
		sConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	case "range":
		sConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	default:
		log.Fatalf("Unrecognized consumer group partition assignor: %s", cfg.Consume.BalanceStrategy)
	}

	if cfg.Consume.InitialOffsetOldest {
		sConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	}
	sConfig.Consumer.MaxWaitTime = cfg.Consume.MaxWaitTime
	sConfig.Consumer.MaxProcessingTime = cfg.Consume.MaxProcessingTime
	sConfig.Consumer.Retry.Backoff = cfg.Consume.Backoff
	sConfig.Consumer.Return.Errors = true
	return sConfig, nil
}

func (cs *Consumer) Start(context context.Context, cancel context.CancelFunc) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// `Consume` should be called inside an infinite loop, when a
			// server-side rebalance happens, the consumer session will need to be
			// recreated to get the new claims
			if err := cs.consumerGroup.Consume(context, []string{cs.topic}, cs); err != nil {
				cs.log.Fatalf("Error from consumer: %v", err)
			}
			// check if context was cancelled, signaling that the consumer should stop
			if context.Err() != nil {
				return
			}
			cs.ready = make(chan bool)
		}
	}()

	<-cs.ready // Await till the consumer has been set up
	cs.log.Info("Sarama consumer up and running!...")

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-context.Done():
		cs.log.Info("terminating: context cancelled")
	case <-sigterm:
		cs.log.Info("terminating: via signal")
	}
	cancel()
	wg.Wait()
	if err := cs.consumerGroup.Close(); err != nil {
		cs.log.Fatalf("Error closing client: %v", err)
	}
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (cs *Consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	close(cs.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (cs *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (cs *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/Shopify/sarama/blob/main/consumer_group.go#L27-L29
	for message := range claim.Messages() {
		cs.log.Debugf("Message claimed: timestamp = %v, topic = %s, partition = %d, offset = %d",
			message.Timestamp, message.Topic, message.Partition, message.Offset)
		session.MarkMessage(message, "")

		var record map[string]interface{}
		err := json.Unmarshal(message.Value, &record)
		if err != nil {
			cs.log.Fatal(err)
		}
		if err := cs.enricher.Enrich(record); err != nil {
			cs.log.Error(err)
		}
	}

	return nil
}

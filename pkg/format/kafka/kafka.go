// Package kafka defines a kafka Format, implementing Format interface for input
package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type Reader struct {
	reader  *kafkago.Reader
	log     *logrus.Entry
	context context.Context
}

func buildKafkaConfig(hostname string, port int, cfg *config.KafkaConfig) (*kafkago.ReaderConfig, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka config is not valid: %w", err)
	}
	if err := cfg.Reader.Validate(); err != nil {
		return nil, fmt.Errorf("the provided kafka consume config is not valid: %w", err)
	}

	var groupBalancers []kafkago.GroupBalancer
	for _, group := range cfg.Reader.GroupBalancers {
		switch group {
		case "range":
			groupBalancers = append(groupBalancers, &kafkago.RangeGroupBalancer{})
		case "roundRobin":
			groupBalancers = append(groupBalancers, &kafkago.RoundRobinGroupBalancer{})
		case "rackAffinity":
			groupBalancers = append(groupBalancers, &kafkago.RackAffinityGroupBalancer{})
		default:
			return nil, fmt.Errorf("the provided kafka balancer is not valid: %s", group)
		}
	}

	var startOffset int64
	if cfg.Reader.StartFirstOffset {
		startOffset = kafkago.FirstOffset
	} else {
		startOffset = kafkago.LastOffset
	}

	return &kafkago.ReaderConfig{
		Dialer:                 config.NewKafkaDialer(cfg),
		Brokers:                []string{fmt.Sprintf("%s:%d", hostname, port)},
		Topic:                  cfg.Topic,
		GroupID:                cfg.Reader.GroupID,
		QueueCapacity:          cfg.Reader.QueueCapacity,
		MinBytes:               cfg.Reader.MinBytes,
		MaxBytes:               cfg.Reader.MaxBytes,
		MaxWait:                cfg.Reader.MaxWait,
		ReadLagInterval:        cfg.Reader.ReadLagInterval,
		GroupBalancers:         groupBalancers,
		HeartbeatInterval:      cfg.Reader.HeartbeatInterval,
		CommitInterval:         cfg.Reader.CommitInterval,
		PartitionWatchInterval: cfg.Reader.PartitionWatchInterval,
		WatchPartitionChanges:  cfg.Reader.WatchPartitionChanges,
		SessionTimeout:         cfg.Reader.SessionTimeout,
		RebalanceTimeout:       cfg.Reader.RebalanceTimeout,
		JoinGroupBackoff:       cfg.Reader.JoinGroupBackoff,
		RetentionTime:          cfg.Reader.RetentionTime,
		StartOffset:            startOffset,
		ReadBackoffMin:         cfg.Reader.ReadBackoffMin,
		ReadBackoffMax:         cfg.Reader.ReadBackoffMax,
	}, nil
}

func NewReader(ctx context.Context, hostname string, port int, log *logrus.Entry, cfg *config.KafkaConfig) *Reader {
	rc, err := buildKafkaConfig(hostname, port, cfg)
	if err != nil {
		log.Fatal(err)
	}
	return &Reader{
		reader:  kafkago.NewReader(*rc),
		log:     log,
		context: ctx,
	}
}

func (r *Reader) Next() (map[string]interface{}, error) {
	m, err := r.reader.ReadMessage(r.context)
	if err != nil {
		return nil, err
	}
	r.log.Debugf("read message: time = %v, topic = %s, partition = %d, offset = %d",
		m.Time, m.Topic, m.Partition, m.Offset)
	var record map[string]interface{}
	err = json.Unmarshal(m.Value, &record)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *Reader) Shutdown() {
	if err := r.reader.Close(); err != nil {
		r.log.Fatal("failed to close reader:", err)
	}
}

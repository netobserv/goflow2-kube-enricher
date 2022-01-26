// Package export enables data exporting to ingestion backends (e.g. Loki, Kafka)
package export

import (
	"errors"
	"math"
	"time"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export/kafka"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export/loki"
	"github.com/sirupsen/logrus"
)

type exporter interface {
	ProcessRecord(timestamp time.Time, record map[string]interface{}) error
}

// Exporters for record
type Exporters struct {
	config   *config.Config
	timeNow  func() time.Time
	exporter exporter
}

var (
	log = logrus.WithField("module", "export")
)

func NewExporters(cfg *config.Config) *Exporters {
	log.Info("creating Loki exporter...")

	if len(cfg.Kafka.Export.Brokers) > 0 {
		log.Info("creating Kafka exporter...")
		k, err := kafka.NewKafka(cfg.Kafka)
		if err != nil {
			log.WithError(err).Fatal("Can't create Kafka exporter")
		}
		return &Exporters{
			config:   cfg,
			timeNow:  time.Now,
			exporter: k,
		}
	}

	l, err := loki.NewLoki(cfg.Loki)
	if err != nil {
		log.WithError(err).Fatal("Can't create Loki exporter")
	}
	return &Exporters{
		config:   cfg,
		timeNow:  time.Now,
		exporter: l,
	}

}

func (e *Exporters) extractTimestamp(record map[string]interface{}) time.Time {
	if e.config.TimestampLabel == "" {
		return e.timeNow()
	}
	timestamp, ok := record[string(e.config.TimestampLabel)]
	if !ok {
		log.WithField("timestampLabel", e.config.TimestampLabel).
			Warnf("Timestamp label not found in record. Using local time")
		return e.timeNow()
	}
	ft, ok := getFloat64(timestamp)
	if !ok {
		log.WithField(string(e.config.TimestampLabel), timestamp).
			Warnf("Invalid timestamp found: float64 expected but got %T. Using local time", timestamp)
		return e.timeNow()
	}
	if ft == 0 {
		log.WithField("timestampLabel", e.config.TimestampLabel).
			Warnf("Empty timestamp in record. Using local time")
		return e.timeNow()
	}
	tsNanos := int64(ft * float64(e.config.TimestampScale))
	return time.Unix(tsNanos/int64(time.Second), tsNanos%int64(time.Second))
}

func getFloat64(timestamp interface{}) (ft float64, ok bool) {
	switch i := timestamp.(type) {
	case float64:
		return i, true
	case float32:
		return float64(i), true
	case int64:
		return float64(i), true
	case int32:
		return float64(i), true
	case uint64:
		return float64(i), true
	case uint32:
		return float64(i), true
	case int:
		return float64(i), true
	default:
		log.Warnf("Type %T is not implemented for float64 conversion\n", i)
		return math.NaN(), false
	}
}

func (e *Exporters) ProcessRecord(record map[string]interface{}) error {
	// Get timestamp from record (default: TimeFlowStart)
	timestamp := e.extractTimestamp(record)
	if e.exporter != nil {
		return e.exporter.ProcessRecord(timestamp, record)
	}
	return errors.New("process record called but no exporter configured")
}

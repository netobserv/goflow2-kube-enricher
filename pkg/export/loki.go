// Package export enables data exporting to ingestion backends (e.g. Loki)
package export

import (
	"fmt"
	"math"
	"strings"
	"time"

	logadapter "github.com/go-kit/kit/log/logrus"
	jsoniter "github.com/json-iterator/go"
	"github.com/netobserv/loki-client-go/loki"
	"github.com/netobserv/loki-client-go/pkg/backoff"
	"github.com/netobserv/loki-client-go/pkg/urlutil"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

var (
	keyReplacer = strings.NewReplacer("/", "_", ".", "_", "-", "_")
	log         = logrus.WithField("module", "export/loki")
)

// Emitter abstracts the records' ingester (e.g. the Loki client)
type emitter interface {
	Handle(labels model.LabelSet, timestamp time.Time, record string) error
}

// Loki record exporter
type Loki struct {
	config     config.LokiConfig
	lokiConfig loki.Config
	emitter    emitter
	timeNow    func() time.Time
}

// NewLoki creates a Loki flow exporter from a given configuration
func NewLoki(cfg *config.LokiConfig) (Loki, error) {
	if err := cfg.Validate(); err != nil {
		return Loki{}, fmt.Errorf("the provided config is not valid: %w", err)
	}
	lcfg, err := buildLokiConfig(cfg)
	if err != nil {
		return Loki{}, err
	}
	lokiClient, err := loki.NewWithLogger(lcfg, logadapter.NewLogger(log))
	if err != nil {
		return Loki{}, err
	}
	return Loki{
		config:     *cfg,
		lokiConfig: lcfg,
		emitter:    lokiClient,
		timeNow:    time.Now,
	}, nil
}

func buildLokiConfig(c *config.LokiConfig) (loki.Config, error) {
	cfg := loki.Config{
		TenantID:  c.TenantID,
		BatchWait: c.BatchWait,
		BatchSize: c.BatchSize,
		Timeout:   c.Timeout,
		BackoffConfig: backoff.BackoffConfig{
			MinBackoff: c.MinBackoff,
			MaxBackoff: c.MaxBackoff,
			MaxRetries: c.MaxRetries,
		},
		Client: c.ClientConfig,
	}
	var clientURL urlutil.URLValue
	err := clientURL.Set(strings.TrimSuffix(c.URL, "/") + "/loki/api/v1/push")
	if err != nil {
		return cfg, fmt.Errorf("failed to parse client URL: %w", err)
	}
	cfg.URL = clientURL
	return cfg, nil
}

func (l *Loki) ProcessRecord(record map[string]interface{}) error {
	// Get timestamp from record (default: TimeFlowStart)
	timestamp := l.extractTimestamp(record)

	labels := model.LabelSet{}

	// Add static labels from config
	for k, v := range l.config.StaticLabels {
		labels[k] = v
	}

	l.addNonStaticLabels(record, labels)

	// Remove labels and configured ignore list from record
	ignoreList := append(l.config.IgnoreList, l.config.Labels...)
	for _, label := range ignoreList {
		delete(record, label)
	}

	js, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(record)
	if err != nil {
		return err
	}
	return l.emitter.Handle(labels, timestamp, string(js))
}

func (l *Loki) extractTimestamp(record map[string]interface{}) time.Time {
	if l.config.TimestampLabel == "" {
		return l.timeNow()
	}
	timestamp, ok := record[string(l.config.TimestampLabel)]
	if !ok {
		log.WithField("timestampLabel", l.config.TimestampLabel).
			Warnf("Timestamp label not found in record. Using local time")
		return l.timeNow()
	}
	ft, ok := getFloat64(timestamp)
	if !ok {
		log.WithField(string(l.config.TimestampLabel), timestamp).
			Warnf("Invalid timestamp found: float64 expected but got %T. Using local time", timestamp)
		return l.timeNow()
	}
	if ft == 0 {
		log.WithField("timestampLabel", l.config.TimestampLabel).
			Warnf("Empty timestamp in record. Using local time")
		return l.timeNow()
	}
	tsNanos := int64(ft * float64(l.config.TimestampScale))
	return time.Unix(tsNanos/int64(time.Second), tsNanos%int64(time.Second))
}

func (l *Loki) addNonStaticLabels(record map[string]interface{}, labels model.LabelSet) {
	// Add non-static labels from record
	for _, label := range l.config.Labels {
		val, ok := record[label]
		if !ok {
			continue
		}
		sanitizedKey := model.LabelName(keyReplacer.Replace(label))
		if !sanitizedKey.IsValid() {
			log.WithFields(logrus.Fields{"key": label, "sanitizedKey": sanitizedKey}).
				Debug("Invalid label. Ignoring it")
			continue
		}
		lv := model.LabelValue(fmt.Sprint(val))
		if !lv.IsValid() {
			log.WithFields(logrus.Fields{"key": label, "sanitizedKey": sanitizedKey, "value": val}).
				Debug("Invalid label value. Ignoring it")
			continue
		}
		labels[sanitizedKey] = lv
	}
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

package config

import (
	"errors"
	"fmt"
	"time"

	promconf "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

type LokiConfig struct {
	URL          string                    `yaml:"url"`
	TenantID     string                    `yaml:"tenantID"`
	BatchWait    time.Duration             `yaml:"batchWait"`
	BatchSize    int                       `yaml:"batchSize"`
	Timeout      time.Duration             `yaml:"timeout"`
	MinBackoff   time.Duration             `yaml:"minBackoff"`
	MaxBackoff   time.Duration             `yaml:"maxBackoff"`
	MaxRetries   int                       `yaml:"maxRetries"`
	Labels       []string                  `yaml:"labels"`
	StaticLabels model.LabelSet            `yaml:"staticLabels"`
	IgnoreList   []string                  `yaml:"ignoreList"`
	ClientConfig promconf.HTTPClientConfig `yaml:"clientConfig"`
}

func (c *LokiConfig) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.URL == "" {
		return errors.New("url can't be empty")
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("invalid batchSize: %v. Required > 0", c.BatchSize)
	}
	return nil
}

func DefaultLoki() *LokiConfig {
	return &LokiConfig{
		URL:        "http://loki:3100/",
		BatchWait:  1 * time.Second,
		BatchSize:  100 * 1024,
		Timeout:    10 * time.Second,
		MinBackoff: 1 * time.Second,
		MaxBackoff: 5 * time.Minute,
		MaxRetries: 10,
		StaticLabels: model.LabelSet{
			"app": "goflow-kube",
		},
	}
}

// Package config provides configuration management functions
package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var clog = logrus.WithField("module", "config")

const JSONFlagName = "json"
const PBFlagName = "pb"

type Config struct {
	Listen         string          `yaml:"listen"`
	StdinFormat    string          `yaml:"stdinFormat"`
	Loki           *LokiConfig     `yaml:"loki"`
	Kafka          *KafkaConfig    `yaml:"kafka"`
	TimestampLabel model.LabelName `yaml:"timestampLabel"`
	// TimestampScale provides the scale in time of the units from the timestamp
	// E.g. UNIX time scale is '1s' (one second) while other clock sources might have
	// scales of '1ms' (one millisecond) or just '1' (one nanosecond)
	// Default value is '1s'
	TimestampScale time.Duration     `yaml:"timestampScale"`
	IPFields       map[string]string `yaml:"ipFields"`
	Enrich         bool              `yaml:"enrich"`
	PrintInput     bool              `yaml:"printInput"`
	PrintOutput    bool              `yaml:"printOutput"`
}

// Load loads the YAML configuration from the file path passed as argument
func Load(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return Read(file)
}

// Read reads a YAML configuration from the io.Reader passed as argument
func Read(in io.Reader) (*Config, error) {
	bytes, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	c := Default()
	err = yaml.Unmarshal(bytes, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	clog.WithField("config", c).Debug("loaded configuration")
	return c, nil
}

func Default() *Config {
	return &Config{
		StdinFormat: JSONFlagName,
		IPFields: map[string]string{
			"SrcAddr": "Src",
			"DstAddr": "Dst",
		},
		Loki:           DefaultLoki(),
		Kafka:          DefaultKafka(),
		TimestampLabel: "TimeReceived",
		TimestampScale: time.Second,
		Enrich:         true,
	}
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("you must provide a configuration")
	}
	if c.TimestampScale == 0 {
		return errors.New("timestampUnit must be a valid Duration > 0 (e.g. 1m, 1s or 1ms)")
	}
	return nil
}

package kafka

import (
	"io/ioutil"
	"testing"
	"time"

	sarama "github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

func TestKafla_BuildClientConfig(t *testing.T) {
	// GIVEN a file that overrides some fields in the default configuration
	cfgFile, err := ioutil.TempFile("", "testload_")
	require.NoError(t, err)
	_, err = cfgFile.WriteString(`
kafka:
  version: 3.0.0
  consume:
    balanceStrategy: sticky
    initialOffsetOldest: true
    backoff: 1000000000
    maxWaitTime: 2000000000
    maxProcessingTime: 3000000000
printInput: true
`)
	require.NoError(t, err)
	require.NoError(t, cfgFile.Close())

	// WHEN it is loaded and transformed into a Kafka Config
	cfg, err := config.Load(cfgFile.Name())
	require.NoError(t, err)

	// THEN the generated kafka config is consistent with the parsed data
	scfg, err := buildKafkaConfig(cfg.Kafka)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", scfg.Version.String())
	assert.Equal(t, sarama.BalanceStrategySticky, scfg.Consumer.Group.Rebalance.Strategy)
	assert.Equal(t, sarama.OffsetOldest, scfg.Consumer.Offsets.Initial)
	assert.Equal(t, time.Duration(1000000000), scfg.Consumer.Retry.Backoff)
	assert.Equal(t, time.Duration(2000000000), scfg.Consumer.MaxWaitTime)
	assert.Equal(t, time.Duration(3000000000), scfg.Consumer.MaxProcessingTime)
}

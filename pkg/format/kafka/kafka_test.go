package kafka

import (
	"io/ioutil"
	"testing"

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
  topic: testTopic
  reader:
    groupID: testGroup
printInput: true
`)
	require.NoError(t, err)
	require.NoError(t, cfgFile.Close())

	// WHEN it is loaded and transformed into a Kafka Config
	cfg, err := config.Load(cfgFile.Name())
	require.NoError(t, err)

	// THEN the generated kafka config is consistent with the parsed data
	rc, err := buildKafkaConfig("http://my-kafka", 9092, cfg.Kafka)
	require.NoError(t, err)
	assert.Equal(t, []string{"http://my-kafka:9092"}, rc.Brokers)
	assert.Equal(t, "testTopic", rc.Topic)
	assert.Equal(t, "testGroup", rc.GroupID)
}

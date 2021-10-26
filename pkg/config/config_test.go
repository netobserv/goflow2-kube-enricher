package config

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_BuildLokiClientConfig(t *testing.T) {
	// GIVEN a file that overrides some fields in the default configuration
	cfgFile, err := ioutil.TempFile("", "testload_")
	require.NoError(t, err)
	_, err = cfgFile.WriteString(`
loki:
  tenantID: theTenant
  url: "https://foo:8888/"
  batchWait: 1m
  minBackOff: 5s
  labels:
    - foo
    - bar
  staticLabels:
    baz: bae
    tiki: taka
printInput: true
`)
	require.NoError(t, err)
	require.NoError(t, cfgFile.Close())

	// WHEN it is loaded
	cfg, err := Load(cfgFile.Name())

	// THEN it is transformed into a Loki Config
	require.NoError(t, err)
	assert.Contains(t, cfg.Loki.Labels, "foo")
	assert.Contains(t, cfg.Loki.Labels, "bar")
	assert.True(t, cfg.PrintInput)
	assert.EqualValues(t, "bae", cfg.Loki.StaticLabels["baz"])
	assert.EqualValues(t, "taka", cfg.Loki.StaticLabels["tiki"])
}

package loki

import (
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

var now = time.Now()

type fakeEmitter struct {
	mock.Mock
}

func (f *fakeEmitter) Handle(labels model.LabelSet, timestamp time.Time, record string) error {
	a := f.Mock.Called(labels, timestamp, record)
	return a.Error(0)
}

func TestLoki_BuildClientConfig(t *testing.T) {
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

	// WHEN it is loaded and transformed into a Loki Config
	cfg, err := config.Load(cfgFile.Name())
	require.NoError(t, err)

	// THEN the generated loki config is consistent with the parsed data
	ccfg, err := buildLokiConfig(cfg.Loki)
	require.NoError(t, err)

	require.NotNil(t, ccfg.URL)
	assert.Equal(t, "https://foo:8888/loki/api/v1/push", ccfg.URL.String())
	assert.Equal(t, "theTenant", ccfg.TenantID)
	assert.Equal(t, time.Minute, ccfg.BatchWait)
	assert.NotZero(t, ccfg.BatchSize)
	assert.Equal(t, cfg.Loki.BatchSize, ccfg.BatchSize)
	assert.Equal(t, cfg.Loki.MinBackoff, ccfg.BackoffConfig.MinBackoff)
}

func TestLoki_ProcessRecord(t *testing.T) {
	// GIVEN a Loki exporter
	fe := fakeEmitter{}
	fe.On("Handle", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	cfg, err := config.Read(strings.NewReader(`
loki:
  timestampLabel: ts
  ignoreList:
    - ignored
  staticLabels:
    static: label
  labels:
    - foo
    - bar
printInput: true
printOutput: true
`))
	require.NoError(t, err)
	loki, err := NewLoki(cfg.Loki)
	require.NoError(t, err)
	loki.emitter = &fe

	// WHEN it processes input records
	require.NoError(t, loki.ProcessRecord(now, map[string]interface{}{
		"ts": 123456, "ignored": "ignored!", "foo": "fooLabel", "bar": "barLabel", "value": 1234}))
	require.NoError(t, loki.ProcessRecord(now, map[string]interface{}{
		"ts": 124567, "ignored": "ignored!", "foo": "fooLabel2", "bar": "barLabel2", "value": 5678, "other": "val"}))

	// THEN it forwards the records extracting the labels from the configuration
	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":    "goflow-kube",
		"bar":    "barLabel",
		"foo":    "fooLabel",
		"static": "label",
	}, now, `{"ts":123456,"value":1234}`)

	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":    "goflow-kube",
		"bar":    "barLabel2",
		"foo":    "fooLabel2",
		"static": "label",
	}, now, `{"other":"val","ts":124567,"value":5678}`)
}

// Tests that labels are sanitized before being sent to loki.
// Labels that are invalid even if sanitized are ignored
func TestSanitizedLabels(t *testing.T) {
	fe := fakeEmitter{}
	fe.On("Handle", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	cfg, err := config.Read(strings.NewReader(`
loki:
  labels:
    - "fo.o"
    - "ba-r"
    - "ba/z"
    - "ignored?"
`))
	require.NoError(t, err)
	loki, err := NewLoki(cfg.Loki)
	require.NoError(t, err)
	loki.emitter = &fe

	require.NoError(t, loki.ProcessRecord(now, map[string]interface{}{
		"ba/z": "isBaz", "fo.o": "isFoo", "ba-r": "isBar", "ignored?": "yes!"}))

	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":  "goflow-kube",
		"ba_r": "isBar",
		"fo_o": "isFoo",
		"ba_z": "isBaz",
	}, mock.Anything, mock.Anything)
}

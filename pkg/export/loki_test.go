package export

import (
	"encoding/json"
	"fmt"
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
	ccfg, err := buildLokiConfig(&cfg.Loki)
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
	loki, err := NewLoki(&cfg.Loki)
	require.NoError(t, err)
	loki.emitter = &fe

	// WHEN it processes input records
	require.NoError(t, loki.ProcessRecord(map[string]interface{}{
		"ts": 123456, "ignored": "ignored!", "foo": "fooLabel", "bar": "barLabel", "value": 1234}))
	require.NoError(t, loki.ProcessRecord(map[string]interface{}{
		"ts": 124567, "ignored": "ignored!", "foo": "fooLabel2", "bar": "barLabel2", "value": 5678, "other": "val"}))

	// THEN it forwards the records extracting the timestamp and labels from the configuration
	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":    "goflow-kube",
		"bar":    "barLabel",
		"foo":    "fooLabel",
		"static": "label",
	}, time.Unix(123456, 0), `{"ts":123456,"value":1234}`)

	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":    "goflow-kube",
		"bar":    "barLabel2",
		"foo":    "fooLabel2",
		"static": "label",
	}, time.Unix(124567, 0), `{"other":"val","ts":124567,"value":5678}`)
}

func TestTimestampScale(t *testing.T) {
	// verifies that the unix residual time (below 1-second precision) is properly
	// incorporated into the timestamp whichever scale it is
	for _, testCase := range []struct {
		unit     string
		expected time.Time
	}{
		{unit: "1m", expected: time.Unix(123456789*60, 0)},
		{unit: "1s", expected: time.Unix(123456789, 0)},
		{unit: "100ms", expected: time.Unix(12345678, 900000000)},
		{unit: "1ms", expected: time.Unix(123456, 789000000)},
	} {
		t.Run(fmt.Sprintf("unit %v", testCase.unit), func(t *testing.T) {
			fe := fakeEmitter{}
			fe.On("Handle", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			cfg, err := config.Read(strings.NewReader(
				fmt.Sprintf("loki: {timestampScale: %s}", testCase.unit)))
			require.NoError(t, err)
			loki, err := NewLoki(&cfg.Loki)
			require.NoError(t, err)
			loki.emitter = &fe

			require.NoError(t, loki.ProcessRecord(map[string]interface{}{"TimeReceived": 123456789}))
			fe.AssertCalled(t, "Handle", model.LabelSet{"app": "goflow-kube"},
				testCase.expected, `{"TimeReceived":123456789}`)
		})
	}
}

// Tests those cases where the timestamp can't be extracted and reports the current time
func TestTimestampExtraction_LocalTime(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		tsLabel model.LabelName
		input   map[string]interface{}
	}{
		{name: "undefined ts label", tsLabel: "", input: map[string]interface{}{"ts": 444}},
		{name: "non-existing ts entry", tsLabel: "asdfasdf", input: map[string]interface{}{"ts": 444}},
		{name: "non-numeric ts value", tsLabel: "ts", input: map[string]interface{}{"ts": "string value"}},
		{name: "zero ts value", tsLabel: "ts", input: map[string]interface{}{"ts": 0}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fe := fakeEmitter{}
			fe.On("Handle", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			cfg := config.Default()
			cfg.Loki.TimestampLabel = testCase.tsLabel
			loki, err := NewLoki(&cfg.Loki)
			require.NoError(t, err)
			loki.emitter = &fe
			loki.timeNow = func() time.Time {
				return time.Unix(12345678, 0)
			}
			jsonInput, _ := json.Marshal(testCase.input)
			require.NoError(t, loki.ProcessRecord(testCase.input))
			fe.AssertCalled(t, "Handle", model.LabelSet{"app": "goflow-kube"},
				time.Unix(12345678, 0), string(jsonInput))
		})
	}
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
	loki, err := NewLoki(&cfg.Loki)
	require.NoError(t, err)
	loki.emitter = &fe

	require.NoError(t, loki.ProcessRecord(map[string]interface{}{
		"ba/z": "isBaz", "fo.o": "isFoo", "ba-r": "isBar", "ignored?": "yes!"}))

	fe.AssertCalled(t, "Handle", model.LabelSet{
		"app":  "goflow-kube",
		"ba_r": "isBar",
		"fo_o": "isFoo",
		"ba_z": "isBaz",
	}, mock.Anything, mock.Anything)
}

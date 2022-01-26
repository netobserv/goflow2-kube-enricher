package export

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
)

type fakeExporter struct {
	mock.Mock
}

func (f *fakeExporter) ProcessRecord(timestamp time.Time, record map[string]interface{}) error {
	a := f.Mock.Called(timestamp, record)
	return a.Error(0)
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
			fe := fakeExporter{}
			fe.On("ProcessRecord", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			cfg, err := config.Read(strings.NewReader(fmt.Sprintf("timestampScale: %s", testCase.unit)))
			require.NoError(t, err)
			e := NewExporters(cfg)
			e.exporter = &fe

			record := map[string]interface{}{"TimeReceived": 123456789}
			require.NoError(t, e.ProcessRecord(record))
			fe.AssertCalled(t, "ProcessRecord", testCase.expected, record)
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
			fe := fakeExporter{}
			fe.On("ProcessRecord", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			cfg := config.Default()
			cfg.TimestampLabel = testCase.tsLabel
			e := NewExporters(cfg)
			e.exporter = &fe
			e.timeNow = func() time.Time {
				return time.Unix(12345678, 0)
			}
			require.NoError(t, e.ProcessRecord(testCase.input))
			fe.AssertCalled(t, "ProcessRecord", time.Unix(12345678, 0), testCase.input)
		})
	}
}

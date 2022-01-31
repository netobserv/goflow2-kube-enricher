package health

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netsampler/goflow2/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	status         Status
	path           string
	expectedCode   int
	expectedStatus string
}

func TestHttpReporter_Health(t *testing.T) {
	reporter := NewReporter(Starting)
	svc := NewHTTPReporter(reporter)
	server := httptest.NewServer(svc.Handler())
	defer server.Close()

	for _, tc := range []testCase{
		{status: Starting, path: "/health", expectedCode: 503, expectedStatus: "DOWN"},
		{status: Starting, path: "/health/ready", expectedCode: 503, expectedStatus: "DOWN"},
		{status: Starting, path: "/health/live", expectedCode: 200, expectedStatus: "UP"},
		{status: Ready, path: "/health", expectedCode: 200, expectedStatus: "UP"},
		{status: Ready, path: "/health/ready", expectedCode: 200, expectedStatus: "UP"},
		{status: Ready, path: "/health/live", expectedCode: 200, expectedStatus: "UP"},
		{status: Error, path: "/health", expectedCode: 503, expectedStatus: "DOWN"},
		{status: Error, path: "/health/ready", expectedCode: 503, expectedStatus: "DOWN"},
		{status: Error, path: "/health/live", expectedCode: 503, expectedStatus: "DOWN"},
	} {
		t.Run(tc.status.String()+tc.path, func(t *testing.T) {
			reporter.Status = tc.status
			resp, err := server.Client().Get(server.URL + tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCode, resp.StatusCode)
			assert.Contains(t, resp.Header["Content-Type"], "application/json")
			reported := Report{}
			d := json.NewDecoder(resp.Body)
			require.NoError(t, d.Decode(&reported))
			assert.Equal(t, tc.expectedStatus, reported.Status)
			require.Len(t, reported.Checks, 1)
			assert.Equal(t, "flows", reported.Checks[0].Name)
			assert.Equal(t, tc.expectedStatus, reported.Checks[0].Status)
			require.IsType(t, map[string]interface{}{}, reported.Checks[0].Data)
			assert.Contains(t, reported.Checks[0].Data.(map[string]interface{}), "host")
		})
	}
}

func TestHttpReporter_Metrics(t *testing.T) {
	reporter := NewReporter(Ready)
	svc := NewHTTPReporter(reporter)
	svc.reporter.RecordEnriched()
	svc.reporter.RecordDiscarded(errors.New("file not found"))
	utils.NetFlowStats.WithLabelValues("foo", "9").Inc()
	utils.NetFlowStats.WithLabelValues("foo", "9").Inc()
	utils.NetFlowStats.WithLabelValues("bar", "9").Inc()
	utils.NetFlowErrors.WithLabelValues("foo", "boom").Inc()
	server := httptest.NewServer(svc.Handler())
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/metrics")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(bodyBytes)
	assert.Contains(t, body, `flow_process_nf_count{router="foo",version="9"} 2`)
	assert.Contains(t, body, `flow_process_nf_count{router="bar",version="9"} 1`)
	assert.Contains(t, body, `flow_process_nf_errors_count{error="boom",router="foo"} 1`)
	assert.Contains(t, body, "reader_record_enriched 1")
	assert.Contains(t, body, `reader_record_discarded{error="file not found"} 1`)
}

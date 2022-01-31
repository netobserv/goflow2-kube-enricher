package health

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"github.com/netsampler/goflow2/utils"
)

const (
	contentTypeHeader   = "Content-Type"
	healthContentType   = "application/json"
	statusCodeUnhealthy = http.StatusServiceUnavailable
	statusCodeHealthy   = http.StatusOK
	statusUp            = "UP"
	statusDown          = "DOWN"
	statusNameFlows     = "flows"
	hostDataFieldName   = "host"
	endpointHealth      = "/health"
	endpointLive        = "/health/live"
	endpointReady       = "/health/ready"
	endpointMetrics     = "/metrics"
)

// HTTPReporter for health and metrics
type HTTPReporter struct {
	endpoints *http.ServeMux
	reporter  *Reporter
	registry  *prometheus.Registry
}

// Report is the representation of the information that will be presented to the
// invoker of the health status. It follows the Microprofile Health 2.1 specification:
// https://download.eclipse.org/microprofile/microprofile-health-2.1/microprofile-health-spec.html
type Report struct {
	Status string        `json:"status"`
	Checks []StatusCheck `json:"checks"`
}

type StatusCheck struct {
	Name   string      `json:"name"`
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

func NewHTTPReporter(reporter *Reporter) HTTPReporter {
	reg := prometheus.NewRegistry()
	reg.MustRegister(reporter.recordEnriched)
	reg.MustRegister(reporter.recordDiscarded)
	reg.MustRegister(utils.NetFlowStats)
	reg.MustRegister(utils.NetFlowErrors)
	reg.MustRegister(utils.NetFlowSetRecordsStatsSum)
	reg.MustRegister(utils.NetFlowTimeStatsSum)
	reg.MustRegister(utils.NetFlowSetStatsSum)
	reg.MustRegister(utils.NetFlowTemplatesStats)
	hr := HTTPReporter{
		reporter:  reporter,
		endpoints: http.NewServeMux(),
		registry:  reg,
	}
	hr.endpoints.HandleFunc(endpointHealth, hr.ready)
	hr.endpoints.HandleFunc(endpointLive, hr.live)
	hr.endpoints.HandleFunc(endpointReady, hr.ready)
	hr.endpoints.HandleFunc(endpointMetrics, hr.metrics)
	return hr
}

func (hs *HTTPReporter) Handler() http.Handler {
	return hs.endpoints
}

func (hs *HTTPReporter) live(rw http.ResponseWriter, _ *http.Request) {
	hs.writeStatus(rw, hs.reporter.Status != Error)
}

func (hs *HTTPReporter) ready(rw http.ResponseWriter, _ *http.Request) {
	hs.writeStatus(rw, hs.reporter.Status == Ready)
}

func (hs *HTTPReporter) writeStatus(rw http.ResponseWriter, up bool) {
	host, err := os.Hostname()
	if err != nil {
		host = err.Error()
	}
	checkMetrics := map[string]interface{}{
		hostDataFieldName: host,
	}
	var statusName string
	var statusCode int
	if up {
		statusName = statusUp
		statusCode = statusCodeHealthy
	} else {
		statusName = statusDown
		statusCode = statusCodeUnhealthy
	}
	report := Report{
		Status: statusName,
		Checks: []StatusCheck{{
			Name:   statusNameFlows,
			Status: statusName,
			Data:   checkMetrics,
		}},
	}
	rw.Header().Set(contentTypeHeader, healthContentType)
	rw.WriteHeader(statusCode)
	out, err := json.Marshal(report)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := rw.Write(out); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

func (hs *HTTPReporter) metrics(rw http.ResponseWriter, _ *http.Request) {
	mfs, err := hs.registry.Gather()
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	for _, mf := range mfs {
		if _, err := expfmt.MetricFamilyToText(rw, mf); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte(err.Error()))
			return
		}
	}
}

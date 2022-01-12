// Package health provides the mechanism to report the health status of the service
package health

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Status represents the different states that a service can have
type Status int

const (
	// Starting the service. It is not yet ready to receive requests
	Starting Status = iota
	// Ready service. It is healthy and ready to receive requests
	Ready
	// Error status. The service is not healthy
	Error
)

func (s Status) String() string {
	switch s {
	case Starting:
		return "STARTING"
	case Ready:
		return "READY"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Reporter of the health status and the statistics of the service
type Reporter struct {
	Status          Status
	recordEnriched  prometheus.Counter
	recordDiscarded *prometheus.CounterVec
}

func NewReporter(s Status) *Reporter {
	return &Reporter{
		Status: s,
		recordEnriched: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "reader_record_enriched",
				Help: "Number of records that have been successfully received and enriched.",
			},
			[]string{},
		).WithLabelValues(),
		recordDiscarded: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "reader_record_discarded",
				Help: "Number of received received records that could not be enriched.",
			},
			[]string{"error"},
		),
	}
}

// RecordEnriched annotates a record as successfully processed
func (r *Reporter) RecordEnriched() {
	r.recordEnriched.Inc()
}

// RecordDiscarded annotates that a record couldn't be enriched due to the given error
func (r *Reporter) RecordDiscarded(err error) {
	r.recordDiscarded.WithLabelValues(err.Error()).Inc()
}

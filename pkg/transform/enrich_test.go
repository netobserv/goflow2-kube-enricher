package transform

import (
	"testing"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/health"
	"github.com/netobserv/goflow2-kube-enricher/pkg/internal/mock"
	"github.com/stretchr/testify/assert"
)

func setupSimpleEnricher() (*Enricher, *mock.InformersMock) {
	informers := new(mock.InformersMock)
	r := Enricher{
		Config:    config.Default(),
		Informers: informers,
		Health:    health.NewReporter(health.Starting),
	}
	return &r, informers
}

func TestEnrichNoMatch(t *testing.T) {
	assert := assert.New(t)
	r, informers := setupSimpleEnricher()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockNoMatch("10.0.0.2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	records = r.Enrich(records)
	assert.Equal(map[string]interface{}{
		"SrcAddr":         "10.0.0.1",
		"SrcPod":          "test-pod1",
		"SrcNamespace":    "test-namespace",
		"SrcHostIP":       "10.0.0.100",
		"SrcWorkload":     "test-pod1",
		"SrcWorkloadKind": "Pod",
		"DstAddr":         "10.0.0.2",
	}, records)
}

func TestEnrichSinglePods(t *testing.T) {
	assert := assert.New(t)
	r, informers := setupSimpleEnricher()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockPod("test-pod2", "test-namespace", "10.0.0.2", "10.0.0.100")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	records = r.Enrich(records)
	assert.Equal(map[string]interface{}{
		"SrcAddr":         "10.0.0.1",
		"SrcPod":          "test-pod1",
		"SrcNamespace":    "test-namespace",
		"SrcHostIP":       "10.0.0.100",
		"SrcWorkload":     "test-pod1",
		"SrcWorkloadKind": "Pod",
		"DstAddr":         "10.0.0.2",
		"DstPod":          "test-pod2",
		"DstNamespace":    "test-namespace",
		"DstHostIP":       "10.0.0.100",
		"DstWorkload":     "test-pod2",
		"DstWorkloadKind": "Pod",
	}, records)
}

func TestEnrichDeploymentPods(t *testing.T) {
	assert := assert.New(t)
	r, informers := setupSimpleEnricher()

	informers.MockPodInDepl("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100", "test-rs-1", "test-deployment1")
	informers.MockPodInDepl("test-pod2", "test-namespace", "10.0.0.2", "10.0.0.100", "test-rs-2", "test-deployment2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	records = r.Enrich(records)
	assert.Equal(map[string]interface{}{
		"SrcAddr":         "10.0.0.1",
		"SrcPod":          "test-pod1",
		"SrcNamespace":    "test-namespace",
		"SrcHostIP":       "10.0.0.100",
		"SrcWorkload":     "test-deployment1",
		"SrcWorkloadKind": "Deployment",
		"DstAddr":         "10.0.0.2",
		"DstPod":          "test-pod2",
		"DstNamespace":    "test-namespace",
		"DstHostIP":       "10.0.0.100",
		"DstWorkload":     "test-deployment2",
		"DstWorkloadKind": "Deployment",
	}, records)
}

func TestEnrichPodAndService(t *testing.T) {
	assert := assert.New(t)
	r, informers := setupSimpleEnricher()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockService("test-service", "test-namespace", "10.0.0.2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	records = r.Enrich(records)
	assert.Equal(map[string]interface{}{
		"SrcAddr":         "10.0.0.1",
		"SrcPod":          "test-pod1",
		"SrcNamespace":    "test-namespace",
		"SrcHostIP":       "10.0.0.100",
		"SrcWorkload":     "test-pod1",
		"SrcWorkloadKind": "Pod",
		"DstAddr":         "10.0.0.2",
		"DstNamespace":    "test-namespace",
		"DstWorkload":     "test-service",
		"DstWorkloadKind": "Service",
	}, records)
}

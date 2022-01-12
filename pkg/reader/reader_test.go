package reader

import (
	"context"
	"testing"
	"time"

	"github.com/netobserv/goflow2-kube-enricher/pkg/health"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/internal/mock"
)

var spy = SpyDriver{shutdownCalled: false, nextCalled: false}

type SpyDriver struct {
	nextCalled     bool
	shutdownCalled bool
}

type TestDriver struct{}

func (gf *TestDriver) Next() (map[string]interface{}, error) {
	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}
	spy.nextCalled = true
	return records, nil
}

func (gf *TestDriver) Shutdown() {
	spy.shutdownCalled = true
}

func setupSimpleReader() (*Reader, *mock.InformersMock) {
	informers := new(mock.InformersMock)
	r := Reader{
		format:    &TestDriver{},
		log:       logrus.NewEntry(logrus.New()),
		informers: informers,
		config:    config.Default(),
		health:    health.NewReporter(health.Starting),
	}
	return &r, informers
}

func TestEnrichNoMatch(t *testing.T) {
	assert := assert.New(t)
	r, informers := setupSimpleReader()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockNoMatch("10.0.0.2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	err := r.enrich(records, nil)

	assert.Nil(err)
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
	r, informers := setupSimpleReader()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockPod("test-pod2", "test-namespace", "10.0.0.2", "10.0.0.100")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	err := r.enrich(records, nil)

	assert.Nil(err)
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
	r, informers := setupSimpleReader()

	informers.MockPodInDepl("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100", "test-rs-1", "test-deployment1")
	informers.MockPodInDepl("test-pod2", "test-namespace", "10.0.0.2", "10.0.0.100", "test-rs-2", "test-deployment2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	err := r.enrich(records, nil)

	assert.Nil(err)
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
	r, informers := setupSimpleReader()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockService("test-service", "test-namespace", "10.0.0.2")

	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}

	err := r.enrich(records, nil)

	assert.Nil(err)
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

func TestShutdown(t *testing.T) {
	loki := export.NewEmptyLoki()
	r, informers := setupSimpleReader()

	informers.MockPod("test-pod1", "test-namespace", "10.0.0.1", "10.0.0.100")
	informers.MockService("test-service", "test-namespace", "10.0.0.2")

	ctx, cancel := context.WithCancel(context.TODO())
	go r.Start(ctx, &loki)

	//check if next has been called and ensure shutdown has not been called, then cancel
	assert.Eventually(t, func() bool { return spy.nextCalled }, time.Second, time.Millisecond)
	assert.Equal(t, false, spy.shutdownCalled)
	cancel()

	//check if shutdown has been called then reset spy
	time.Sleep(time.Millisecond)
	assert.Eventually(t, func() bool { return spy.shutdownCalled }, time.Second, time.Millisecond)
	spy = SpyDriver{shutdownCalled: false, nextCalled: false}

	//check that shutdown and next are not called anymore
	time.Sleep(time.Millisecond)
	assert.Eventually(t, func() bool { return !spy.shutdownCalled }, time.Second, time.Millisecond)
	assert.Eventually(t, func() bool { return !spy.nextCalled }, time.Second, time.Millisecond)
}

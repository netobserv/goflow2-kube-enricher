package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const timeout = 5 * time.Second

func TestIntegration(t *testing.T) {
	// GIVEN a running cluster
	clientset := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "influxdb-v2",
			Namespace: "default",
			Annotations: map[string]string{
				"anonotation": "true",
			},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			PodIP:  "1.2.3.4",
			PodIPs: []corev1.PodIP{{IP: "1.2.3.4"}},
		},
	})
	now := time.Now()
	ke, err := StartKubeEnricher(clientset, func() time.Time {
		return now
	})
	require.NoError(t, err)
	defer ke.Close()

	require.NoError(t, ke.SendTemplate())

	// WHEN a Pod flow is captured
	require.NoError(t, ke.SendFlow("1.2.3.4"))

	// THEN the flow data is forwarded to Loki
	select {
	case flow := <-ke.LokiFlows:
		assert.Equal(t, "1.2.3.4", flow["SrcAddr"])
		assert.EqualValues(t, now.Unix(), flow["TimeFlowStart"])
		assert.EqualValues(t, now.Unix(), flow["TimeFlowEnd"])

		// AND it is decorated with the proper Pod metadata
		assert.Equal(t, "influxdb-v2", flow["Pod"])
		assert.Equal(t, "influxdb-v2", flow["Workload"])
		assert.Equal(t, "Pod", flow["WorkloadKind"])
		assert.Equal(t, "default", flow["Namespace"])
	case <-time.After(timeout):
		require.Fail(t, "timeout while waiting for flows")
	}

	// AND WHEN a Flow is captured from an unknown Kubernetes entity
	now = now.Add(2 * time.Second)
	require.NoError(t, ke.SendFlow("4.3.2.1"))
	select {
	case flow := <-ke.LokiFlows:
		// THEN the flow data is forwarded anyway
		assert.Equal(t, "4.3.2.1", flow["SrcAddr"])
		assert.EqualValues(t, now.Unix(), flow["TimeFlowStart"])
		assert.EqualValues(t, now.Unix(), flow["TimeFlowEnd"])

		// BUT without any metadata decoration
		assert.NotContains(t, flow, "Pod")
		assert.NotContains(t, flow, "Workload")
		assert.NotContains(t, flow, "WorkloadKind")
		assert.NotContains(t, flow, "Namespace")
	case <-time.After(timeout):
		require.Fail(t, "timeout while waiting for flows")
	}

}

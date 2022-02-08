// Package transform provides transformation tools for flow records
package transform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/flow"
	"github.com/netobserv/goflow2-kube-enricher/pkg/health"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var elog = logrus.WithField("module", "reader/Enricher")

var ownerNameFunc = func(owners interface{}, idx int) string {
	owner := owners.([]metav1.OwnerReference)[idx]
	return owner.Kind + "/" + owner.Name
}

type Enricher struct {
	Config    *config.Config
	Health    *health.Reporter
	Informers meta.InformersInterface
}

func (r *Enricher) Enrich(record flow.Record) flow.Record {
	if r.Config.PrintInput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}
	for ipField, prefixOut := range r.Config.IPFields {
		val, ok := record[ipField]
		if !ok {
			elog.Infof("Field %s not found in record", ipField)
			continue
		}
		ip, ok := val.(string)
		if !ok {
			elog.Warnf("String expected for field %s value %v", ipField, val)
			continue
		}
		if pod := r.Informers.PodByIP(ip); pod != nil {
			r.enrichPod(record, prefixOut, pod)
		} else {
			// If there is no Pod for such IP, we try searching for a service
			r.enrichService(ip, record, prefixOut)
		}
	}

	// Printing output before loki processing, because Loki will remove
	// indexed fields from the records hence making them hidden in output
	if r.Config.PrintOutput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}

	r.Health.RecordEnriched()

	return record
}

func (r *Enricher) enrichService(ip string, record map[string]interface{}, prefixOut string) {
	if svc := r.Informers.ServiceByIP(ip); svc != nil {
		fillWorkloadRecord(record, prefixOut, "Service", svc.Name, svc.Namespace)
	} else {
		elog.Warnf("Failed to get Service [ip=%v]", ip)
	}
}

func (r *Enricher) enrichPod(record map[string]interface{}, prefixOut string, pod *v1.Pod) {
	fillPodRecord(record, prefixOut, pod)
	var warnings []string
	if len(pod.OwnerReferences) > 0 {
		warnings = r.checkTooMany(warnings, "owners", "pod "+pod.Name, pod.OwnerReferences, len(pod.OwnerReferences), ownerNameFunc)
		ref := pod.OwnerReferences[0]
		if ref.Kind == "ReplicaSet" {
			// Search deeper (e.g. Deployment, DeploymentConfig)
			if rs := r.Informers.ReplicaSet(pod.Namespace, ref.Name); rs != nil {
				if len(rs.OwnerReferences) > 0 {
					warnings = r.checkTooMany(warnings, "owners", "replica "+rs.Name, rs.OwnerReferences, len(rs.OwnerReferences), ownerNameFunc)
					ref = rs.OwnerReferences[0]
				}
			} else {
				elog.Warnf("Failed to get ReplicaSet [ns=%s,name=%s]", pod.Namespace, ref.Name)
			}
		}
		fillWorkloadRecord(record, prefixOut, ref.Kind, ref.Name, "")
	} else {
		// Consider a pod without owner as self-owned
		fillWorkloadRecord(record, prefixOut, "Pod", pod.Name, "")
	}
	if len(warnings) > 0 {
		record[prefixOut+"Warn"] = strings.Join(warnings, "; ")
	}
}

func (r *Enricher) checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
	if size > 1 {
		var names []string
		for i := 0; i < size; i++ {
			names = append(names, nameFunc(items, i))
		}
		elog.Tracef("%d %s found for %s, using first", size, kind, ref)
		warn := fmt.Sprintf("Several %s found for %s: %s", kind, ref, strings.Join(names, ","))
		warnings = append(warnings, warn)
	}
	return warnings
}

func fillPodRecord(record map[string]interface{}, prefix string, pod *v1.Pod) {
	record[prefix+"Pod"] = pod.Name
	record[prefix+"Namespace"] = pod.Namespace
	record[prefix+"HostIP"] = pod.Status.HostIP
}

func fillWorkloadRecord(record map[string]interface{}, prefix, kind, name, ns string) {
	record[prefix+"Workload"] = name
	record[prefix+"WorkloadKind"] = kind
	if ns != "" {
		record[prefix+"Namespace"] = ns
	}
}

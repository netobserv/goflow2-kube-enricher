// Package enricher proceeds with kube enrichment
package enricher

import (
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
	"github.com/sirupsen/logrus"
)

type Enricher struct {
	log       *logrus.Entry
	informers meta.InformersInterface
	config    *config.Config
	exporters *export.Exporters
}

func NewEnricher(log *logrus.Entry, cfg *config.Config, clientset *kubernetes.Clientset, exporters *export.Exporters) Enricher {
	informers := meta.NewInformers(clientset)

	stopCh := make(chan struct{})
	defer close(stopCh)
	if err := informers.Start(stopCh); err != nil {
		log.WithError(err).Fatal("can't start informers")
	}
	log.Info("waiting for informers to be synchronized")
	informers.WaitForCacheSync(stopCh)
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		informers.DebugInfo(log.Writer())
	}

	return Enricher{
		log:       log,
		informers: &informers,
		config:    cfg,
		exporters: exporters,
	}
}

var ownerNameFunc = func(owners interface{}, idx int) string {
	owner := owners.([]metav1.OwnerReference)[idx]
	return owner.Kind + "/" + owner.Name
}

func (e *Enricher) Enrich(record map[string]interface{}) error {
	if e.config.PrintInput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}

	if e.config.Enrich {
		for ipField, prefixOut := range e.config.IPFields {
			val, ok := record[ipField]
			if !ok {
				e.log.Infof("Field %s not found in record", ipField)
				continue
			}
			ip, ok := val.(string)
			if !ok {
				e.log.Warnf("String expected for field %s value %v", ipField, val)
				continue
			}
			if pod := e.informers.PodByIP(ip); pod != nil {
				e.enrichPod(record, prefixOut, pod)
			} else {
				// If there is no Pod for such IP, we try searching for a service
				e.enrichService(ip, record, prefixOut)
			}
		}
	}

	// Printing output before loki processing, because Loki will remove
	// indexed fields from the records hence making them hidden in output
	if e.config.PrintOutput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}

	var err error
	if e.exporters != nil {
		err = e.exporters.ProcessRecord(record)
	} else {
		e.log.Debugf("can't process record. exporters is nil !")
	}

	return err
}

func (e *Enricher) enrichService(ip string, record map[string]interface{}, prefixOut string) {
	if svc := e.informers.ServiceByIP(ip); svc != nil {
		fillWorkloadRecord(record, prefixOut, "Service", svc.Name, svc.Namespace)
	} else {
		e.log.Warnf("Failed to get Service [ip=%v]", ip)
	}
}

func (e *Enricher) enrichPod(record map[string]interface{}, prefixOut string, pod *v1.Pod) {
	fillPodRecord(record, prefixOut, pod)
	var warnings []string
	if len(pod.OwnerReferences) > 0 {
		warnings = e.checkTooMany(warnings, "owners", "pod "+pod.Name, pod.OwnerReferences, len(pod.OwnerReferences), ownerNameFunc)
		ref := pod.OwnerReferences[0]
		if ref.Kind == "ReplicaSet" {
			// Search deeper (e.g. Deployment, DeploymentConfig)
			if rs := e.informers.ReplicaSet(pod.Namespace, ref.Name); rs != nil {
				if len(rs.OwnerReferences) > 0 {
					warnings = e.checkTooMany(warnings, "owners", "replica "+rs.Name, rs.OwnerReferences, len(rs.OwnerReferences), ownerNameFunc)
					ref = rs.OwnerReferences[0]
				}
			} else {
				e.log.Warnf("Failed to get ReplicaSet [ns=%s,name=%s]", pod.Namespace, ref.Name)
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

func (e *Enricher) checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
	if size > 1 {
		var names []string
		for i := 0; i < size; i++ {
			names = append(names, nameFunc(items, i))
		}
		e.log.Tracef("%d %s found for %s, using first", size, kind, ref)
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

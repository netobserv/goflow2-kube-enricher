package reader

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Reader struct {
	log       *logrus.Entry
	informers meta.InformersInterface
	mapping   []FieldMapping
	format    format.Format
}

func NewReader(format format.Format, log *logrus.Entry, mapping []FieldMapping, clientset *kubernetes.Clientset) Reader {
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

	return Reader{
		log:       log,
		informers: &informers,
		mapping:   mapping,
		format:    format,
	}
}

func (r *Reader) Start(loki export.Loki) {
	for {
		record, err := r.format.Next()
		if err != nil {
			r.log.Error(err)
			return
		}
		if record == nil {
			return
		}
		_, err = r.enrich(record, loki)
		if err != nil {
			r.log.Error(err)
		}
	}
}

type FieldMapping struct {
	FieldName string
	PrefixOut string
}

var ownerNameFunc = func(owners interface{}, idx int) string {
	owner := owners.([]metav1.OwnerReference)[idx]
	return owner.Kind + "/" + owner.Name
}

func (r *Reader) enrich(record map[string]interface{}, loki export.Loki) ([]byte, error) {
	for _, fieldMap := range r.mapping {
		val, ok := record[fieldMap.FieldName]
		if !ok {
			r.log.Infof("Field %s not found in record", fieldMap.FieldName)
			continue
		}
		ip, ok := val.(string)
		if !ok {
			r.log.Warnf("String expected for field %s value %v", fieldMap.FieldName, val)
			continue
		}
		if pod := r.informers.PodByIP(ip); pod != nil {
			r.enrichPod(record, fieldMap, pod)
		} else {
			// If there is no Pod for such IP, we try searching for a service
			r.enrichService(ip, record, fieldMap)
		}
	}

	if err := loki.ProcessJsonRecord(record); err != nil {
		r.log.Error(err)
	}

	return json.Marshal(record)
}

func (r *Reader) enrichService(ip string, record map[string]interface{}, fieldMap FieldMapping) {
	if svc := r.informers.ServiceByIP(ip); svc != nil {
		fillWorkloadRecord(record, fieldMap.PrefixOut, "Service", svc.Name, svc.Namespace)
	} else {
		r.log.Warnf("Failed to get Service [ip=%v]", ip)
	}
}

func (r *Reader) enrichPod(record map[string]interface{}, fieldMap FieldMapping, pod *v1.Pod) {
	fillPodRecord(record, fieldMap.PrefixOut, pod)
	var warnings []string
	if len(pod.OwnerReferences) > 0 {
		warnings = r.checkTooMany(warnings, "owners", "pod "+pod.Name, pod.OwnerReferences, len(pod.OwnerReferences), ownerNameFunc)
		ref := pod.OwnerReferences[0]
		if ref.Kind == "ReplicaSet" {
			// Search deeper (e.g. Deployment, DeploymentConfig)
			if rs := r.informers.ReplicaSet(pod.Namespace, ref.Name); rs != nil {
				if len(rs.OwnerReferences) > 0 {
					warnings = r.checkTooMany(warnings, "owners", "replica "+rs.Name, rs.OwnerReferences, len(rs.OwnerReferences), ownerNameFunc)
					ref = rs.OwnerReferences[0]
				}
			} else {
				r.log.Warnf("Failed to get ReplicaSet [ns=%s,name=%s]", pod.Namespace, ref.Name)
			}
		}
		fillWorkloadRecord(record, fieldMap.PrefixOut, ref.Kind, ref.Name, "")
	} else {
		// Consider a pod without owner as self-owned
		fillWorkloadRecord(record, fieldMap.PrefixOut, "Pod", pod.Name, "")
	}
	if len(warnings) > 0 {
		record[fieldMap.PrefixOut+"Warn"] = strings.Join(warnings, "; ")
	}
}

func (r *Reader) checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
	if size > 1 {
		var names []string
		for i := 0; i < size; i++ {
			names = append(names, nameFunc(items, i))
		}
		r.log.Tracef("%d %s found for %s, using first", size, kind, ref)
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

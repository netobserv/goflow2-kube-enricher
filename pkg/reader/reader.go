// Package reader reads input flows and proceeds with kube enrichment
package reader

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	"github.com/netobserv/goflow2-kube-enricher/pkg/meta"
)

var log = logrus.WithField("component", "reader.Reader")

type lokiProcessor interface {
	ProcessRecord(record map[string]interface{}) error
}

type Reader struct {
	informers meta.InformersInterface
	config    *config.Config
	format    format.Format
	loki      lokiProcessor
}

func NewReader(config *config.Config, format format.Format, informers meta.InformersInterface) (Reader, error) {
	loki, err := export.NewLoki(&config.Loki)
	if err != nil {
		return Reader{}, nil
	}
	return Reader{
		format:    format,
		informers: informers,
		config:    config,
		loki:      &loki,
	}, err
}

func (r *Reader) Start(ctx context.Context) {
	// Start listening on input flows (in background)
	go r.format.Start(ctx)
	for {
		select {
		case <-ctx.Done():
			r.format.Shutdown()
			return
		default:
			record, err := r.format.Next()
			if err != nil {
				log.Error(err)
				return
			}
			if record == nil {
				log.Error("nil record")
				return
			}
			r.enrich(record)
			if err := r.loki.ProcessRecord(record); err != nil {
				log.Error(err)
			}
		}
	}
}

var ownerNameFunc = func(owners interface{}, idx int) string {
	owner := owners.([]metav1.OwnerReference)[idx]
	return owner.Kind + "/" + owner.Name
}

func (r *Reader) enrich(record map[string]interface{}) {
	if r.config.PrintInput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}
	for ipField, prefixOut := range r.config.IPFields {
		val, ok := record[ipField]
		if !ok {
			log.Infof("Field %s not found in record", ipField)
			continue
		}
		ip, ok := val.(string)
		if !ok {
			log.Warnf("String expected for field %s value %v", ipField, val)
			continue
		}
		if pod := r.informers.PodByIP(ip); pod != nil {
			r.enrichPod(record, prefixOut, pod)
		} else {
			// If there is no Pod for such IP, we try searching for a service
			r.enrichService(ip, record, prefixOut)
		}
	}

	// Printing output before loki processing, because Loki will remove
	// indexed fields from the records hence making them hidden in output
	if r.config.PrintOutput {
		bs, _ := json.Marshal(record)
		fmt.Println(string(bs))
	}
}

func (r *Reader) enrichService(ip string, record map[string]interface{}, prefixOut string) {
	if svc := r.informers.ServiceByIP(ip); svc != nil {
		fillWorkloadRecord(record, prefixOut, "Service", svc.Name, svc.Namespace)
	} else {
		log.Warnf("Failed to get Service [ip=%v]", ip)
	}
}

func (r *Reader) enrichPod(record map[string]interface{}, prefixOut string, pod *v1.Pod) {
	fillPodRecord(record, prefixOut, pod)
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
				log.Warnf("Failed to get ReplicaSet [ns=%s,name=%s]", pod.Namespace, ref.Name)
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

func (r *Reader) checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
	if size > 1 {
		var names []string
		for i := 0; i < size; i++ {
			names = append(names, nameFunc(items, i))
		}
		log.Tracef("%d %s found for %s, using first", size, kind, ref)
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

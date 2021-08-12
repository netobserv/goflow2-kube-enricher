package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	version       = "unknown"
	app           = "kube-enricher"
	fieldsMapping = flag.String("mapping", "SrcAddr=Src,DstAddr=Dst", "Mapping of fields containing IPs to prefixes for new fields")
	logLevel      = flag.String("loglevel", "info", "Log level")
	versionFlag   = flag.Bool("v", false, "Print version")
	log           = logrus.WithField("module", app)
	appVersion    = fmt.Sprintf("%s %s", app, version)
)

func init() {
}

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Println(appVersion)
		os.Exit(0)
	}

	lvl, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		log.Errorf("Log level %s not recognized, using info", *logLevel)
		*logLevel = "info"
		lvl = logrus.InfoLevel
	}
	logrus.SetLevel(lvl)

	mapping, err := parseFieldMapping(*fieldsMapping)
	if err != nil {
		log.Fatal(err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("Starting %s at log level %s", appVersion, *logLevel)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		in := scanner.Bytes()
		enriched, err := enrich(clientset, in, mapping)
		if err != nil {
			log.Error(err)
			fmt.Println(string(in))
		} else {
			fmt.Println(string(enriched))
		}
	}

	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

type fieldMapping struct {
	fieldName string
	prefixOut string
}

func parseFieldMapping(in string) ([]fieldMapping, error) {
	mapping := []fieldMapping{}
	fields := strings.Split(in, ",")
	for _, pair := range fields {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return mapping, fmt.Errorf("Invalid fields mapping pair '%s' in '%s'", pair, in)
		}
		mapping = append(mapping, fieldMapping{
			fieldName: kv[0],
			prefixOut: kv[1],
		})
	}
	return mapping, nil
}

var podNameFunc = func(pods interface{}, idx int) string {
	pod := pods.([]v1.Pod)[idx]
	return pod.Name + "." + pod.Namespace
}
var ownerNameFunc = func(owners interface{}, idx int) string {
	owner := owners.([]metav1.OwnerReference)[idx]
	return owner.Kind + "/" + owner.Name
}
var svcNameFunc = func(services interface{}, idx int) string {
	svc := services.([]v1.Service)[idx]
	return svc.Name + "." + svc.Namespace
}

func enrich(clientset *kubernetes.Clientset, rawRecord []byte, mapping []fieldMapping) ([]byte, error) {
	// TODO: allow protobuf input
	var record map[string]interface{}
	err := json.Unmarshal(rawRecord, &record)
	if err != nil {
		return nil, err
	}

	// TODO: use cache
	for _, fieldMap := range mapping {
		addWarnings := []string{}

		if val, ok := record[fieldMap.fieldName]; ok {
			if ip, ok := val.(string); ok {
				pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{FieldSelector: "status.podIP=" + ip})
				if err != nil {
					log.Warnf("Failed to get pod [ip=%s,err=%v]", ip, err)
				} else if len(pods.Items) > 0 {
					addWarnings = checkTooMany(addWarnings, "pods", "same IP", pods.Items, len(pods.Items), podNameFunc)
					pod := pods.Items[0]
					record[fieldMap.prefixOut+"Pod"] = pod.Name
					record[fieldMap.prefixOut+"Namespace"] = pod.Namespace
					record[fieldMap.prefixOut+"HostIP"] = pod.Status.HostIP
					if len(pod.OwnerReferences) > 0 {
						addWarnings = checkTooMany(addWarnings, "owners", "pod "+pod.Name, pod.OwnerReferences, len(pod.OwnerReferences), ownerNameFunc)
						ref := pod.OwnerReferences[0]
						if ref.Kind == "ReplicaSet" {
							// Search deeper (e.g. Deployment, DeploymentConfig)
							rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
							if err != nil {
								log.Warnf("Failed to get replicaset [ns=%s,name=%s,err=%v]", pod.Namespace, ref.Name, err)
							} else if len(rs.OwnerReferences) > 0 {
								addWarnings = checkTooMany(addWarnings, "owners", "replica "+rs.Name, rs.OwnerReferences, len(rs.OwnerReferences), ownerNameFunc)
								ref = rs.OwnerReferences[0]
							}
						}
						record[fieldMap.prefixOut+"Workload"] = ref.Name
						record[fieldMap.prefixOut+"WorkloadKind"] = ref.Kind
					} else {
						// Consider a pod without owner as self-owned
						record[fieldMap.prefixOut+"Workload"] = pod.Name
						record[fieldMap.prefixOut+"WorkloadKind"] = "Pod"
					}
				} else {
					// Try service
					services, err := findServicesByIP(clientset, ip)
					if err != nil {
						log.Warnf("Failed to get services [err=%v]", err)
					} else if len(services) > 0 {
						addWarnings = checkTooMany(addWarnings, "services", "same IP", services, len(services), svcNameFunc)
						svc := services[0]
						record[fieldMap.prefixOut+"Workload"] = svc.Name
						record[fieldMap.prefixOut+"WorkloadKind"] = "Service"
						record[fieldMap.prefixOut+"Namespace"] = svc.Namespace
					} else {
						log.Tracef("No pods or services found for IP %s", ip)
					}
				}
			} else {
				log.Warnf("String expected for field %s value %v", fieldMap.fieldName, val)
			}
		} else {
			log.Infof("Field %s not found in record", fieldMap.fieldName)
		}

		if len(addWarnings) > 0 {
			record[fieldMap.prefixOut+"Warn"] = strings.Join(addWarnings, "; ")
		}
	}

	return json.Marshal(record)
}

func checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
	if size > 1 {
		names := []string{}
		for i := 0; i < size; i++ {
			names = append(names, nameFunc(items, i))
		}
		log.Tracef("%d %s found for %s, using first", size, kind, ref)
		warn := fmt.Sprintf("Several %s found for %s: %s", kind, ref, strings.Join(names, ","))
		warnings = append(warnings, warn)
	}
	return warnings
}

func findServicesByIP(clientset *kubernetes.Clientset, ip string) ([]v1.Service, error) {
	services, err := clientset.CoreV1().Services("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return []v1.Service{}, err
	}
	filtered := []v1.Service{}
	for _, svc := range services.Items {
		if svc.Spec.ClusterIP == ip {
			filtered = append(filtered, svc)
		}
	}
	return filtered, nil
}

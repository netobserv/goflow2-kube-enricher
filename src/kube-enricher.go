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
					if len(pods.Items) > 1 {
						names := []string{}
						for _, pod := range pods.Items {
							names = append(names, pod.Name+"."+pod.Namespace)
						}
						addWarnings = append(addWarnings, "Several pods found with same IP: "+strings.Join(names, ","))
						log.Tracef("%d pods found for IP %s, using first", len(pods.Items), ip)
					}
					pod := pods.Items[0]
					record[fieldMap.prefixOut+"Pod"] = pod.Name
					record[fieldMap.prefixOut+"Namespace"] = pod.Namespace
					record[fieldMap.prefixOut+"HostIP"] = pod.Status.HostIP
					if len(pod.OwnerReferences) > 0 {
						if len(pod.OwnerReferences) > 1 {
							names := []string{}
							for _, ref := range pod.OwnerReferences {
								names = append(names, ref.Kind+"/"+ref.Name)
							}
							addWarnings = append(addWarnings, "Several owners for pod: "+strings.Join(names, ","))
							log.Tracef("%d owners found for pod %s, using first", len(pod.OwnerReferences), pod.Name)
						}
						ref := pod.OwnerReferences[0]
						if ref.Kind == "ReplicaSet" {
							// Search deeper (e.g. Deployment, DeploymentConfig)
							rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
							if err != nil {
								log.Warnf("Failed to get replicaset [ns=%s,name=%s,err=%v]", pod.Namespace, ref.Name, err)
							} else if len(rs.OwnerReferences) > 0 {
								if len(rs.OwnerReferences) > 1 {
									names := []string{}
									for _, rsRef := range rs.OwnerReferences {
										names = append(names, rsRef.Kind+"/"+rsRef.Name)
									}
									addWarnings = append(addWarnings, "Several owners for replica: "+strings.Join(names, ","))
									log.Tracef("%d owners found for replica %s, using first", len(rs.OwnerReferences), rs.Name)
								}
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
					log.Tracef("No pods found for IP %s", ip)
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

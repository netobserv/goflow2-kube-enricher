package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/jotak/goflow2-kube-enricher/meta"

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
	kubeconfig    = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
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

	clientset, err := kubernetes.NewForConfig(loadConfig())
	if err != nil {
		log.Fatal(err)
	}
	informers := meta.NewInformers(clientset)
	log.Infof("Starting %s at log level %s", appVersion, *logLevel)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		in := scanner.Bytes()
		enriched, err := enrich(informers, in, mapping)
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

// loadConfig fetches a given kubernetes configuration in the following order
// 1. path provided by the -kubeconfig CLI argument
// 2. path provided by the KUBECONFIG environment variable
// 3. REST InClusterConfig
func loadConfig() *rest.Config {
	var config *rest.Config
	var err error
	if kubeconfig != nil && *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			log.WithError(err).WithField("kubeconfig", *kubeconfig).
				Fatal("can't find provided kubeconfig param path")
		}
	} else if kfgPath := os.Getenv("KUBECONFIG"); kfgPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kfgPath)
		if err != nil {
			log.WithError(err).WithField("kubeconfig", kfgPath).
				Fatal("can't find provided KUBECONFIG env path")
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			log.WithError(err).Fatal("can't load in-cluster REST config")
		}
	}
	return config
}

type fieldMapping struct {
	fieldName string
	prefixOut string
}

func parseFieldMapping(in string) ([]fieldMapping, error) {
	var mapping []fieldMapping
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

func enrich(informers meta.Informers, rawRecord []byte, mapping []fieldMapping) ([]byte, error) {
	// TODO: allow protobuf input
	var record map[string]interface{}
	err := json.Unmarshal(rawRecord, &record)
	if err != nil {
		return nil, err
	}

	for _, fieldMap := range mapping {
		val, ok := record[fieldMap.fieldName]
		if !ok {
			log.Infof("Field %s not found in record", fieldMap.fieldName)
			continue
		}
		ip, ok := val.(string)
		if !ok {
			log.Warnf("String expected for field %s value %v", fieldMap.fieldName, val)
			continue
		}
		if pod, ok := informers.PodByIP(ip); ok {
			enrichPod(informers, record, fieldMap, pod)
		} else {
			// If there is no Pod for such IP, we try searching for a service
			enrichService(informers, ip, record, fieldMap)
		}
	}

	return json.Marshal(record)
}

func enrichService(informers meta.Informers, ip string, record map[string]interface{}, fieldMap fieldMapping) {
	svc, ok := informers.ServiceByIP(ip)
	if !ok {
		log.Warnf("Failed to get Service [ip=%v]", ip)
	} else {
		record[fieldMap.prefixOut+"Workload"] = svc.Name
		record[fieldMap.prefixOut+"WorkloadKind"] = "Service"
		record[fieldMap.prefixOut+"Namespace"] = svc.Namespace
	}
}

func enrichPod(informers meta.Informers, record map[string]interface{}, fieldMap fieldMapping, pod *v1.Pod) {
	var warnings []string
	record[fieldMap.prefixOut+"Pod"] = pod.Name
	record[fieldMap.prefixOut+"Namespace"] = pod.Namespace
	record[fieldMap.prefixOut+"HostIP"] = pod.Status.HostIP
	if len(pod.OwnerReferences) > 0 {
		warnings = checkTooMany(warnings, "owners", "pod "+pod.Name, pod.OwnerReferences, len(pod.OwnerReferences), ownerNameFunc)
		ref := pod.OwnerReferences[0]
		if ref.Kind == "ReplicaSet" {
			// Search deeper (e.g. Deployment, DeploymentConfig)
			rs, ok := informers.ReplicaSet(pod.Namespace, ref.Name)
			if !ok {
				log.Warnf("Failed to get ReplicaSet [ns=%s,name=%s]", pod.Namespace, ref.Name)
			} else if len(rs.OwnerReferences) > 0 {
				warnings = checkTooMany(warnings, "owners", "replica "+rs.Name, rs.OwnerReferences, len(rs.OwnerReferences), ownerNameFunc)
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
	if len(warnings) > 0 {
		record[fieldMap.prefixOut+"Warn"] = strings.Join(warnings, "; ")
	}
}

func checkTooMany(warnings []string, kind, ref string, items interface{}, size int, nameFunc func(interface{}, int) string) []string {
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

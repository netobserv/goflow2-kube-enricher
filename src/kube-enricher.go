package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	version       = ""
	buildinfos    = ""
	AppVersion    = "kube-enricher " + version + " " + buildinfos
	FieldsMapping = flag.String("mapping", "SrcAddr=Src,DstAddr=Dst", "Mapping of fields containing IPs to prefixes for new fields")
	LogLevel      = flag.String("loglevel", "info", "Log level")
	Version       = flag.Bool("v", false, "Print version")
)

func init() {
}

func main() {
	flag.Parse()

	if *Version {
		fmt.Println(AppVersion)
		os.Exit(0)
	}

	lvl, _ := log.ParseLevel(*LogLevel)
	log.SetLevel(lvl)
	mapping, err := parseFieldMapping(*FieldsMapping)
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

	log.Info("Starting kube-enricher")

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
		if val, ok := record[fieldMap.fieldName]; ok {
			if ip, ok := val.(string); ok {
				pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{FieldSelector: "status.podIP=" + ip})
				if err != nil {
					log.Warnf("Could not find pod with IP %s: %v", ip, err)
				}
				if len(pods.Items) > 0 {
					pod := pods.Items[0]
					record[fieldMap.prefixOut+"Pod"] = pod.Name
					record[fieldMap.prefixOut+"Namespace"] = pod.Namespace
					record[fieldMap.prefixOut+"HostIP"] = pod.Status.HostIP
					if len(pods.Items) > 1 {
						log.Infof("%d pods found for IP %s, using first", len(pods.Items), ip)
					}
				} else {
					log.Infof("No pods found for IP %s", ip)
				}
			} else {
				log.Warnf("String expected for field %s value %v", fieldMap.fieldName, val)
			}
		} else {
			log.Warnf("Field %s not found in record", fieldMap.fieldName)
		}
	}

	return json.Marshal(record)
}

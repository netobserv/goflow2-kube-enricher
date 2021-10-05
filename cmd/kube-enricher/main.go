package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	jsonFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/json"
	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	"github.com/netobserv/goflow2-kube-enricher/pkg/reader"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

var (
	version       = "unknown"
	app           = "kube-enricher"
	fieldsMapping = flag.String("mapping", "SrcAddr=Src,DstAddr=Dst", "Mapping of fields containing IPs to prefixes for new fields")
	kubeConfig    = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	logLevel      = flag.String("loglevel", "info", "Log level")
	versionFlag   = flag.Bool("v", false, "Print version")
	log           = logrus.WithField("module", app)
	appVersion    = fmt.Sprintf("%s %s", app, version)
	sourceFormat  = flag.String("sourceformat", "json", "format of the input string")
)

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
	log.Infof("Starting %s at log level %s", appVersion, *logLevel)

	mapping, err := parseFieldMapping(*fieldsMapping)
	if err != nil {
		log.Fatal(err)
	}

	var in format.Format
	switch *sourceFormat {
	case "json":
		in = jsonFormat.NewScanner(os.Stdin)
	case "pb":
		in = pbFormat.NewScanner(os.Stdin)
	default:
		log.Fatal("Unknown source format: ", sourceFormat)
	}

	clientset, err := kubernetes.NewForConfig(loadKubeConfig())
	if err != nil {
		log.Fatal(err)
	}

	r := reader.NewReader(in, log, mapping, clientset)
	log.Info("Starting reader...")
	r.Start()
}

// loadKubeConfig fetches a given kubernetes configuration in the following order
// 1. path provided by the -kubeConfig CLI argument
// 2. path provided by the KUBECONFIG environment variable
// 3. REST InClusterConfig
func loadKubeConfig() *rest.Config {
	var config *rest.Config
	var err error
	if kubeConfig != nil && *kubeConfig != "" {
		log.Info("Using command line supplied kube config")
		config, err = clientcmd.BuildConfigFromFlags("", *kubeConfig)
		if err != nil {
			log.WithError(err).WithField("kubeConfig", *kubeConfig).
				Fatal("can't find provided kubeConfig param path")
		}
	} else if kfgPath := os.Getenv("KUBECONFIG"); kfgPath != "" {
		log.Info("Using environment KUBECONFIG")
		config, err = clientcmd.BuildConfigFromFlags("", kfgPath)
		if err != nil {
			log.WithError(err).WithField("kubeConfig", kfgPath).
				Fatal("can't find provided KUBECONFIG env path")
		}
	} else {
		log.Info("Using in-cluster kube config")
		config, err = rest.InClusterConfig()
		if err != nil {
			log.WithError(err).Fatal("can't load in-cluster REST config")
		}
	}
	return config
}

func parseFieldMapping(in string) ([]reader.FieldMapping, error) {
	var mapping []reader.FieldMapping
	fields := strings.Split(in, ",")
	for _, pair := range fields {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return mapping, fmt.Errorf("Invalid fields mapping pair '%s' in '%s'", pair, in)
		}
		mapping = append(mapping, reader.FieldMapping{
			FieldName: kv[0],
			PrefixOut: kv[1],
		})
	}
	return mapping, nil
}

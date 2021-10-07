package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	jsonFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/json"
	nfFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/netflow"
	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	"github.com/netobserv/goflow2-kube-enricher/pkg/reader"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

var (
	version           = "unknown"
	app               = "kube-enricher"
	listenAddress     = flag.String("listen", "", "listen address, if empty, will listen to stdin")
	fieldsMapping     = flag.String("mapping", "SrcAddr=Src,DstAddr=Dst", "Mapping of fields containing IPs to prefixes for new fields")
	kubeConfig        = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	logLevel          = flag.String("loglevel", "info", "Log level")
	versionFlag       = flag.Bool("v", false, "Print version")
	log               = logrus.WithField("module", app)
	appVersion        = fmt.Sprintf("%s %s", app, version)
	stdinSourceFormat = flag.String("stdinsourceformat", "json", "format of the input string")
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
	if *listenAddress == "" {
		switch *stdinSourceFormat {
		case "json":
			in = jsonFormat.NewScanner(os.Stdin)
		case "pb":
			in = pbFormat.NewScanner(os.Stdin)
		default:
			log.Fatal("Unknown source format: ", stdinSourceFormat)
		}
	} else {
		listenAddrUrl, err := url.Parse(*listenAddress)
		if err != nil {
			log.Fatal(err)
		}
		if listenAddrUrl.Scheme == "netflow" {
			hostname := listenAddrUrl.Hostname()
			port, err := strconv.ParseUint(listenAddrUrl.Port(), 10, 64)
			if err != nil {
				log.Fatal("Failed reading listening port: ", err)
			}
			in = nfFormat.NewDriver(hostname, int(port))
		} else {
			log.Fatal("Unknown listening protocol")
		}
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

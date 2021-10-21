package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netobserv/goflow2-kube-enricher/pkg/config"
	"github.com/netobserv/goflow2-kube-enricher/pkg/export"
	"github.com/netobserv/goflow2-kube-enricher/pkg/format"
	jsonFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/json"
	nfFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/netflow"
	pbFormat "github.com/netobserv/goflow2-kube-enricher/pkg/format/pb"
	"github.com/netobserv/goflow2-kube-enricher/pkg/reader"
)

const netflowScheme = "netflow"

var (
	version        = "unknown"
	app            = "goflow-kube"
	mainConfigPath = flag.String("config", "", "absolute path to the main configuration file")
	kubeConfigPath = flag.String("kubeconfig", "", "absolute path to a kubeconfig file for advanced kube client configuration")
	logLevel       = flag.String("loglevel", "info", "log level")
	versionFlag    = flag.Bool("v", false, "print version")
	log            = logrus.WithField("module", app)
	appVersion     = fmt.Sprintf("%s %s", app, version)
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

	cfg := loadMainConfig()
	log.Info("Creating loki...")
	loki, err := export.NewLoki(&cfg.Loki)
	if err != nil {
		log.WithError(err).Fatal("Can't load Loki exporter")
	}

	var in format.Format
	if cfg.Listen == "" {
		switch cfg.StdinFormat {
		case config.JSONFlagName:
			in = jsonFormat.NewScanner(os.Stdin)
		case config.PBFlagName:
			in = pbFormat.NewScanner(os.Stdin)
		default:
			log.Fatal("Unknown source format: ", cfg.StdinFormat)
		}
	} else {
		listenAddrURL, err := url.Parse(cfg.Listen)
		if err != nil {
			log.Fatal(err)
		}
		if listenAddrURL.Scheme == netflowScheme {
			hostname := listenAddrURL.Hostname()
			port, err := strconv.ParseUint(listenAddrURL.Port(), 10, 64)
			if err != nil {
				log.Fatal("Failed reading listening port: ", err)
			}
			log.Infof("Start listening on %s", cfg.Listen)
			ctx := context.Background()
			in = nfFormat.StartDriver(ctx, hostname, int(port))
		} else {
			log.Fatal("Unknown listening protocol")
		}
	}

	clientset, err := kubernetes.NewForConfig(loadKubeConfig())
	if err != nil {
		log.Fatal(err)
	}

	r := reader.NewReader(in, log, cfg, clientset)
	log.Info("Starting reader...")
	r.Start(&loki)
}

// loadKubeConfig fetches a given kubernetes configuration in the following order
// 1. path provided by the -kubeConfig CLI argument
// 2. path provided by the KUBECONFIG environment variable
// 3. REST InClusterConfig
func loadKubeConfig() *rest.Config {
	var config *rest.Config
	var err error
	if kubeConfigPath != nil && *kubeConfigPath != "" {
		flog := log.WithField("kubeConfig", *kubeConfigPath)
		flog.Info("Using command line supplied kube config")
		config, err = clientcmd.BuildConfigFromFlags("", *kubeConfigPath)
		if err != nil {
			flog.WithError(err).Fatal("Can't load kube config file")
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

// loadMainConfig fetches a given configuration in the following order
// 1. path provided by the -config CLI argument
// 2. path provided by the CONFIG environment variable
// 3. default configuration
func loadMainConfig() *config.Config {
	var cfg *config.Config
	var err error
	if mainConfigPath != nil && *mainConfigPath != "" {
		flog := log.WithField("configFile", *mainConfigPath)
		flog.Info("Using command line supplied loki config")
		cfg, err = config.Load(*mainConfigPath)
		if err != nil {
			flog.WithError(err).Fatal("Can't load loki config file")
		}
	} else if lfgPath := os.Getenv("CONFIG"); lfgPath != "" {
		log.Info("Using environment CONFIG")
		cfg, err = config.Load(lfgPath)
		if err != nil {
			log.WithError(err).WithField("config", lfgPath).
				Fatal("can't find provided CONFIG env path")
		}
	} else {
		log.Info("Using default configuration")
		cfg = config.Default()
	}
	return cfg
}

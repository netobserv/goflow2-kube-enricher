module github.com/netobserv/goflow2-kube-enricher

go 1.15

require (
	github.com/frankban/quicktest v1.14.0 // indirect
	github.com/go-kit/kit v0.12.0
	github.com/json-iterator/go v1.1.12
	github.com/mitchellh/mapstructure v1.4.2
	github.com/netobserv/loki-client-go v0.0.0-20211018150932-cb17208397a9
	github.com/netsampler/goflow2 v1.0.5-0.20220106210010-20e8e567090c
	github.com/prometheus/common v0.31.1
	github.com/segmentio/kafka-go v0.4.27
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20220105145211-5b0dc2dfae98 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.5
	k8s.io/apimachinery v0.21.5
	k8s.io/client-go v0.21.5
)

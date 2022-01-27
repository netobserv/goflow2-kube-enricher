module github.com/netobserv/goflow2-kube-enricher

go 1.15

require (
	github.com/go-kit/kit v0.12.0
	github.com/golang/snappy v0.0.4
	github.com/json-iterator/go v1.1.12
	github.com/mitchellh/mapstructure v1.4.2
	github.com/netobserv/loki-client-go v0.0.0-20211018150932-cb17208397a9
	github.com/netsampler/goflow2 v1.0.5-0.20220106210010-20e8e567090c
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.31.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/vmware/go-ipfix v0.5.11
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.5
	k8s.io/apimachinery v0.21.5
	k8s.io/client-go v0.21.5
)

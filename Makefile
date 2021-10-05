VERSION ?= dev
IMAGE ?= quay.io/netobserv/goflow2-kube

ifeq (,$(shell which podman 2>/dev/null))
OCI_BIN ?= docker
else
OCI_BIN ?= podman
endif

all: fmt build lint test

fmt:
	go fmt ./...

lint:
	golangci-lint run

test:
	go test ./...

build:
	go build -o kube-enricher cmd/kube-enricher/main.go

image:
	$(OCI_BIN) build --build-arg VERSION="$(VERSION)" -t $(IMAGE):$(VERSION) .

push:
	$(OCI_BIN) push $(IMAGE):$(VERSION)

ovnk-deploy:
	sed -e 's/goflow2-kube:dev/goflow2-kube:$(VERSION)/' ./examples/goflow-kube.yaml | kubectl apply -f - && \
	GF_IP=`kubectl get svc goflow -ojsonpath='{.spec.clusterIP}'` && echo "Goflow IP: $$GF_IP" && \
	kubectl set env daemonset/ovnkube-node -c ovnkube-node -n ovn-kubernetes OVN_IPFIX_TARGETS="$$GF_IP:2055"

cno-deploy:
	sed -e 's/goflow2-kube:dev/goflow2-kube:$(VERSION)/' ./examples/goflow-kube.yaml | kubectl apply -f - && \
	GF_IP=`oc get svc goflow -ojsonpath='{.spec.clusterIP}'` && "Goflow IP: $$GF_IP" && \
	oc patch networks.operator.openshift.io cluster --type='json' -p "$(sed -e "s/GF_IP/$$GF_IP/" examples/net-cluster-patch.json)"

kill:
	kubectl delete pod -l app=goflow

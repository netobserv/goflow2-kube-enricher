VERSION ?= dev
IMAGE ?= quay.io/netobserv/goflow2-kube
GOLANGCI_LINT_VERSION ?= v1.42.1
COVERPROFILE ?= coverage.out

ifeq (,$(shell which podman 2>/dev/null))
OCI_BIN ?= docker
else
OCI_BIN ?= podman
endif

all: fmt build lint test

fmt:
	go fmt ./...

prereqs:
	@echo "### Test if prerequisites are met, and installing missing dependencies"
	test -f $(go env GOPATH)/bin/golangci-lint || go install github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}
	test -f $(go env GOPATH)/bin/staticcheck || go install honnef.co/go/tools/cmd/staticcheck@latest

lint: prereqs
	@echo "### Linting code"
	# staticcheck does not work properly when invoked inside golangci-lint
	staticcheck -f stylish ./...
	golangci-lint run ./...

verify: 
	lint test

.PHONY: 
	prereqs lint image lint test verify

test:
	go test ./... -coverprofile ${COVERPROFILE}

build:
	go build -o kube-enricher cmd/kube-enricher/main.go

image:
	@echo "### Building container with ${OCI_BIN}"
	$(OCI_BIN) build --build-arg VERSION="$(VERSION)" -t $(IMAGE):$(VERSION) .

push:
	$(OCI_BIN) push $(IMAGE):$(VERSION)

ovnk-deploy:
	sed -e 's~quay\.io/netobserv/goflow2-kube:dev~$(IMAGE):$(VERSION)~' ./examples/goflow-kube.yaml | kubectl apply -f - && \
	GF_IP=`kubectl get svc goflow -ojsonpath='{.spec.clusterIP}'` && echo "Goflow IP: $$GF_IP" && \
	kubectl set env daemonset/ovnkube-node -c ovnkube-node -n ovn-kubernetes OVN_IPFIX_TARGETS="$$GF_IP:2055"

cno-deploy:
	sed -e 's~quay\.io/netobserv/goflow2-kube:dev~$(IMAGE):$(VERSION)~' ./examples/goflow-kube.yaml | oc apply -f - && \
	GF_IP=`oc get svc goflow -ojsonpath='{.spec.clusterIP}'` && "Goflow IP: $$GF_IP" && \
	oc patch networks.operator.openshift.io cluster --type='json' -p "$(sed -e "s/GF_IP/$$GF_IP/" examples/net-cluster-patch.json)"

kill:
	kubectl delete pod -l app=goflow

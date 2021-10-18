IMAGE ?= quay.io/netobserv/goflow-kube
VERSION ?= $(shell git describe --long HEAD)
GOLANGCI_LINT_VERSION = v1.42.1
COVERPROFILE = coverage.out

ifeq (,$(shell which podman 2>/dev/null))
OCI_BIN ?= docker
else
OCI_BIN ?= podman
endif

.PHONY: prereqs
prereqs:
	@echo "### Test if prerequisites are met, and installing missing dependencies"
	test -f $(go env GOPATH)/bin/golangci-lint || go install github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}
	test -f $(go env GOPATH)/bin/staticcheck || go install honnef.co/go/tools/cmd/staticcheck@latest

.PHONY: vendors
vendors:
	@echo "### Checking vendors"
	go mod tidy && go mod vendor

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint: prereqs
	@echo "### Linting code"
	# staticcheck does not work properly when invoked inside golangci-lint
	staticcheck -f stylish ./...
	golangci-lint run ./...

.PHONY: test
test:
	@echo "### Testing"
	go test ./... -coverprofile ${COVERPROFILE}

.PHONY: verify
verify: lint test

.PHONY: build
build:
	@echo "### Building"
	go build -mod vendor -o goflow-kube cmd/goflow-kube.go

.PHONY: image
image:
	@echo "### Building image with ${OCI_BIN}"
	$(OCI_BIN) build --build-arg VERSION="$(VERSION)" -t $(IMAGE):$(VERSION) .

.PHONY: push
push:
	$(OCI_BIN) push $(IMAGE):$(VERSION)

.PHONY: ovnk-deploy
ovnk-deploy:
	sed -e 's~quay\.io/netobserv/goflow-kube:dev~$(IMAGE):$(VERSION)~' ./examples/goflow-kube.yaml | kubectl apply -f - && \
	GF_IP=`kubectl get svc goflow-kube -ojsonpath='{.spec.clusterIP}'` && echo "Goflow IP: $$GF_IP" && \
	kubectl set env daemonset/ovnkube-node -c ovnkube-node -n ovn-kubernetes OVN_IPFIX_TARGETS="$$GF_IP:2055"

.PHONY: cno-deploy
cno-deploy:
	sed -e 's~quay\.io/netobserv/goflow-kube:dev~$(IMAGE):$(VERSION)~' ./examples/goflow-kube.yaml | oc apply -f - && \
	GF_IP=`oc get svc goflow-kube -ojsonpath='{.spec.clusterIP}'` && "Goflow IP: $$GF_IP" && \
	oc patch networks.operator.openshift.io cluster --type='json' -p "$(sed -e "s/GF_IP/$$GF_IP/" examples/net-cluster-patch.json)"

.PHONY: kill
kill:
	kubectl delete pod -l app=goflow-kube


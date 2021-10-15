FROM registry.access.redhat.com/ubi8/go-toolset:1.15.14-10 as builder
ARG VERSION=""

WORKDIR /opt/app-root/src

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .


RUN go build -ldflags "-X main.version=${VERSION}" -o kube-enricher cmd/kube-enricher/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.4-210

COPY --from=builder /opt/app-root/src/kube-enricher ./

ENTRYPOINT ["./kube-enricher"]

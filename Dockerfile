FROM registry.access.redhat.com/ubi8/go-toolset:1.16.7-5 as builder
ARG VERSION=""

WORKDIR /opt/app-root
COPY . .

RUN go build -ldflags "-X main.version=${VERSION}" -mod vendor -o goflow-kube cmd/goflow-kube.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.5-204

COPY --from=builder /opt/app-root/goflow-kube ./

ENTRYPOINT ["./goflow-kube"]

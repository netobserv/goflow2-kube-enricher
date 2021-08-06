# goflow2-kube-enricher

Note: this is currently a prototype, not suitable for a production usage. Some improvements must be done such as adding a cache (kube informers) and supporting protobuf i/o.

## Build image

(This image will contain both goflow2 and the plugin)

```bash
docker build -t quay.io/jotak/goflow:v2-kube .
docker push quay.io/jotak/goflow:v2-kube

# or

podman build -t quay.io/jotak/goflow:v2-kube .
podman push quay.io/jotak/goflow:v2-kube
```

## Run in kube

Simply `pipe` goflow2 output to `kube-enricher`.

Example of usage in kube (assuming built image `quay.io/jotak/goflow:v2-kube`)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: goflow
  name: goflow
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: goflow
  template:
    metadata:
      labels:
        app: goflow
    spec:
      containers:
      - command:
        - /bin/sh
        - -c
        - /goflow2 -loglevel "debug" | /kube-enricher
        image: quay.io/jotak/goflow:v2-kube
        imagePullPolicy: IfNotPresent
        name: goflow
---
apiVersion: v1
kind: Service
metadata:
  name: goflow
  namespace: default
  labels:
    app: goflow
spec:
  ports:
  - port: 2055
    protocol: UDP
  selector:
    app: goflow
```

Example of output:

```
{"BiFlowDirection":0,"Bytes":345600,"DstAS":0,"DstAddr":"10.244.0.3","DstHostIP":"10.89.0.3","DstMac":"0a:58:0a:f4:00:03","DstNamespace":"local-path-storage","DstNet":0,"DstPod":"local-path-provisioner-78776bfc44-g2k4h","DstPort":48066,"DstVlan":0,"EgressVrfID":0,"Etype":2048,"EtypeName":"IPv4","ForwardingStatus":0,"FragmentId":0,"FragmentOffset":0,"IPTTL":0,"IPTos":0,"IPv6FlowLabel":0,"IcmpCode":0,"IcmpName":"","IcmpType":0,"InIf":7,"IngressVrfID":0,"NextHop":"","NextHopAS":0,"OutIf":0,"Packets":400,"Proto":6,"ProtoName":"TCP","SamplerAddress":"10.244.0.2","SamplingRate":0,"SequenceNum":1212,"SrcAS":0,"SrcAddr":"10.89.0.2","SrcHostIP":"10.89.0.2","SrcMac":"0e:25:5c:f5:a0:8b","SrcNamespace":"kube-system","SrcNet":0,"SrcPod":"etcd-ovn-control-plane","SrcPort":6443,"SrcVlan":0,"TCPFlags":0,"TimeFlowEnd":0,"TimeFlowStart":0,"TimeReceived":1628250609,"Type":"IPFIX","VlanId":0}
```

Notice `"SrcPod":"etcd-ovn-control-plane"`, `"SrcNamespace":"kube-system"`, etc.

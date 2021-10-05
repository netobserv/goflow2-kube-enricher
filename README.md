# goflow2-kube-enricher

Note: this is currently a prototype, not suitable for a production usage. Some improvements must be done such as adding a cache (kube informers) and supporting protobuf i/o.

## Description

This enricher adds kubernetes data to the output of `goflow2` based, by default, on the source and destination addresses of each record.

The fields mapping can be overriden for more general purpose using the `-mapping` option. The default is `SrcAddr=Src,DstAddr=Dst`. Keys refer to the fields to look for in goflow2 output and values refer to the prefix to use in created fields. For instance, it could be possible to process the `NextHop` field the same way with `-mapping "SrcAddr=Src,DstAddr=Dst,NextHop=Nxt"`

Generated fields are (with `[Prefix]` being by default `Src` or `Dst`):

- `[Prefix]Pod`: pod name
- `[Prefix]Namespace`: pod namespace
- `[Prefix]HostIP`: pod's host IP
- `[Prefix]Workload`: pod's workload, ie. controller/owner
- `[Prefix]WorkloadKind`: workload kind (deployment, daemon set, etc.)
- `[Prefix]Warn`: any warning message that could have been triggered while processing kube info

## Build binary

```bash
make all # = make fmt build lint test
```

## Build image

(This image will contain both goflow2 and the plugin)

```bash
# build an image with version "dev":
make image

# build and push a test version:
IMAGE=quay.io/myuser/goflow2-kube VERSION=test make image push
```

To run it, simply `pipe` goflow2 output to `kube-enricher`.

## RBAC

If RBAC is enabled, `kube-enricher` needs a few cluster-wide permissions:
- LIST on Pods and Services
- GET on ReplicaSets

Check [goflow-kube.yaml](./examples/goflow-kube.yaml) for an example.

## Examples in Kube

Assuming built image is `quay.io/netobserv/goflow2-kube:dev`.

Since both goflow + enricher are contained inside a single image, you can declare the following command inside the pod container:

```yaml
# ...
      containers:
      - command:
        - /bin/sh
        - -c
        - /goflow2 -loglevel "trace" | /kube-enricher -loglevel "trace"
        image: quay.io/netobserv/goflow2-kube:dev
# ...
```

Check the [examples](./examples) directory.

Example of output:

```
{"BiFlowDirection":0,"Bytes":20800,"DstAS":0,"DstAddr":"10.96.0.1","DstMac":"0a:58:0a:f4:00:01","DstNet":0,"DstPort":443,"DstVlan":0,"EgressVrfID":0,"Etype":2048,"EtypeName":"IPv4","ForwardingStatus":0,"FragmentId":0,"FragmentOffset":0,"IPTTL":0,"IPTos":0,"IPv6FlowLabel":0,"IcmpCode":0,"IcmpName":"","IcmpType":0,"InIf":12,"IngressVrfID":0,"NextHop":"","NextHopAS":0,"OutIf":0,"Packets":400,"Proto":6,"ProtoName":"TCP","SamplerAddress":"10.244.0.2","SamplingRate":0,"SequenceNum":577,"SrcAS":0,"SrcAddr":"10.244.0.5","SrcHostIP":"10.89.0.2","SrcMac":"0a:58:0a:f4:00:05","SrcNamespace":"local-path-storage","SrcNet":0,"SrcPod":"local-path-provisioner-78776bfc44-p2xkl","SrcPort":56144,"SrcVlan":0,"SrcWorkload":"local-path-provisioner","SrcWorkloadKind":"Deployment","TCPFlags":0,"TimeFlowEnd":0,"TimeFlowStart":0,"TimeReceived":1628419398,"Type":"IPFIX","VlanId":0}


```

Notice `"SrcPod":"local-path-provisioner-78776bfc44-p2xkl"`, `"SrcWorkload":"local-path-provisioner"`, `"SrcNamespace":"local-path-storage"`, etc.

### Run on Kind with ovn-kubernetes

First, [refer to this documentation](https://github.com/ovn-org/ovn-kubernetes/blob/master/docs/kind.md) to setup ovn-k on Kind.
Then:

```bash
kubectl apply -f ./examples/goflow-kube.yaml
GF_IP=`kubectl get svc goflow -ojsonpath='{.spec.clusterIP}'` && echo $GF_IP
kubectl set env daemonset/ovnkube-node -c ovnkube-node -n ovn-kubernetes OVN_IPFIX_TARGETS="$GF_IP:2055"
```

or simply:

```bash
make ovnk-deploy
```

Finally check goflow's logs for output

### Run on OpenShift with OVNKubernetes network provider

- Pre-requisite: make sure you have a running OpenShift cluster (4.8 at least) with `OVNKubernetes` set as the network provider.

In OpenShift, a difference with the upstream `ovn-kubernetes` is that the flows export config is managed by the `ClusterNetworkOperator`.

```bash
oc apply -f ./examples/goflow-kube.yaml
GF_IP=`oc get svc goflow -ojsonpath='{.spec.clusterIP}'` && echo $GF_IP
oc patch networks.operator.openshift.io cluster --type='json' -p "$(sed -e "s/GF_IP/$GF_IP/" examples/net-cluster-patch.json)"
```
or simply:

```bash
make cno-deploy
```

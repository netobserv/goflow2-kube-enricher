# goflow2-kube-enricher

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

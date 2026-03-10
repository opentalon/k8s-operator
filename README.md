# OpenTalon Kubernetes Operator

[![CI](https://github.com/opentalon/k8s-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/opentalon/k8s-operator/actions/workflows/ci.yaml)
[![Release](https://github.com/opentalon/k8s-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/opentalon/k8s-operator/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/opentalon/k8s-operator)](https://goreportcard.com/report/github.com/opentalon/k8s-operator)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A Kubernetes operator for deploying and managing [OpenTalon](https://github.com/opentalon/opentalon) instances with production-grade security, observability, and lifecycle management.

## Overview

The OpenTalon Kubernetes Operator simplifies self-hosting OpenTalon on Kubernetes. A single `OpenTalonInstance` custom resource defines the entire deployment stack — StatefulSet, Service, ConfigMap, RBAC, NetworkPolicy, PVC, Ingress, and monitoring — without writing any manifests by hand.

```yaml
apiVersion: opentalon.io/v1alpha1
kind: OpenTalonInstance
metadata:
  name: my-opentalon
  namespace: default
spec:
  envFrom:
    - secretRef:
        name: opentalon-api-keys
  config:
    models:
      - name: claude-sonnet-4-6
        provider: anthropic
    routing:
      primary: claude-sonnet-4-6
  storage:
    persistence:
      enabled: true
      size: 1Gi
```

## Features

**Declarative Management** — One resource defines the entire stack. The operator reconciles all child resources and keeps them in sync with the desired state.

**Config-Driven** — Inline model, routing, channel, plugin, state, and logging configuration generates a valid `config.yaml` automatically. Reference an existing ConfigMap for full control.

**Rolling Updates on Config Change** — SHA-256 hashing of the rendered config triggers automatic StatefulSet rollouts whenever configuration changes, with no manual restarts needed.

**Multi-Channel Support** — Configure console, Slack (with secret references for bot tokens), webhook, and WebSocket channels directly in the CRD.

**Plugin Management** — Declare gRPC plugins by source (binary path or GitHub URL) and the operator injects them into the container environment.

**Security Hardening** — Non-root execution (UID 1000), read-only root filesystem, all Linux capabilities dropped, seccomp `RuntimeDefault`, and optional default-deny `NetworkPolicy`.

**Persistent SQLite State** — PVC-backed `/data` volume for SQLite conversation history, surviving pod restarts and rescheduling.

**Observability** — Prometheus metrics endpoint with optional `ServiceMonitor` for automatic scrape target registration.

**High Availability** — Horizontal Pod Autoscaler and PodDisruptionBudget support for production deployments.

**Auto-Updates** — Optional scheduled version checks with configurable cron schedule.

## Quick Start

### Prerequisites

- Kubernetes 1.28+
- `kubectl` configured against your cluster
- Helm 3 (for Helm installation)

### Install with Helm

```bash
helm install opentalon-operator oci://ghcr.io/opentalon/charts/opentalon-operator \
  --namespace opentalon-operator-system \
  --create-namespace
```

### Install with kubectl

```bash
# Install CRDs
kubectl apply -f https://github.com/opentalon/k8s-operator/releases/latest/download/opentalon-operator.crds.yaml

# Deploy the operator
kubectl apply -f https://github.com/opentalon/k8s-operator/releases/latest/download/opentalon-operator.yaml
```

### Deploy an instance

1. Create a secret with your LLM provider API key:

```bash
kubectl create secret generic opentalon-api-keys \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-...
```

2. Apply an `OpenTalonInstance`:

```bash
kubectl apply -f config/samples/opentalon_v1alpha1_opentaloninstance.yaml
```

3. Check status:

```bash
kubectl get opentaloninstances
kubectl describe opentaloninstance my-opentalon
```

## Configuration Reference

### Image

```yaml
spec:
  image:
    repository: ghcr.io/opentalon/opentalon  # default
    tag: latest                               # default
    pullPolicy: IfNotPresent                  # default
```

### Models and Routing

```yaml
spec:
  config:
    models:
      - name: claude-sonnet-4-6
        provider: anthropic
        apiKeySecret:
          name: my-secret
          key: ANTHROPIC_API_KEY
      - name: gpt-4o
        provider: openai
        apiKeySecret:
          name: my-secret
          key: OPENAI_API_KEY
    routing:
      primary: claude-sonnet-4-6
      fallbacks:
        - gpt-4o
```

### Channels

```yaml
spec:
  config:
    channels:
      slack:
        enabled: true
        tokenSecret:
          name: slack-credentials
          key: SLACK_BOT_TOKEN
        appTokenSecret:
          name: slack-credentials
          key: SLACK_APP_TOKEN
      webhook:
        enabled: true
        port: 8080
        path: /webhook
      websocket:
        enabled: true
        port: 8081
```

### Plugins

```yaml
spec:
  config:
    plugins:
      - name: my-tool
        source: github.com/my-org/my-plugin
      - name: local-tool
        source: /usr/local/bin/my-tool
        args: ["--verbose"]
```

### Persistence

```yaml
spec:
  storage:
    persistence:
      enabled: true
      size: 5Gi
      storageClassName: standard
```

### Networking

```yaml
spec:
  networking:
    service:
      type: ClusterIP
      port: 8080
    ingress:
      enabled: true
      className: nginx
      host: opentalon.example.com
      tlsSecretName: opentalon-tls
      annotations:
        cert-manager.io/cluster-issuer: letsencrypt
    networkPolicy:
      enabled: true
```

### Observability

```yaml
spec:
  observability:
    metrics:
      enabled: true
      port: 9090
      serviceMonitor:
        enabled: true
        interval: 30s
        labels:
          prometheus: kube-prometheus
```

### High Availability

```yaml
spec:
  replicas: 3
  availability:
    podDisruptionBudget:
      enabled: true
      minAvailable: 2
    horizontalPodAutoscaler:
      enabled: true
      minReplicas: 2
      maxReplicas: 10
      cpuUtilization: 70
```

### Full config.yaml control

Reference an existing ConfigMap to bypass inline config generation entirely:

```yaml
spec:
  configFrom:
    name: my-opentalon-config
```

The ConfigMap must contain a `config.yaml` key.

## Development

### Prerequisites

- Go 1.24+
- Docker
- [kubebuilder](https://book.kubebuilder.io/quick-start.html)
- [kind](https://kind.sigs.k8s.io/) (for local testing)

### Build

```bash
make build
```

### Generate CRD manifests and DeepCopy

```bash
make manifests generate
```

### Run tests

```bash
make test
```

### Run locally against a cluster

```bash
make install        # install CRDs
make run            # run controller locally
```

### Build and push image

```bash
make docker-build docker-push IMG=ghcr.io/opentalon/opentalon-operator:dev
```

### Deploy to cluster

```bash
make deploy IMG=ghcr.io/opentalon/opentalon-operator:dev
```

### Sync Helm chart CRDs

```bash
make sync-chart-crds
```

## Release Process

Push a version tag to trigger a release:

```bash
git tag v1.2.3
git push origin v1.2.3
```

The release workflow then:

1. Builds and pushes multi-arch Docker images (`linux/amd64`, `linux/arm64`)
2. Signs images with [Cosign](https://github.com/sigstore/cosign)
3. Generates and attests an SBOM
4. Packages and pushes the Helm chart to the OCI registry

## License

Apache License 2.0. See [LICENSE](LICENSE).

OpenTalon and the OpenTalon Kubernetes Operator are not affiliated with any commercial entity.

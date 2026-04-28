[![GitHub License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Overview

The CCE Karpenter Provider enables node autoprovisioning using [Karpenter](https://karpenter.sh/) on your CCE cluster.
Karpenter improves the efficiency and cost of running workloads on Kubernetes clusters by:

* **Watching** for pods that the Kubernetes scheduler has marked as unschedulable
* **Evaluating** scheduling constraints (resource requests, node selectors, affinities, tolerations, and topology-spread constraints) requested by the pods
* **Provisioning** nodes that meet the requirements of the pods
* **Removing** the nodes when they are no longer needed
* **Consolidating** existing nodes onto cheaper nodes with higher utilization per node

## Known Limitations

This project is at an early **v1alpha1** stage. Please be aware of the following limitations:

- **On-demand only**: Only `on-demand` capacity type is supported. Spot instances are not yet available.
- **Kubelet configuration**: Only the following fields are consumed at runtime for capacity/overhead calculations: `maxPods`, `podsPerCore`, `kubeReserved`, `systemReserved`, `evictionHard`, `evictionSoft`. Other fields (`clusterDNS`, `evictionSoftGracePeriod`, `evictionMaxPodGracePeriod`, `imageGCHighThresholdPercent`, `imageGCLowThresholdPercent`, `cpuCFSQuota`) are defined in the API schema but are not yet wired to node launch configuration.
- **Default k8s data disk**: CCE requires a data disk for worker nodes. If `spec.blockDeviceMappings.k8s` is omitted, a **100 GiB data disk** using the same type as `spec.blockDeviceMappings.root` is added automatically.
- **Data disk minimum size**: In the current CCE CreateNode validation, explicit `k8s` and `users` data volumes must be at least **100 GiB**.

## Prerequisites

- A **Huawei Cloud CCE cluster** running Kubernetes **1.26 - 1.34**
- Huawei Cloud credentials (AK/SK) with permissions for:
  - **ECS** - Elastic Cloud Server (flavor listing, server tagging)
  - **CCE** - Cloud Container Engine (node create/delete/list)
  - **VPC** - Virtual Private Cloud (subnet discovery)
  - **BSS** - Billing (on-demand pricing queries)
- [Helm](https://helm.sh/) v3


## Installation

### 1. Install via Helm

```bash
helm install karpenter-provider-huawei charts/karpenter-provider-huawei \
  --namespace karpenter-provider-huawei-system \
  --create-namespace \
  --set-string credentials.accessKey=<your-access-key> \
  --set-string credentials.secretKey=<your-secret-key> \
  --set-string credentials.region=<region-id> \
  --set-string credentials.clusterID=<cce-cluster-id>
```

The chart creates a `huawei-credentials` Secret by default and loads it into the controller.
To use an existing Secret instead, set `credentials.create=false` and `credentials.existingSecret=<secret-name>`.
When using an existing Secret, provide `HUAWEICLOUD_SDK_AK`, `HUAWEICLOUD_SDK_SK`, `HUAWEICLOUD_SDK_REGION_ID`, and
`HUAWEICLOUD_SDK_CCE_CLUSTER_ID`.
Optional SDK settings can be passed through Helm values or the existing Secret:
`credentials.projectID`, `credentials.domainID`, `credentials.iamEndpoint`, `credentials.ecsEndpoint`,
`credentials.vpcEndpoint`, `credentials.cceEndpoint`, `credentials.bssEndpoint`, and
`credentials.ignoreSSLVerification`.
For existing Secrets, service endpoint overrides use `HUAWEICLOUD_SDK_REGION_<SERVICE>_<REGION_ID>` keys such as
`HUAWEICLOUD_SDK_REGION_ECS_AP_SOUTHEAST_3`, and TLS verification can be controlled with
`HUAWEICLOUD_SDK_IGNORE_SSL_VERIFICATION`.

The default controller image is:
```
swr.ap-southeast-3.myhuaweicloud.com/huaweiclouddeveloper/cce/karpenter/controller:latest
```

To use a custom controller image:

```bash
helm install karpenter-provider-huawei charts/karpenter-provider-huawei \
  --namespace karpenter-provider-huawei-system \
  --create-namespace \
  --set image.repository=<your-registry>/controller \
  --set image.tag=<your-tag> \
  --set-string credentials.accessKey=<your-access-key> \
  --set-string credentials.secretKey=<your-secret-key> \
  --set-string credentials.region=<region-id> \
  --set-string credentials.clusterID=<cce-cluster-id>
```

## Getting Started

### Step 1: Create a CCENodeClass

`CCENodeClass` is a **cluster-scoped** resource that defines Huawei Cloud-specific node configuration:

```yaml
apiVersion: karpenter.k8s.huawei/v1alpha1
kind: CCENodeClass
metadata:
  name: default
spec:
  subnetSelectorTerms:
    - id: "<subnet-uuid>"                  # Your VPC subnet ID
  imsSelector:
    imsFamily: "HCE OS 2.0"                # OS family
  blockDeviceMappings:
    k8s:
      volumeSize: 120
      volumeType: SAS
    root:
      volumeSize: 120
      volumeType: SATA
    users:
      - volumeSize: 100
        volumeType: SAS
  runtimeConfiguration:
    type: containerd
  login:
    userPassword:
      username: root
      password: "<salted-and-encrypted-password>"
```

After creation, wait for the `SubnetsReady` condition to become `True` before the NodeClass can be used for provisioning:

```bash
kubectl get ccenodeclass default -o jsonpath='{.status.conditions}'
```

### Step 2: Create a NodePool

Create a Karpenter `NodePool` that references your `CCENodeClass`:

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    spec:
      nodeClassRef:
        group: karpenter.k8s.huawei
        kind: CCENodeClass
        name: default
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 1m
```

### Step 3: Deploy a Workload

Deploy a workload with resource requests. Karpenter will automatically provision right-sized nodes:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: inflate
spec:
  replicas: 5
  selector:
    matchLabels:
      app: inflate
  template:
    metadata:
      labels:
        app: inflate
    spec:
      containers:
        - name: inflate
          image: nginx:latest
          resources:
            requests:
              cpu: "1"
              memory: 1Gi
EOF
```

## Development

### Prerequisites

- [Go](https://go.dev/) 1.25+
- [Docker](https://www.docker.com/) (or Podman)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/) v3
- [Kind](https://kind.sigs.k8s.io/) (for e2e tests)

### Build

```bash
make build                                              # Build controller binary
make docker-build IMG=<your-registry>/controller:<tag>  # Build Docker image
make docker-push IMG=<your-registry>/controller:<tag>   # Push Docker image
make docker-buildx IMG=<your-registry>/controller:<tag> # Cross-platform build
```

### Test

```bash
make test      # Unit tests
make test-e2e  # E2E tests (uses Kind)
```

### Lint

```bash
make lint      # Run golangci-lint
make lint-fix  # Run with auto-fix
```

### Code Generation

```bash
make manifests  # Generate CRD manifests
make generate   # Generate DeepCopy methods
```

### Helm

```bash
make helm-lint       # Lint the chart
make helm-template   # Render templates locally
make helm-install    # Install
make helm-upgrade    # Upgrade
make helm-uninstall  # Uninstall
```

## Roadmap

See [docs/ROADMAP.md](docs/ROADMAP.md) for the project roadmap and progress.

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes and ensure tests pass (`make test && make lint`)
4. Submit a Pull Request following the [PR template](.github/PULL_REQUEST_TEMPLATE.md)

## Community

- **Issues**: [GitHub Issues](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/issues)
- **Karpenter Upstream**: [karpenter.sh](https://karpenter.sh/) | [Karpenter GitHub](https://github.com/kubernetes-sigs/karpenter)

## Code of Conduct

This project follows the [CNCF Community Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

## License

This project is licensed under the [Apache License 2.0](LICENSE).

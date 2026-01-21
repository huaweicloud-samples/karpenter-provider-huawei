# Karpenter Provider Huawei

Huawei Cloud provider for [Karpenter](https://karpenter.sh/) - the Kubernetes Node Autoscaling project.

## Overview

This project implements the Karpenter cloud provider interface for Huawei Cloud, enabling automatic provisioning of Kubernetes nodes on Huawei Cloud infrastructure.

## Status

🚧 **Under Development** - This project is in early development stages.

## Features (Planned)

- [ ] Automatic node provisioning based on pending pod requirements
- [ ] Support for Huawei Cloud ECS instance types
- [ ] Spot instance support
- [ ] Custom HuaweiNodeClass CRD for advanced configuration
- [ ] Integration with Huawei Cloud pricing for cost optimization

## Prerequisites

- Kubernetes 1.25+
- Karpenter v1.0+
- Huawei Cloud CCE cluster

## Installation

*Coming soon*

## Development

### Requirements

- Go 1.23+
- Docker
- kubectl
- Access to a Huawei Cloud CCE cluster

### Build

```bash
# Build the controller
make build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

### Project Structure

```
├── cmd/
│   └── controller/          # Main entry point
├── pkg/
│   ├── apis/                # CRD definitions (HuaweiNodeClass)
│   ├── controllers/         # Kubernetes controllers
│   ├── providers/           # Cloud provider implementations
│   │   ├── instance/        # Instance lifecycle management
│   │   ├── instancetype/    # Instance type discovery
│   │   ├── pricing/         # Pricing information
│   │   └── securitygroup/   # Security group management
│   └── cloudprovider/       # Karpenter CloudProvider interface
├── charts/                  # Helm charts
├── hack/                    # Build and utility scripts
└── Makefile
```

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting a pull request.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Acknowledgments

- [Karpenter](https://github.com/kubernetes-sigs/karpenter) - The upstream Karpenter project
- [karpenter-provider-aws](https://github.com/aws/karpenter-provider-aws) - AWS provider reference implementation
- [karpenter-provider-alibabacloud](https://github.com/cloudpilot-ai/karpenter-provider-alibabacloud) - Alibaba Cloud provider reference

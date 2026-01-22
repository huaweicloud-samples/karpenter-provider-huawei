# Karpenter Provider Huawei

Huawei Cloud provider for [Karpenter](https://karpenter.sh/) - the Kubernetes Node Autoscaling project.

## Overview

This project implements the Karpenter cloud provider interface for Huawei Cloud, enabling automatic provisioning of Kubernetes nodes on Huawei Cloud infrastructure.

## Prerequisites

- Karpenter v1.8+
- Huawei Cloud CCE cluster

## Installation

TODO

## Development

TODO

### Project Structure

```
├── cmd/
│   └── controller/          # Main entry point
├── pkg/
│   ├── apis/                # CRD definitions (ECSNodeClass)
│   ├── controllers/         # Kubernetes controllers
│   ├── providers/           # Cloud provider implementations
│   │   ├── instance/        # Instance lifecycle management
│   │   ├── instancetype/    # Instance type discovery
│   │   ├── pricing/         # Pricing information
│   │   └── securitygroup/   # Security group management
│   └── cloudprovider/       # Karpenter CloudProvider interface
├── hack/                    # Build and utility scripts
└── Makefile
```

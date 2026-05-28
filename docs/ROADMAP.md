# Karpenter Provider Huawei — Roadmap

> 本路线图将持续迭代，并会不定期更新调整。

## v0.2.1

> 完成设备存储能力支持，并继续完善节点预留资源。

- [x] 支持配置 Kubernetes 设备存储，增强节点本地存储编排能力 ([#74](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/74))
- [x] 接入 SDK credential provider chain，增强凭据获取方式的兼容性 ([#78](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/78))
- [x] 修正默认 CCE 节点预留资源配置，使其与实际行为保持一致 ([#73](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/73))

## v0.2.0

> 完成 CCENodeClass 重构，并增强 Helm Chart 的部署配置能力。

- [x] 重构为CCENodeClass，统一资源命名语义 ([#68](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/68))
- [x] 支持通过 Helm Chart 配置凭据 Secret，简化镜像拉取与部署认证配置 ([#69](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/69))
- [x] 支持通过 Helm Chart 配置 ENI EIP annotations，增强网络能力配置 ([#71](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/71))


## v0.1.0 (v1alpha1)

> 完成项目初始化、ECSNodeClass API 定义及核心 Provider 实现。

- [x] 搭建项目基本框架，并定义 ECSNodeClass CRD ([#1](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/1))
- [x] 实现 NodeClass 控制器的协调逻辑框架 ([#10](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/10))
- [x] 实现 Version Provider 及其控制器协调逻辑 ([#8](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/8))
- [x] 实现 Subnet Provider 及其控制器协调逻辑 ([#11](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/11))
- [x] 实现 InstanceType Provider 及其控制器协调逻辑 ([#17](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/17))
- [x] 实现 Instance Provider 及其控制器协调逻辑 ([#29](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/29))
- [x] 实现 Pricing Provider 及其控制器协调逻辑 ([#32](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/32))
- [x] 实现 CCE 集群节点扩缩容，打通 NodeClaim 的完整调度链路 ([#31](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/31))
- [x] 支持通过 Helm Chart 模板进行自动化部署 ([#46](https://github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pull/46))

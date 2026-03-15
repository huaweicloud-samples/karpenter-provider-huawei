# Karpenter Provider Huawei — Roadmap

> 本路线图将持续迭代，并会不定期更新调整。

## v0.0.1 (v1alpha1)

> 完成项目初始化、ECSNodeClass API 定义及核心 Provider 实现。

- [x] 搭建项目基本框架，并定义 ECSNodeClass CRD ([#1](https://github.com/setoru/karpenter-provider-huawei/pull/1))
- [x] 实现 NodeClass 控制器的协调逻辑框架 ([#10](https://github.com/setoru/karpenter-provider-huawei/pull/10))
- [x] 支持基于 kind 的本地开发工作流
- [x] 实现 Version Provider 及其控制器协调逻辑 ([#8](https://github.com/setoru/karpenter-provider-huawei/pull/8))
- [x] 实现 Subnet Provider 及其控制器协调逻辑 ([#11](https://github.com/setoru/karpenter-provider-huawei/pull/11))
- [x] 实现 InstanceType Provider 及其控制器协调逻辑 ([#17](https://github.com/setoru/karpenter-provider-huawei/pull/17))
- [x] 打通 NodeClaim 的整体调度链路 ([#23](https://github.com/setoru/karpenter-provider-huawei/pull/23))
- [ ] 实现 Instance Provider 及其控制器协调逻辑
- [ ] 实现 Pricing Provider 及其控制器协调逻辑
- [ ] 实现实例引导并注册为 CCE 集群节点
- [ ] 打通 NodeClaim 的完整生命周期（创建、删除、状态同步）
- [ ] 集群节点扩缩容能力可用
- [ ] 支持通过 Helm Chart 模板进行自动化部署

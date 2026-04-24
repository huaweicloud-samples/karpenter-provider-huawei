/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package operator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	coreRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
	cceRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/region"
	ecsRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
	vpcRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/region"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/operator"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instance"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instancetype"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/pricing"

	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/version"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/utils"
)

const (
	// DefaultTTL is the default cache TTL for Huawei Cloud API calls used during
	// setup verification and provisioning. Cache hits enable faster provisioning
	// and reduced API load, which can have a serious impact on performance and
	// scalability. DO NOT CHANGE THIS VALUE WITHOUT DUE CONSIDERATION.
	DefaultTTL = time.Minute
	// DefaultCleanupInterval triggers cache cleanup (lazy eviction) at this interval.
	DefaultCleanupInterval = time.Minute
	// AvailableIPAddressTTL is time to drop AvailableIPAddress data if it is not updated within the TTL
	AvailableIPAddressTTL = 5 * time.Minute
	// InstanceTypesZonesAndOfferingsTTL is the time before we refresh instance types, zones, and offerings at EC2
	InstanceTypesZonesAndOfferingsTTL = 5 * time.Minute
	// UnavailableOfferingTTL is the duration to suppress recently sold-out offerings from scheduling.
	UnavailableOfferingTTL = 5 * time.Minute
	// if it is not updated by a node creation event or refreshed during controller reconciliation
	DiscoveredCapacityCacheTTL = 60 * 24 * time.Hour
	BillingEndpoint            = "https://bss.myhuaweicloud.com"
)

// Operator is injected into the HuaweiCloud CloudProvider's factories
type Operator struct {
	*operator.Operator
	VersionProvider      *version.DefaultProvider
	SubnetProvider       subnet.Provider
	InstanceTypeProvider *instancetype.DefaultProvider
	InstanceProvider     instance.Provider
	PricingProvider      *pricing.DefaultProvider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	logger := log.FromContext(ctx)

	reg := os.Getenv("HUAWEICLOUD_REGION")
	vpcReg, err := vpcRegion.SafeValueOf(reg)
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get VPC region: %w", err))
	}
	ecsReg, err := ecsRegion.SafeValueOf(reg)
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get ECS region: %w", err))
	}
	cceReg, err := cceRegion.SafeValueOf(reg)
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get CCE region: %w", err))
	}
	ak := os.Getenv("HUAWEICLOUD_AK")
	if ak == "" {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}
	sk := os.Getenv("HUAWEICLOUD_SK")
	if sk == "" {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}
	credentialsBuilder := basic.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk)
	credentials, err := credentialsBuilder.SafeBuild()
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}
	globalCredentials, err := global.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk).
		SafeBuild()
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get global credentials"))
	}

	clusterID := os.Getenv("HUAWEICLOUD_CCE_CLUSTER_ID")
	if clusterID == "" {
		lo.Must0(fmt.Errorf("unable to get CCE cluster id"))
	}

	vpcApi := sdk.NewVPCService(vpcReg, credentials, config.DefaultHttpConfig())
	subnetProvider := subnet.NewDefaultProvider(vpcApi, cache.New(DefaultTTL, DefaultCleanupInterval), cache.New(AvailableIPAddressTTL, DefaultCleanupInterval))

	versionProvider := version.NewDefaultProvider(operator.KubernetesInterface)
	lo.Must0(versionProvider.UpdateVersionWithValidation(ctx))

	ecsApi := sdk.NewECSService(ecsReg, credentials, config.DefaultHttpConfig())
	bssApi := sdk.NewBSSService(billingRegion(reg), globalCredentials, config.DefaultHttpConfig())
	cceApi := sdk.NewCCEService(cceReg, credentials, config.DefaultHttpConfig())
	unavailableOfferingCache := utils.NewOfferingAvailabilityCache(UnavailableOfferingTTL, DefaultCleanupInterval)
	instanceProvider := instance.NewDefaultProvider(clusterID, cceApi, ecsApi, subnetProvider, unavailableOfferingCache)
	pricingProvider := pricing.NewDefaultProvider(bssApi, reg, func() string { return credentials.ProjectId })
	instanceTypeProvider := instancetype.NewDefaultProvider(
		ecsApi,
		cache.New(InstanceTypesZonesAndOfferingsTTL, DefaultCleanupInterval),
		cache.New(DiscoveredCapacityCacheTTL, DefaultCleanupInterval),
		unavailableOfferingCache,
		instancetype.NewDefaultResolver(reg),
		pricingProvider.OnDemandPrice,
	)

	if err := instanceTypeProvider.Refresh(ctx); err != nil {
		logger.Error(err, "failed to preload instance types and offerings")
	}
	if err := pricingProvider.UpdateOnDemandPricing(ctx, instanceTypeProvider.InstanceTypeInfos()); err != nil {
		logger.Error(err, "failed to preload on-demand pricing")
	}
	return ctx, &Operator{
		Operator:             operator,
		VersionProvider:      versionProvider,
		SubnetProvider:       subnetProvider,
		InstanceTypeProvider: instanceTypeProvider,
		InstanceProvider:     instanceProvider,
		PricingProvider:      pricingProvider,
	}
}

func billingRegion(regionID string) *coreRegion.Region {
	return coreRegion.NewRegion(regionID, BillingEndpoint)
}

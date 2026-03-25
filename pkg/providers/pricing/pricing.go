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

package pricing

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"

	"github.com/awslabs/operatorpkg/serrors"
	bssMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/shopspring/decimal"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/utils/pretty"

	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

const (
	ecsCloudServiceTypeCode       = "hws.service.type.ec2"
	ecsVMResourceTypeCode         = "hws.resource.type.vm"
	durationUsageFactor           = "Duration"
	hourUsageMeasureID      int32 = 4
	inquiryPrecisionHigh    int32 = 1
	maxProductsPerBatch           = 100
	linuxResourceSpecSuffix       = ".linux"
)

type Provider interface {
	LivenessProbe(*http.Request) error
	InstanceTypes() []sdk.InstanceType
	OnDemandPrice(sdk.InstanceType) (float64, bool)
	UpdateOnDemandPricing(context.Context, map[sdk.InstanceType]ecsMdl.Flavor) error
}

// DefaultProvider provides actual pricing data to the Huawei cloud provider to allow it to make more informed decisions
// regarding which instances to launch.
type DefaultProvider struct {
	pricing sdk.PricingAPI
	cm      *pretty.ChangeMonitor

	region            string
	projectIDProvider func() string

	muOnDemand     sync.RWMutex
	onDemandPrices map[sdk.InstanceType]float64
}

func NewDefaultProvider(
	pricing sdk.PricingAPI,
	region string,
	projectIDProvider func() string,
) *DefaultProvider {
	return &DefaultProvider{
		pricing:           pricing,
		cm:                pretty.NewChangeMonitor(),
		region:            region,
		projectIDProvider: projectIDProvider,
		onDemandPrices:    map[sdk.InstanceType]float64{},
	}
}

func (p *DefaultProvider) LivenessProbe(_ *http.Request) error {
	p.muOnDemand.Lock()
	//nolint: staticcheck
	p.muOnDemand.Unlock()
	return nil
}

func (p *DefaultProvider) InstanceTypes() []sdk.InstanceType {
	p.muOnDemand.RLock()
	defer p.muOnDemand.RUnlock()

	instanceTypes := make([]sdk.InstanceType, 0, len(p.onDemandPrices))
	for instanceType := range p.onDemandPrices {
		instanceTypes = append(instanceTypes, instanceType)
	}
	sort.Slice(instanceTypes, func(i, j int) bool {
		return instanceTypes[i] < instanceTypes[j]
	})
	return instanceTypes
}

func (p *DefaultProvider) OnDemandPrice(instanceType sdk.InstanceType) (float64, bool) {
	p.muOnDemand.RLock()
	defer p.muOnDemand.RUnlock()

	price, ok := p.onDemandPrices[instanceType]
	return price, ok
}

func (p *DefaultProvider) UpdateOnDemandPricing(ctx context.Context, instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor) error {
	projectID := p.projectID()
	if projectID == "" {
		return fmt.Errorf("project id is empty")
	}
	if len(instanceTypeInfos) == 0 {
		return fmt.Errorf("no instance types found")
	}

	prices, err := p.fetchOnDemandPricing(projectID, instanceTypeInfos)
	if err != nil {
		return err
	}
	if len(prices) == 0 {
		return fmt.Errorf("no on-demand pricing found")
	}

	p.muOnDemand.Lock()
	defer p.muOnDemand.Unlock()

	p.onDemandPrices = mergeOnDemandPricing(p.onDemandPrices, prices)
	if p.cm.HasChanged("on-demand-prices", p.onDemandPrices) {
		log.FromContext(ctx).WithValues("instance-type-count", len(p.onDemandPrices)).V(1).Info("updated on-demand pricing")
	}
	return nil
}

func (p *DefaultProvider) projectID() string {
	if p.projectIDProvider == nil {
		return ""
	}
	return p.projectIDProvider()
}

func (p *DefaultProvider) fetchOnDemandPricing(
	projectID string,
	instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor,
) (map[sdk.InstanceType]float64, error) {
	productInfos := buildDemandProductInfos(p.region, instanceTypeInfos)
	if len(productInfos) == 0 {
		return nil, fmt.Errorf("no demand product infos built")
	}

	prices := map[sdk.InstanceType]float64{}
	for start := 0; start < len(productInfos); start += maxProductsPerBatch {
		end := start + maxProductsPerBatch
		if end > len(productInfos) {
			end = len(productInfos)
		}
		response, err := p.pricing.ListOnDemandResourceRatings(&bssMdl.ListOnDemandResourceRatingsRequest{
			Body: &bssMdl.RateOnDemandReq{
				ProjectId:        projectID,
				InquiryPrecision: int32Ptr(inquiryPrecisionHigh),
				ProductInfos:     productInfos[start:end],
			},
		})
		if err != nil {
			return nil, serrors.Wrap(fmt.Errorf("list on-demand resource ratings, %w", err))
		}
		prices = mergeOnDemandPricing(prices, onDemandPage(response))
	}
	return prices, nil
}

func buildDemandProductInfos(
	region string,
	instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor,
) []bssMdl.DemandProductInfo {
	instanceTypes := make([]sdk.InstanceType, 0, len(instanceTypeInfos))
	for instanceType := range instanceTypeInfos {
		instanceTypes = append(instanceTypes, instanceType)
	}
	sort.Slice(instanceTypes, func(i, j int) bool {
		return instanceTypes[i] < instanceTypes[j]
	})

	productInfos := make([]bssMdl.DemandProductInfo, 0, len(instanceTypes))
	for _, instanceType := range instanceTypes {
		flavor := instanceTypeInfos[instanceType]
		productInfos = append(productInfos, bssMdl.DemandProductInfo{
			Id:               string(instanceType),
			CloudServiceType: ecsCloudServiceTypeCode,
			ResourceType:     ecsVMResourceTypeCode,
			ResourceSpec:     resourceSpec(flavor),
			Region:           region,
			UsageFactor:      durationUsageFactor,
			UsageValue:       decimalPtr(decimal.NewFromInt(1)),
			UsageMeasureId:   hourUsageMeasureID,
			SubscriptionNum:  1,
		})
	}
	return productInfos
}

func resourceSpec(flavor ecsMdl.Flavor) string {
	spec := flavor.Id
	if spec == "" {
		spec = flavor.Name
	}
	return spec + linuxResourceSpecSuffix
}

func onDemandPage(response *bssMdl.ListOnDemandResourceRatingsResponse) map[sdk.InstanceType]float64 {
	prices := map[sdk.InstanceType]float64{}
	if response == nil || response.ProductRatingResults == nil {
		return prices
	}
	for _, result := range *response.ProductRatingResults {
		if result.Id == nil {
			continue
		}
		price := priceFromRatingResult(result)
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}
		prices[sdk.InstanceType(*result.Id)] = price
	}
	return prices
}

func priceFromRatingResult(result bssMdl.DemandProductRatingResult) float64 {
	if result.Amount != nil {
		return result.Amount.InexactFloat64()
	}
	if result.OfficialWebsiteAmount != nil {
		return result.OfficialWebsiteAmount.InexactFloat64()
	}
	return 0
}

func mergeOnDemandPricing(
	existing map[sdk.InstanceType]float64,
	updated map[sdk.InstanceType]float64,
) map[sdk.InstanceType]float64 {
	merged := map[sdk.InstanceType]float64{}
	for instanceType, price := range existing {
		merged[instanceType] = price
	}
	for instanceType, price := range updated {
		merged[instanceType] = price
	}
	return merged
}

func int32Ptr(v int32) *int32 {
	return &v
}

func decimalPtr(v decimal.Decimal) *decimal.Decimal {
	return &v
}

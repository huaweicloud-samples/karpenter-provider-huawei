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
	"strings"
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
	productNotFoundPrefix         = "Can not find product "
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

	muOnDemand          sync.RWMutex
	onDemandPrices      map[sdk.InstanceType]float64
	muUnsupported       sync.RWMutex
	unsupportedProducts map[sdk.InstanceType]struct{}
}

type demandProductInfo struct {
	instanceType sdk.InstanceType
	resourceSpec string
	productInfo  bssMdl.DemandProductInfo
}

func NewDefaultProvider(
	pricing sdk.PricingAPI,
	region string,
	projectIDProvider func() string,
) *DefaultProvider {
	return &DefaultProvider{
		pricing:             pricing,
		cm:                  pretty.NewChangeMonitor(),
		region:              region,
		projectIDProvider:   projectIDProvider,
		onDemandPrices:      map[sdk.InstanceType]float64{},
		unsupportedProducts: map[sdk.InstanceType]struct{}{},
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

	prices, err := p.fetchOnDemandPricing(ctx, projectID, instanceTypeInfos)
	if err != nil {
		return err
	}
	if len(prices) == 0 {
		return fmt.Errorf("no on-demand pricing found")
	}

	p.muOnDemand.Lock()
	defer p.muOnDemand.Unlock()

	p.onDemandPrices = mergeOnDemandPricing(p.onDemandPrices, prices)
	for instanceType := range p.unsupportedInstanceTypes() {
		delete(p.onDemandPrices, instanceType)
	}
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
	ctx context.Context,
	projectID string,
	instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor,
) (map[sdk.InstanceType]float64, error) {
	productInfos := buildDemandProductInfos(p.region, p.filterUnsupportedInstanceTypes(instanceTypeInfos))
	if len(productInfos) == 0 {
		return nil, fmt.Errorf("no demand product infos built")
	}

	prices := map[sdk.InstanceType]float64{}
	for start := 0; start < len(productInfos); start += maxProductsPerBatch {
		end := start + maxProductsPerBatch
		if end > len(productInfos) {
			end = len(productInfos)
		}
		response, err := p.fetchOnDemandPricingBatch(ctx, projectID, productInfos[start:end])
		if err != nil {
			return nil, err
		}
		prices = mergeOnDemandPricing(prices, onDemandPage(response))
	}
	return prices, nil
}

func (p *DefaultProvider) fetchOnDemandPricingBatch(
	ctx context.Context,
	projectID string,
	productInfos []demandProductInfo,
) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
	remaining := append([]demandProductInfo(nil), productInfos...)
	for len(remaining) > 0 {
		response, err := p.pricing.ListOnDemandResourceRatings(&bssMdl.ListOnDemandResourceRatingsRequest{
			Body: &bssMdl.RateOnDemandReq{
				ProjectId:        projectID,
				InquiryPrecision: int32Ptr(inquiryPrecisionHigh),
				ProductInfos:     requestProductInfos(remaining),
			},
		})
		if err == nil {
			return response, nil
		}

		resourceSpec, ok := missingResourceSpecFromError(err)
		if !ok {
			return nil, serrors.Wrap(fmt.Errorf("list on-demand resource ratings, %w", err))
		}

		filtered, removed := filterOutProductInfoByResourceSpec(remaining, resourceSpec)
		if len(removed) == 0 {
			return nil, serrors.Wrap(fmt.Errorf("list on-demand resource ratings, %w", err))
		}
		p.markUnsupportedProducts(ctx, removed)
		remaining = filtered
	}
	return &bssMdl.ListOnDemandResourceRatingsResponse{}, nil
}

func buildDemandProductInfos(
	region string,
	instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor,
) []demandProductInfo {
	instanceTypes := make([]sdk.InstanceType, 0, len(instanceTypeInfos))
	for instanceType := range instanceTypeInfos {
		instanceTypes = append(instanceTypes, instanceType)
	}
	sort.Slice(instanceTypes, func(i, j int) bool {
		return instanceTypes[i] < instanceTypes[j]
	})

	productInfos := make([]demandProductInfo, 0, len(instanceTypes))
	for _, instanceType := range instanceTypes {
		flavor := instanceTypeInfos[instanceType]
		spec := resourceSpec(flavor)
		productInfos = append(productInfos, demandProductInfo{
			instanceType: instanceType,
			resourceSpec: spec,
			productInfo: bssMdl.DemandProductInfo{
				Id:               string(instanceType),
				CloudServiceType: ecsCloudServiceTypeCode,
				ResourceType:     ecsVMResourceTypeCode,
				ResourceSpec:     spec,
				Region:           region,
				UsageFactor:      durationUsageFactor,
				UsageValue:       decimalPtr(decimal.NewFromInt(1)),
				UsageMeasureId:   hourUsageMeasureID,
				SubscriptionNum:  1,
			},
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

func requestProductInfos(productInfos []demandProductInfo) []bssMdl.DemandProductInfo {
	out := make([]bssMdl.DemandProductInfo, 0, len(productInfos))
	for _, productInfo := range productInfos {
		out = append(out, productInfo.productInfo)
	}
	return out
}

func filterOutProductInfoByResourceSpec(
	productInfos []demandProductInfo,
	resourceSpec string,
) ([]demandProductInfo, []demandProductInfo) {
	filtered := make([]demandProductInfo, 0, len(productInfos))
	removed := make([]demandProductInfo, 0, 1)
	for _, productInfo := range productInfos {
		if productInfo.resourceSpec == resourceSpec {
			removed = append(removed, productInfo)
			continue
		}
		filtered = append(filtered, productInfo)
	}
	return filtered, removed
}

func missingResourceSpecFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	text := err.Error()
	idx := strings.Index(text, productNotFoundPrefix)
	if idx == -1 {
		return "", false
	}
	value := text[idx+len(productNotFoundPrefix):]
	end := len(value)
	for i, ch := range value {
		switch ch {
		case '"', '\\', ' ', ',', ']':
			end = i
			goto done
		}
	}
done:
	resourceSpec := strings.TrimSpace(value[:end])
	if resourceSpec == "" {
		return "", false
	}
	return resourceSpec, true
}

func (p *DefaultProvider) markUnsupportedProducts(ctx context.Context, productInfos []demandProductInfo) {
	instanceTypes := make([]string, 0, len(productInfos))
	resourceSpecs := make([]string, 0, len(productInfos))

	p.muUnsupported.Lock()
	defer p.muUnsupported.Unlock()

	for _, productInfo := range productInfos {
		if _, ok := p.unsupportedProducts[productInfo.instanceType]; ok {
			continue
		}
		p.unsupportedProducts[productInfo.instanceType] = struct{}{}
		instanceTypes = append(instanceTypes, string(productInfo.instanceType))
		resourceSpecs = append(resourceSpecs, productInfo.resourceSpec)
	}
	if len(instanceTypes) == 0 {
		return
	}
	log.FromContext(ctx).WithValues("instance-types", instanceTypes, "resource-specs", resourceSpecs).Info("skipping unsupported on-demand pricing products")
}

func (p *DefaultProvider) filterUnsupportedInstanceTypes(
	instanceTypeInfos map[sdk.InstanceType]ecsMdl.Flavor,
) map[sdk.InstanceType]ecsMdl.Flavor {
	p.muUnsupported.RLock()
	defer p.muUnsupported.RUnlock()

	filtered := make(map[sdk.InstanceType]ecsMdl.Flavor, len(instanceTypeInfos))
	for instanceType, flavor := range instanceTypeInfos {
		if _, ok := p.unsupportedProducts[instanceType]; ok {
			continue
		}
		filtered[instanceType] = flavor
	}
	return filtered
}

func (p *DefaultProvider) unsupportedInstanceTypes() map[sdk.InstanceType]struct{} {
	p.muUnsupported.RLock()
	defer p.muUnsupported.RUnlock()

	unsupported := make(map[sdk.InstanceType]struct{}, len(p.unsupportedProducts))
	for instanceType := range p.unsupportedProducts {
		unsupported[instanceType] = struct{}{}
	}
	return unsupported
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

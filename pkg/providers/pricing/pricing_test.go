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
	"errors"
	"fmt"
	"testing"

	sdkerr "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	bssMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/shopspring/decimal"

	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

func TestUpdateOnDemandPricing(t *testing.T) {
	api := &stubPricingAPI{
		response: &bssMdl.ListOnDemandResourceRatingsResponse{
			ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
				{
					Id:     stringPtr("c6.large.2"),
					Amount: decimalPtr(decimal.NewFromFloat(0.42)),
				},
			},
		},
	}
	projectID := "project-id"
	provider := NewDefaultProvider(api, "cn-north-4", func() string { return projectID })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"c6.large.2": {
			Id:   "c6.large.2",
			Name: "c6.large.2",
		},
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	price, ok := provider.OnDemandPrice("c6.large.2")
	if !ok {
		t.Fatalf("expected on-demand price to exist")
	}
	if price != 0.42 {
		t.Fatalf("expected price 0.42, got %f", price)
	}
	if len(api.requests) != 1 {
		t.Fatalf("expected 1 pricing request, got %d", len(api.requests))
	}

	got := api.requests[0].Body.ProductInfos
	if len(got) != 1 {
		t.Fatalf("expected 1 product info, got %d", len(got))
	}
	if got[0].CloudServiceType != ecsCloudServiceTypeCode {
		t.Fatalf("expected cloud service type %q, got %q", ecsCloudServiceTypeCode, got[0].CloudServiceType)
	}
	if got[0].ResourceType != ecsVMResourceTypeCode {
		t.Fatalf("expected resource type %q, got %q", ecsVMResourceTypeCode, got[0].ResourceType)
	}
	if got[0].ResourceSpec != "c6.large.2.linux" {
		t.Fatalf("expected resource spec c6.large.2.linux, got %q", got[0].ResourceSpec)
	}
	if got[0].Region != "cn-north-4" {
		t.Fatalf("expected region cn-north-4, got %q", got[0].Region)
	}
	if got[0].UsageFactor != durationUsageFactor {
		t.Fatalf("expected usage factor %q, got %q", durationUsageFactor, got[0].UsageFactor)
	}
	if got[0].UsageValue == nil || !got[0].UsageValue.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("expected usage value 1, got %v", got[0].UsageValue)
	}
	if got[0].UsageMeasureId != hourUsageMeasureID {
		t.Fatalf("expected usage measure id %d, got %d", hourUsageMeasureID, got[0].UsageMeasureId)
	}
	if got[0].SubscriptionNum != 1 {
		t.Fatalf("expected subscription num 1, got %d", got[0].SubscriptionNum)
	}
	if api.requests[0].Body.ProjectId != "project-id" {
		t.Fatalf("expected project id project-id, got %q", api.requests[0].Body.ProjectId)
	}
}

func TestUpdateOnDemandPricingUsesLatestProjectID(t *testing.T) {
	api := &stubPricingAPI{
		response: &bssMdl.ListOnDemandResourceRatingsResponse{
			ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
				{
					Id:     stringPtr("c6.large.2"),
					Amount: decimalPtr(decimal.NewFromFloat(0.42)),
				},
			},
		},
	}
	projectID := ""
	provider := NewDefaultProvider(api, "cn-north-4", func() string { return projectID })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"c6.large.2": {
			Id:   "c6.large.2",
			Name: "c6.large.2",
		},
	}

	projectID = "auto-project-id"

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("expected 1 pricing request, got %d", len(api.requests))
	}
	if api.requests[0].Body.ProjectId != "auto-project-id" {
		t.Fatalf("expected project id auto-project-id, got %q", api.requests[0].Body.ProjectId)
	}
}

func TestUpdateOnDemandPricingSkipsUnsupportedProducts(t *testing.T) {
	api := &stubPricingAPI{
		fn: func(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
			productInfos := request.Body.ProductInfos
			if len(productInfos) == 2 {
				return nil, newProductNotFoundError("bad.large.1.linux")
			}
			if len(productInfos) != 1 {
				t.Fatalf("expected retried request with 1 product info, got %d", len(productInfos))
			}
			if productInfos[0].Id != "c6.large.2" {
				t.Fatalf("expected retry to keep c6.large.2, got %q", productInfos[0].Id)
			}
			return &bssMdl.ListOnDemandResourceRatingsResponse{
				ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
					{
						Id:     stringPtr("c6.large.2"),
						Amount: decimalPtr(decimal.NewFromFloat(0.42)),
					},
				},
			}, nil
		},
	}
	provider := NewDefaultProvider(api, "ap-southeast-3", func() string { return "project-id" })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"bad.large.1": {
			Id:   "bad.large.1",
			Name: "bad.large.1",
		},
		"c6.large.2": {
			Id:   "c6.large.2",
			Name: "c6.large.2",
		},
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(api.requests) != 2 {
		t.Fatalf("expected 2 pricing requests, got %d", len(api.requests))
	}
	if _, ok := provider.OnDemandPrice("bad.large.1"); ok {
		t.Fatalf("expected unsupported product to have no cached price")
	}
	if _, ok := provider.OnDemandPrice("c6.large.2"); !ok {
		t.Fatalf("expected supported product to have cached price")
	}

	api.requests = nil
	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error on second refresh, got %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("expected second refresh to skip unsupported product, got %d requests", len(api.requests))
	}
	if got := api.requests[0].Body.ProductInfos; len(got) != 1 || got[0].Id != "c6.large.2" {
		t.Fatalf("expected second refresh to query only c6.large.2, got %+v", got)
	}
}

func TestUpdateOnDemandPricingSkipsUnsupportedProductsByResourceSpec(t *testing.T) {
	api := &stubPricingAPI{
		fn: func(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
			productInfos := request.Body.ProductInfos
			if len(productInfos) == 2 {
				return nil, newProductNotFoundError("bad.large.1.billing.linux")
			}
			if len(productInfos) != 1 {
				t.Fatalf("expected retried request with 1 product info, got %d", len(productInfos))
			}
			if productInfos[0].Id != "c6.large.2" {
				t.Fatalf("expected retry to keep c6.large.2, got %q", productInfos[0].Id)
			}
			return &bssMdl.ListOnDemandResourceRatingsResponse{
				ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
					{
						Id:     stringPtr("c6.large.2"),
						Amount: decimalPtr(decimal.NewFromFloat(0.42)),
					},
				},
			}, nil
		},
	}
	provider := NewDefaultProvider(api, "ap-southeast-3", func() string { return "project-id" })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"bad.large.1": {
			Id:   "bad.large.1.billing",
			Name: "bad.large.1",
		},
		"c6.large.2": {
			Id:   "c6.large.2.billing",
			Name: "c6.large.2",
		},
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := provider.OnDemandPrice("bad.large.1"); ok {
		t.Fatalf("expected unsupported product to have no cached price")
	}

	api.requests = nil
	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error on second refresh, got %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("expected second refresh to skip unsupported product, got %d requests", len(api.requests))
	}
	got := api.requests[0].Body.ProductInfos
	if len(got) != 1 || got[0].Id != "c6.large.2" {
		t.Fatalf("expected second refresh to query only c6.large.2, got %+v", got)
	}
}

func TestUpdateOnDemandPricingClearsCachedPriceWhenAllProductsUnsupported(t *testing.T) {
	api := &stubPricingAPI{
		response: &bssMdl.ListOnDemandResourceRatingsResponse{
			ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
				{
					Id:     stringPtr("bad.large.1"),
					Amount: decimalPtr(decimal.NewFromFloat(0.42)),
				},
			},
		},
	}
	provider := NewDefaultProvider(api, "ap-southeast-3", func() string { return "project-id" })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"bad.large.1": {
			Id:   "bad.large.1",
			Name: "bad.large.1",
		},
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := provider.OnDemandPrice("bad.large.1"); !ok {
		t.Fatalf("expected initial cached price to exist")
	}

	api.requests = nil
	api.fn = func(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
		return nil, newProductNotFoundError("bad.large.1.linux")
	}
	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error when all products become unsupported, got %v", err)
	}
	if len(api.requests) != 1 {
		t.Fatalf("expected 1 pricing request, got %d", len(api.requests))
	}
	if _, ok := provider.OnDemandPrice("bad.large.1"); ok {
		t.Fatalf("expected unsupported product cached price to be cleared")
	}

	api.requests = nil
	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error when all products are already unsupported, got %v", err)
	}
	if len(api.requests) != 0 {
		t.Fatalf("expected no pricing requests after product is marked unsupported, got %d", len(api.requests))
	}
}

func TestUpdateOnDemandPricingClearsCachedPriceWhenNoPriceReturned(t *testing.T) {
	api := &stubPricingAPI{
		responses: []*bssMdl.ListOnDemandResourceRatingsResponse{
			{
				ProductRatingResults: &[]bssMdl.DemandProductRatingResult{
					{
						Id:     stringPtr("c6.large.2"),
						Amount: decimalPtr(decimal.NewFromFloat(0.42)),
					},
				},
			},
			{},
		},
	}
	provider := NewDefaultProvider(api, "cn-north-4", func() string { return "project-id" })
	instanceTypeInfos := map[sdk.InstanceType]ecsMdl.Flavor{
		"c6.large.2": {
			Id:   "c6.large.2",
			Name: "c6.large.2",
		},
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := provider.OnDemandPrice("c6.large.2"); !ok {
		t.Fatalf("expected initial cached price to exist")
	}

	if err := provider.UpdateOnDemandPricing(context.Background(), instanceTypeInfos); err != nil {
		t.Fatalf("expected nil error when response contains no prices, got %v", err)
	}
	if _, ok := provider.OnDemandPrice("c6.large.2"); ok {
		t.Fatalf("expected cached price to be cleared when no price is returned")
	}
}

func TestUnsupportedProductResourceSpecUsesStructuredErrorCode(t *testing.T) {
	resourceSpec, ok := unsupportedProductResourceSpec(newProductNotFoundError("bad.large.1.linux"))
	if !ok {
		t.Fatalf("expected product-not-found error to be recognized")
	}
	if resourceSpec != "bad.large.1.linux" {
		t.Fatalf("expected resource spec bad.large.1.linux, got %q", resourceSpec)
	}
}

func TestUnsupportedProductResourceSpecRejectsNonServiceErrors(t *testing.T) {
	resourceSpec, ok := unsupportedProductResourceSpec(errors.New("Can not find product bad.large.1.linux"))
	if ok {
		t.Fatalf("expected plain error to be ignored, got %q", resourceSpec)
	}
}

func TestProductResourceSpecFromMessageHandlesDelimitersWithoutGoto(t *testing.T) {
	testCases := map[string]string{
		`Can not find product bad.large.1.linux"`: "bad.large.1.linux",
		`Can not find product bad.large.1.linux\`: "bad.large.1.linux",
		`Can not find product bad.large.1.linux,`: "bad.large.1.linux",
		`Can not find product bad.large.1.linux]`: "bad.large.1.linux",
		`Can not find product bad.large.1.linux `: "bad.large.1.linux",
		`prefix Can not find product bad.large.1`: "bad.large.1",
	}

	for message, expected := range testCases {
		t.Run(expected, func(t *testing.T) {
			resourceSpec, ok := productResourceSpecFromMessage(message)
			if !ok {
				t.Fatalf("expected resource spec to be extracted from %q", message)
			}
			if resourceSpec != expected {
				t.Fatalf("expected resource spec %q, got %q", expected, resourceSpec)
			}
		})
	}
}

type stubPricingAPI struct {
	requests  []*bssMdl.ListOnDemandResourceRatingsRequest
	response  *bssMdl.ListOnDemandResourceRatingsResponse
	err       error
	responses []*bssMdl.ListOnDemandResourceRatingsResponse
	fn        func(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error)
}

func (s *stubPricingAPI) ListOnDemandResourceRatings(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
	s.requests = append(s.requests, request)
	if s.fn != nil {
		return s.fn(request)
	}
	if s.err != nil {
		return nil, s.err
	}
	if len(s.responses) != 0 {
		response := s.responses[0]
		s.responses = s.responses[1:]
		return response, nil
	}
	return s.response, nil
}

func newProductNotFoundError(resourceSpec string) error {
	return &sdkerr.ServiceResponseError{
		StatusCode: 400,
		ErrorCode:  bssWrappedProductNotFoundCode,
		ErrorMessage: fmt.Sprintf(
			`Product not found..cse://CSBBillingRatingService/rest/cbc/csbbillingratingservice/v2/inquiry/ondemand_resource[error_code]:%s[msg]:{"error_code":"%s","error_msg":"Can not find product %s","error_list":[]}`,
			bssProductNotFoundCode,
			bssProductNotFoundCode,
			resourceSpec,
		),
	}
}

func stringPtr(v string) *string {
	return &v
}

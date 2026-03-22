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

package huawei

import (
	"sync"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	coreRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
	bss "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2"
	bssMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
)

// NewBSSService returns a PricingAPI that defers building the Huawei BSS client until the first request.
func NewBSSService(region *coreRegion.Region, credential *global.Credentials, httpConfig *config.HttpConfig) PricingAPI {
	if httpConfig == nil {
		httpConfig = config.DefaultHttpConfig()
	}
	return &bssService{
		region:     region,
		credential: credential,
		httpConfig: httpConfig,
	}
}

type bssService struct {
	mu         sync.Mutex
	client     *bss.BssClient
	region     *coreRegion.Region
	credential *global.Credentials
	httpConfig *config.HttpConfig
}

func (s *bssService) getClient() (*bss.BssClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		return s.client, nil
	}

	hcClient, err := bss.BssClientBuilder().
		WithRegion(s.region).
		WithCredential(s.credential).
		WithHttpConfig(s.httpConfig).
		SafeBuild()
	if err != nil {
		return nil, err
	}
	s.client = bss.NewBssClient(hcClient)
	return s.client, nil
}

func (s *bssService) ListOnDemandResourceRatings(request *bssMdl.ListOnDemandResourceRatingsRequest) (*bssMdl.ListOnDemandResourceRatingsResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListOnDemandResourceRatings(request)
}

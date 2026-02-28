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

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	coreRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
	vpc "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2"
	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
)

// NewVPCService returns a VPCAPI that defers building the Huawei VPC client until the first request.
// This avoids calling Huawei IAM APIs (e.g. auto ProjectId discovery) during controller startup.
func NewVPCService(region *coreRegion.Region, credential *basic.Credentials, httpConfig *config.HttpConfig) VPCAPI {
	if httpConfig == nil {
		httpConfig = config.DefaultHttpConfig()
	}
	return &vpcService{
		region:     region,
		credential: credential,
		httpConfig: httpConfig,
	}
}

type vpcService struct {
	mu         sync.Mutex
	client     *vpc.VpcClient
	region     *coreRegion.Region
	credential *basic.Credentials
	httpConfig *config.HttpConfig
}

func (s *vpcService) getClient() (*vpc.VpcClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		return s.client, nil
	}
	hcClient, err := vpc.VpcClientBuilder().
		WithRegion(s.region).
		WithCredential(s.credential).
		WithHttpConfig(s.httpConfig).
		SafeBuild()
	if err != nil {
		return nil, err
	}
	s.client = vpc.NewVpcClient(hcClient)
	return s.client, nil
}

func (s *vpcService) ListSubnets(request *vpcMdl.ListSubnetsRequest) (*vpcMdl.ListSubnetsResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListSubnets(request)
}

func (s *vpcService) ListSecurityGroups(request *vpcMdl.ListSecurityGroupsRequest) (*vpcMdl.ListSecurityGroupsResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListSecurityGroups(request)
}

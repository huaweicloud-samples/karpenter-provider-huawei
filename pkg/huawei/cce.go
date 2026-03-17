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
	cce "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3"
	cceMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
)

// NewCCEService returns a CCEAPI that defers building the Huawei CCE client until the first request.
// This avoids calling Huawei IAM APIs (e.g. auto ProjectId discovery) during controller startup.
func NewCCEService(region *coreRegion.Region, credential *basic.Credentials, httpConfig *config.HttpConfig) CCEAPI {
	if httpConfig == nil {
		httpConfig = config.DefaultHttpConfig()
	}
	return &cceService{
		region:     region,
		credential: credential,
		httpConfig: httpConfig,
	}
}

type cceService struct {
	mu         sync.Mutex
	client     *cce.CceClient
	region     *coreRegion.Region
	credential *basic.Credentials
	httpConfig *config.HttpConfig
}

func (s *cceService) getClient() (*cce.CceClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		return s.client, nil
	}

	hcClient, err := cce.CceClientBuilder().
		WithRegion(s.region).
		WithCredential(s.credential).
		WithHttpConfig(s.httpConfig).
		SafeBuild()
	if err != nil {
		return nil, err
	}
	s.client = cce.NewCceClient(hcClient)
	return s.client, nil
}

func (s *cceService) ShowCluster(request *cceMdl.ShowClusterRequest) (*cceMdl.ShowClusterResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ShowCluster(request)
}

func (s *cceService) CreateNode(request *cceMdl.CreateNodeRequest) (*cceMdl.CreateNodeResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.CreateNode(request)
}

func (s *cceService) DeleteNode(request *cceMdl.DeleteNodeRequest) (*cceMdl.DeleteNodeResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.DeleteNode(request)
}

func (s *cceService) ListNodes(request *cceMdl.ListNodesRequest) (*cceMdl.ListNodesResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListNodes(request)
}

func (s *cceService) ShowNode(request *cceMdl.ShowNodeRequest) (*cceMdl.ShowNodeResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ShowNode(request)
}

func (s *cceService) ShowJob(request *cceMdl.ShowJobRequest) (*cceMdl.ShowJobResponse, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ShowJob(request)
}

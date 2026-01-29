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
	bss "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/bss/v2/model"
	cce "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	cms "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cms/v1/model"
	cse "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cse/v1/model"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	ims "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ims/v2/model"
	vpc "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
)

type ECSAPI interface {
	CreateAutoLaunchGroup(request *cms.CreateAutoLaunchGroupRequest) (*cms.CreateAutoLaunchGroupResponse, error)
	CreateServers(request *ecs.CreateServersRequest) (*ecs.CreateServersResponse, error)
	DeleteServers(request *ecs.DeleteServersRequest) (*ecs.DeleteServersResponse, error)
	ListServersDetails(request *ecs.ListServersDetailsRequest) (*ecs.ListServersDetailsResponse, error)
	BatchCreateServerTags(request *ecs.BatchCreateServerTagsRequest) (*ecs.BatchCreateServerTagsResponse, error)
	ListFlavors(request *cse.ListFlavorsRequest) (*cse.ListFlavorsResponse, error)
	ListSubnets(request *vpc.ListSubnetsRequest) (*vpc.ListSubnetsResponse, error)
	ListSecurityGroups(request *vpc.ListSecurityGroupsRequest) (*vpc.ListSecurityGroupsResponse, error)
	ListImages(request *ims.ListImagesRequest) (*ims.ListImagesResponse, error)
}

type CCEAPI interface {
	ShowCluster(request *cce.ShowClusterRequest) (*cce.ShowClusterResponse, error)
}

type PricingAPI interface {
	ListOnDemandResourceRatings(request *bss.ListOnDemandResourceRatingsRequest) (*bss.ListOnDemandResourceRatingsResponse, error)
}

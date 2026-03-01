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

package nodeclass

import (
	"context"
	"fmt"
	"sort"
	"time"

	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
)

type Subnet struct {
	subnetProvider subnet.Provider
}

func NewSubnetReconciler(subnetProvider subnet.Provider) *Subnet {
	return &Subnet{
		subnetProvider: subnetProvider,
	}
}

func (s *Subnet) Reconcile(ctx context.Context, nodeClass *v1alpha1.ECSNodeClass) (reconcile.Result, error) {
	subnets, err := s.subnetProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting subnets, %w", err)
	}
	if len(subnets) == 0 {
		nodeClass.Status.Subnets = nil
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeSubnetsReady, "SubnetsNotFound", "SubnetSelector did not match any Subnets")
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}
	sort.Slice(subnets, func(i, j int) bool {
		if subnets[i].AvailableIpAddressCount != subnets[j].AvailableIpAddressCount {
			return subnets[i].AvailableIpAddressCount > subnets[j].AvailableIpAddressCount
		}
		return subnets[i].Id < subnets[j].Id
	})
	nodeClass.Status.Subnets = lo.Map(subnets, func(subnet vpcMdl.Subnet, _ int) v1alpha1.Subnet {
		return v1alpha1.Subnet{
			ID:   subnet.Id,
			Zone: subnet.AvailabilityZone,
		}
	})
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeSubnetsReady)
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}

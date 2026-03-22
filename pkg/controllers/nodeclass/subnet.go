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
	"strings"
	"time"

	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
)

const subnetIDLabelKey = "node.kubernetes.io/subnetid"

type Subnet struct {
	kubeClient     client.Client
	subnetProvider subnet.Provider
}

func NewSubnetReconciler(kubeClient client.Client, subnetProvider subnet.Provider) *Subnet {
	return &Subnet{
		kubeClient:     kubeClient,
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
	subnetZones, err := s.subnetZonesFromNodes(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("resolving subnet zones from nodes, %w", err)
	}
	nodeClass.Status.Subnets = lo.Map(subnets, func(subnet vpcMdl.Subnet, _ int) v1alpha1.Subnet {
		zone := strings.TrimSpace(subnet.AvailabilityZone)
		if zone == "" {
			zone = subnetZones[subnet.Id]
		}
		return v1alpha1.Subnet{
			ID:   subnet.Id,
			Zone: zone,
		}
	})
	if unresolved := lo.Filter(nodeClass.Status.Subnets, func(subnet v1alpha1.Subnet, _ int) bool {
		return strings.TrimSpace(subnet.Zone) == ""
	}); len(unresolved) > 0 {
		nodeClass.StatusConditions().SetFalse(
			v1alpha1.ConditionTypeSubnetsReady,
			"SubnetZonesNotResolved",
			fmt.Sprintf("Unable to resolve availability zone for subnets %v", lo.Map(unresolved, func(subnet v1alpha1.Subnet, _ int) string { return subnet.ID })),
		)
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeSubnetsReady)
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}

func (s *Subnet) subnetZonesFromNodes(ctx context.Context) (map[string]string, error) {
	if s.kubeClient == nil {
		return map[string]string{}, nil
	}
	nodes := &corev1.NodeList{}
	if err := s.kubeClient.List(ctx, nodes); err != nil {
		return nil, err
	}
	zones := map[string]string{}
	for _, node := range nodes.Items {
		subnetID := strings.TrimSpace(node.Labels[subnetIDLabelKey])
		if subnetID == "" {
			continue
		}
		zone := strings.TrimSpace(node.Labels[corev1.LabelTopologyZone])
		if zone == "" {
			zone = strings.TrimSpace(node.Labels[corev1.LabelFailureDomainBetaZone])
		}
		if zone == "" {
			continue
		}
		if existing, ok := zones[subnetID]; ok && existing != zone {
			return nil, fmt.Errorf("conflicting zones for subnet %s: %s vs %s", subnetID, existing, zone)
		}
		zones[subnetID] = zone
	}
	return zones, nil
}

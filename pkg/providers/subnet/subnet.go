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

package subnet

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/awslabs/operatorpkg/serrors"
	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/log"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/pretty"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/utils"
)

type Provider interface {
	LivenessProbe(*http.Request) error
	List(context.Context, *v1alpha1.CCENodeClass) ([]vpcMdl.Subnet, error)
	SelectForLaunch(context.Context, *v1alpha1.CCENodeClass, []*cloudprovider.InstanceType, string) (*Subnet, error)
	ReleaseInflightIPs(*Subnet)
}

type DefaultProvider struct {
	sync.Mutex
	vpcapi                  sdk.VPCAPI
	cache                   *cache.Cache
	availableIPAddressCache *cache.Cache
	cm                      *pretty.ChangeMonitor
	inflightIPs             map[string]int32
}

type Subnet struct {
	ID                      string
	AvailableIPAddressCount int32
	reservedIPAddressCount  int32
}

func NewDefaultProvider(vpcapi sdk.VPCAPI, cache *cache.Cache, availableIPAddressCache *cache.Cache) Provider {
	return &DefaultProvider{
		vpcapi:                  vpcapi,
		cache:                   cache,
		availableIPAddressCache: availableIPAddressCache,
		cm:                      pretty.NewChangeMonitor(),
		// inflightIPs is used to track IPs from known launched instances
		inflightIPs: map[string]int32{},
	}
}

func (p *DefaultProvider) LivenessProbe(_ *http.Request) error {
	p.Lock()
	//nolint: staticcheck
	p.Unlock()
	return nil
}

func (p *DefaultProvider) List(ctx context.Context, nodeClass *v1alpha1.CCENodeClass) ([]vpcMdl.Subnet, error) {
	p.Lock()
	defer p.Unlock()

	if len(nodeClass.Spec.SubnetSelectorTerms) == 0 {
		return []vpcMdl.Subnet{}, nil
	}
	hash := utils.GetNodeClassHash(nodeClass)

	if subnets, ok := p.cache.Get(hash); ok {
		// Ensure what's returned from this function is a shallow-copy of the slice (not a deep-copy of the data itself)
		// so that modifications to the ordering of the data don't affect the original
		return append([]vpcMdl.Subnet{}, subnets.([]vpcMdl.Subnet)...), nil
	}
	// Ensure that all the subnets that are returned here are unique
	subnets := map[string]vpcMdl.Subnet{}
	response, err := p.vpcapi.ListSubnets(&vpcMdl.ListSubnetsRequest{
		Limit: lo.ToPtr(int32(500)),
	})
	if err != nil {
		return nil, serrors.Wrap(
			fmt.Errorf("list subnets, %w", err),
			"subnetSelectorTerms", nodeClass.Spec.SubnetSelectorTerms,
			"nodeClass", nodeClass.Name,
		)
	}
	for _, subnet := range lo.FromPtr(response.Subnets) {
		if !matchesSubnetSelectorTerms(subnet, nodeClass.Spec.SubnetSelectorTerms) {
			continue
		}
		subnets[subnet.Id] = subnet
		p.availableIPAddressCache.SetDefault(subnet.Id, subnet.AvailableIpAddressCount)
		delete(p.inflightIPs, subnet.Id) // remove any previously tracked IP addresses since we just refreshed from ECS
	}
	p.cache.SetDefault(hash, lo.Values(subnets))
	if p.cm.HasChanged(fmt.Sprintf("subnets/%s", nodeClass.Name), lo.Keys(subnets)) {
		log.FromContext(ctx).
			WithValues("subnets", lo.Map(lo.Values(subnets), func(s vpcMdl.Subnet, _ int) v1alpha1.Subnet {
				return v1alpha1.Subnet{ID: s.Id}
			})).V(1).Info("discovered subnets")
	}
	return lo.Values(subnets), nil
}

func matchesSubnetSelectorTerm(subnet vpcMdl.Subnet, term v1alpha1.SubnetSelectorTerm) bool {
	if term.ID == "" && term.Name == "" {
		return false
	}
	if term.ID != "" && subnet.Id != term.ID {
		return false
	}
	if term.Name != "" && subnet.Name != term.Name {
		return false
	}
	return true
}

func matchesSubnetSelectorTerms(subnet vpcMdl.Subnet, terms []v1alpha1.SubnetSelectorTerm) bool {
	for _, term := range terms {
		if matchesSubnetSelectorTerm(subnet, term) {
			return true
		}
	}
	return false
}

// SelectForLaunch returns the subnet with the most available IP addresses and reserves the predicted usage.
func (p *DefaultProvider) SelectForLaunch(_ context.Context, nodeClass *v1alpha1.CCENodeClass, instanceTypes []*cloudprovider.InstanceType, capacityType string) (*Subnet, error) {
	if len(nodeClass.Status.Subnets) == 0 {
		return nil, fmt.Errorf("no subnets matched selector %v", nodeClass.Spec.SubnetSelectorTerms)
	}

	p.Lock()
	defer p.Unlock()

	availableIPAddressCount := map[string]int32{}
	for _, subnet := range nodeClass.Status.Subnets {
		if subnetAvailableIP, ok := p.availableIPAddressCache.Get(subnet.ID); ok {
			availableIPAddressCount[subnet.ID] = subnetAvailableIP.(int32)
		}
	}

	// Select the subnet with the most available IPs.
	selectedSubnetID := ""
	var selectedSubnetAvailableIPs int32 = -1
	for _, subnet := range nodeClass.Status.Subnets {
		ipCount := availableIPAddressCount[subnet.ID]
		if ips, ok := p.inflightIPs[subnet.ID]; ok {
			ipCount = ips
		}
		if selectedSubnetID == "" || ipCount > selectedSubnetAvailableIPs {
			selectedSubnetID = subnet.ID
			selectedSubnetAvailableIPs = ipCount
		}
	}
	if selectedSubnetID == "" {
		return nil, fmt.Errorf("no subnets matched selector %v", nodeClass.Spec.SubnetSelectorTerms)
	}

	zones := sets.New[string]()
	for _, it := range instanceTypes {
		for _, of := range it.Offerings {
			if !of.Requirements.Get(karpv1.CapacityTypeLabelKey).Has(capacityType) {
				continue
			}
			zone := of.Requirements.Get(corev1.LabelTopologyZone).Any()
			if zone == "" {
				continue
			}
			zones.Insert(zone)
		}
	}

	// Reserve inflight IPs once per subnet (by ID) using the maximum predicted usage across all offering zones.
	maxPredictedIPsUsed := int32(0)
	for zone := range zones {
		predictedIPsUsed := p.minPods(instanceTypes, scheduling.NewRequirements(
			scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		))
		if predictedIPsUsed > maxPredictedIPsUsed {
			maxPredictedIPsUsed = predictedIPsUsed
		}
	}
	if maxPredictedIPsUsed > 0 {
		prevIPs := selectedSubnetAvailableIPs
		if trackedIPs, ok := p.inflightIPs[selectedSubnetID]; ok {
			prevIPs = trackedIPs
		}
		p.inflightIPs[selectedSubnetID] = prevIPs - maxPredictedIPsUsed
	}
	return &Subnet{
		ID:                      selectedSubnetID,
		AvailableIPAddressCount: selectedSubnetAvailableIPs,
		reservedIPAddressCount:  maxPredictedIPsUsed,
	}, nil
}

// ReleaseInflightIPs releases the predicted reservation after a node creation attempt completes.
// Until instance creation results are wired in, this method always adds back the full reservation made in SelectForLaunch.
func (p *DefaultProvider) ReleaseInflightIPs(subnet *Subnet) {
	if subnet == nil || subnet.ID == "" {
		return
	}

	p.Lock()
	defer p.Unlock()

	reserved := subnet.reservedIPAddressCount
	if reserved == 0 {
		return
	}
	subnet.reservedIPAddressCount = 0

	current, ok := p.inflightIPs[subnet.ID]
	if !ok {
		return
	}
	updated := current + reserved

	if baselineValue, ok := p.availableIPAddressCache.Get(subnet.ID); ok {
		baseline := baselineValue.(int32)
		if updated >= baseline {
			delete(p.inflightIPs, subnet.ID)
			return
		}
	}
	p.inflightIPs[subnet.ID] = updated
}

func (p *DefaultProvider) minPods(instanceTypes []*cloudprovider.InstanceType, reqs scheduling.Requirements) int32 {
	// filter for instance types available in the zone and capacity type being requested
	filteredInstanceTypes := lo.Filter(instanceTypes, func(it *cloudprovider.InstanceType, _ int) bool {
		return it.Offerings.Available().HasCompatible(reqs)
	})
	if len(filteredInstanceTypes) == 0 {
		return 0
	}
	// Get minimum pods to use when selecting a subnet and deducting what will be launched
	minInstanceType := lo.MinBy(filteredInstanceTypes, func(i *cloudprovider.InstanceType, j *cloudprovider.InstanceType) bool {
		iPods := i.Capacity[corev1.ResourcePods]
		jPods := j.Capacity[corev1.ResourcePods]
		return iPods.Cmp(jPods) < 0
	})
	//nolint:gosec
	minPods := minInstanceType.Capacity[corev1.ResourcePods]
	return int32(minPods.Value())
}

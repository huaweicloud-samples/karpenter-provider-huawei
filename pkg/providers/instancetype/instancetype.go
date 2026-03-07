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

package instancetype

import (
	"context"
	"fmt"
	"sort"
	"sync"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/pretty"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

type InstanceType string

type NodeClass interface {
	KubeletConfiguration() *v1alpha1.KubeletConfiguration
	Zones() []string
}

type Provider interface {
	Get(context.Context, NodeClass, InstanceType) (*cloudprovider.InstanceType, error)
	List(context.Context, NodeClass) ([]*cloudprovider.InstanceType, error)
}

type DefaultProvider struct {
	ecsapi                sdk.ECSAPI
	instanceTypesResolver Resolver

	muInstanceTypesInfo sync.RWMutex
	instanceTypesInfo   map[InstanceType]ecsMdl.Flavor

	muInstanceTypesOfferings sync.RWMutex
	instanceTypesOfferings   map[InstanceType]sets.Set[string]
	allZones                 sets.Set[string]

	instanceTypesCache      *cache.Cache
	discoveredCapacityCache *cache.Cache
	cm                      *pretty.ChangeMonitor
}

func NewDefaultProvider(
	instanceTypesCache *cache.Cache,
	discoveredCapacityCache *cache.Cache,
	ecsapi sdk.ECSAPI,
) *DefaultProvider {
	return &DefaultProvider{
		instanceTypesCache:      instanceTypesCache,
		discoveredCapacityCache: discoveredCapacityCache,
		ecsapi:                  ecsapi,
		cm:                      pretty.NewChangeMonitor(),
	}
}

func (p *DefaultProvider) Get(ctx context.Context, nodeClass NodeClass, name InstanceType) (*cloudprovider.InstanceType, error) {
	p.muInstanceTypesInfo.RLock()
	p.muInstanceTypesOfferings.RLock()
	defer p.muInstanceTypesInfo.RUnlock()
	defer p.muInstanceTypesOfferings.RUnlock()

	if len(p.instanceTypesInfo) == 0 {
		return nil, fmt.Errorf("no instance types found")
	}
	if len(p.instanceTypesOfferings) == 0 {
		return nil, fmt.Errorf("no instance types offerings found")
	}
	if len(nodeClass.Zones()) == 0 {
		return nil, fmt.Errorf("no subnets found")
	}

	var instanceType *cloudprovider.InstanceType
	if item, ok := p.instanceTypesCache.Get(p.cacheKey(nodeClass)); ok {
		instanceType, _ = lo.Find(item.([]*cloudprovider.InstanceType), func(i *cloudprovider.InstanceType) bool {
			return InstanceType(i.Name) == name
		})
	}
	if instanceType == nil {
		var err error
		instanceType, err = p.get(ctx, nodeClass, name)
		if err != nil {
			return nil, err
		}
	}
	return p.InjectOfferings(
		ctx,
		[]*cloudprovider.InstanceType{instanceType},
		nodeClass,
		p.allZones,
	)[0], nil
}

func (p *DefaultProvider) List(ctx context.Context, nodeClass NodeClass) ([]*cloudprovider.InstanceType, error) {
	p.muInstanceTypesInfo.RLock()
	p.muInstanceTypesOfferings.RLock()
	defer p.muInstanceTypesInfo.RUnlock()
	defer p.muInstanceTypesOfferings.RUnlock()

	if len(p.instanceTypesInfo) == 0 {
		return nil, fmt.Errorf("no instance types found")
	}
	if len(p.instanceTypesOfferings) == 0 {
		return nil, fmt.Errorf("no instance types offerings found")
	}
	if len(nodeClass.Zones()) == 0 {
		return nil, fmt.Errorf("no subnets found")
	}

	key := p.cacheKey(nodeClass)
	var instanceTypes []*cloudprovider.InstanceType
	if item, ok := p.instanceTypesCache.Get(key); ok {
		// Ensure what's returned from this function is a shallow-copy of the slice (not a deep-copy of the data itself)
		// so that modifications to the ordering of the data don't affect the original
		instanceTypes = item.([]*cloudprovider.InstanceType)
	} else {
		instanceTypes = lo.FilterMapToSlice(p.instanceTypesInfo, func(name InstanceType, info ecsMdl.Flavor) (*cloudprovider.InstanceType, bool) {
			it, err := p.get(ctx, nodeClass, name)
			if err != nil {
				return nil, false
			}
			return it, true
		})
		p.instanceTypesCache.SetDefault(key, instanceTypes)
	}
	return p.InjectOfferings(
		ctx,
		instanceTypes,
		nodeClass,
		p.allZones,
	), nil
}

func (p *DefaultProvider) cacheKey(nodeClass NodeClass) string {
	// Compute fully initialized instance types hash key
	subnetZonesHash, _ := hashstructure.Hash(nodeClass.Zones(), hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	return fmt.Sprintf("%016x", subnetZonesHash)
}

func (p *DefaultProvider) get(ctx context.Context, nodeClass NodeClass, name InstanceType) (*cloudprovider.InstanceType, error) {
	info, ok := p.instanceTypesInfo[name]
	if !ok {
		return nil, fmt.Errorf("instance type %s not found in cache", name)
	}
	it := p.instanceTypesResolver.Resolve(ctx, info, p.instanceTypesOfferings[InstanceType(info.Name)].UnsortedList(), nodeClass)
	if it == nil {
		return nil, fmt.Errorf("failed to generate instance type %s", name)
	}
	if cached, ok := p.discoveredCapacityCache.Get(discoveredCapacityCacheKey(it.Name, nodeClass)); ok {
		it.Capacity[corev1.ResourceMemory] = cached.(resource.Quantity)
	}
	return it, nil
}

func discoveredCapacityCacheKey(instanceType string, nodeClass NodeClass) string {
	hash, _ := hashstructure.Hash(nodeClass.Zones(), hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	return fmt.Sprintf("%s-%016x", instanceType, hash)
}

func (p *DefaultProvider) InjectOfferings(
	ctx context.Context,
	instanceTypes []*cloudprovider.InstanceType,
	nodeClass NodeClass,
	allZones sets.Set[string],
) []*cloudprovider.InstanceType {
	var its []*cloudprovider.InstanceType
	for _, it := range instanceTypes {
		offerings := p.createOfferings(
			ctx,
			it,
			nodeClass,
			allZones,
		)
		// NOTE: By making this copy one level deep, we can modify the offerings without mutating the results from previous
		// GetInstanceTypes calls. This should still be done with caution - it is currently done here in the provider, and
		// once in the instance provider (filterReservedInstanceTypes)
		its = append(its, &cloudprovider.InstanceType{
			Name:         it.Name,
			Requirements: it.Requirements,
			Offerings:    offerings,
			Capacity:     it.Capacity,
			Overhead:     it.Overhead,
		})
	}
	return its
}

func (p *DefaultProvider) createOfferings(
	ctx context.Context,
	it *cloudprovider.InstanceType,
	nodeClass NodeClass,
	allZones sets.Set[string],
) cloudprovider.Offerings {
	_ = ctx
	_ = allZones

	subnetZones := sets.New(nodeClass.Zones()...)
	offeringZones := p.instanceTypesOfferings[InstanceType(it.Name)]

	availableZones := sets.New[string](it.Requirements.Get(corev1.LabelTopologyZone).Values()...)
	if len(availableZones) == 0 {
		availableZones = offeringZones.Intersection(subnetZones)
	}

	capacityTypes := it.Requirements.Get(karpv1.CapacityTypeLabelKey).Values()
	if len(capacityTypes) == 0 {
		capacityTypes = []string{karpv1.CapacityTypeOnDemand}
	}

	zones := availableZones.UnsortedList()
	sort.Strings(zones)
	sort.Strings(capacityTypes)

	offerings := make(cloudprovider.Offerings, 0, len(zones)*len(capacityTypes))
	for _, zone := range zones {
		for _, capacityType := range capacityTypes {
			offerings = append(offerings, &cloudprovider.Offering{
				Available: true,
				Price:     0,
				Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
				),
			})
		}
	}
	return offerings
}

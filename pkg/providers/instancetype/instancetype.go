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
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/awslabs/operatorpkg/serrors"
	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

const listFlavorsPageSize int32 = 1000

type NodeClass interface {
	KubeletConfiguration() *v1alpha1.KubeletConfiguration
	Zones() []string
}

type Provider interface {
	Get(context.Context, NodeClass, sdk.InstanceType) (*cloudprovider.InstanceType, error)
	List(context.Context, NodeClass) ([]*cloudprovider.InstanceType, error)
}

type DefaultProvider struct {
	ecsapi                sdk.ECSAPI
	instanceTypesResolver Resolver
	onDemandPrice         func(sdk.InstanceType) (float64, bool)

	muFetch sync.Mutex

	muInstanceTypes   sync.RWMutex
	instanceTypesInfo map[sdk.InstanceType]ecsMdl.Flavor

	instanceTypesOfferings map[sdk.InstanceType]sets.Set[string]
	allZones               sets.Set[string]

	instanceTypesCache        *cache.Cache
	discoveredCapacityCache   *cache.Cache
	offeringAvailabilityCache *utils.OfferingAvailabilityCache
	cm                        *pretty.ChangeMonitor
}

func NewDefaultProvider(
	ecsapi sdk.ECSAPI,
	instanceTypesCache *cache.Cache,
	discoveredCapacityCache *cache.Cache,
	offeringAvailabilityCache *utils.OfferingAvailabilityCache,
	instanceTypesResolver Resolver,
	onDemandPrice func(sdk.InstanceType) (float64, bool),
) *DefaultProvider {
	return &DefaultProvider{
		ecsapi:                    ecsapi,
		instanceTypesInfo:         map[sdk.InstanceType]ecsMdl.Flavor{},
		instanceTypesOfferings:    map[sdk.InstanceType]sets.Set[string]{},
		instanceTypesResolver:     instanceTypesResolver,
		instanceTypesCache:        instanceTypesCache,
		discoveredCapacityCache:   discoveredCapacityCache,
		offeringAvailabilityCache: offeringAvailabilityCache,
		onDemandPrice:             onDemandPrice,
		cm:                        pretty.NewChangeMonitor(),
	}
}

func (p *DefaultProvider) InstanceTypeInfos() map[sdk.InstanceType]ecsMdl.Flavor {
	p.muInstanceTypes.RLock()
	defer p.muInstanceTypes.RUnlock()

	instanceTypeInfos := make(map[sdk.InstanceType]ecsMdl.Flavor, len(p.instanceTypesInfo))
	for instanceType, info := range p.instanceTypesInfo {
		instanceTypeInfos[instanceType] = info
	}
	return instanceTypeInfos
}

func (p *DefaultProvider) Get(ctx context.Context, nodeClass NodeClass, name sdk.InstanceType) (*cloudprovider.InstanceType, error) {
	p.muInstanceTypes.RLock()
	defer p.muInstanceTypes.RUnlock()

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
			return sdk.InstanceType(i.Name) == name
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
	p.muInstanceTypes.RLock()
	defer p.muInstanceTypes.RUnlock()

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
		instanceTypes = lo.FilterMapToSlice(p.instanceTypesInfo, func(name sdk.InstanceType, info ecsMdl.Flavor) (*cloudprovider.InstanceType, bool) {
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
	itsHash := ""
	if p.instanceTypesResolver != nil {
		itsHash = p.instanceTypesResolver.CacheKey(nodeClass)
	}
	return fmt.Sprintf("%016x-%s", subnetZonesHash, itsHash)
}

func (p *DefaultProvider) get(ctx context.Context, nodeClass NodeClass, name sdk.InstanceType) (*cloudprovider.InstanceType, error) {
	info, ok := p.instanceTypesInfo[name]
	if !ok {
		return nil, fmt.Errorf("instance type %s not found in cache", name)
	}
	if p.instanceTypesResolver == nil {
		return nil, fmt.Errorf("instance types resolver is nil")
	}
	it := p.instanceTypesResolver.Resolve(ctx, info, p.instanceTypesOfferings[sdk.InstanceType(info.Name)].UnsortedList(), nodeClass)
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
	offeringZones := p.instanceTypesOfferings[sdk.InstanceType(it.Name)]

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
			available := true
			if p.offeringAvailabilityCache != nil && p.offeringAvailabilityCache.IsUnavailable(capacityType, sdk.InstanceType(it.Name), zone) {
				available = false
			}
			offerings = append(offerings, &cloudprovider.Offering{
				Available: available,
				Price:     p.offeringPrice(capacityType, sdk.InstanceType(it.Name)),
				Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
				),
			})
		}
	}
	return offerings
}

func (p *DefaultProvider) offeringPrice(capacityType string, instanceType sdk.InstanceType) float64 {
	if capacityType != karpv1.CapacityTypeOnDemand {
		return 0
	}
	if p.onDemandPrice == nil {
		return 0
	}
	price, ok := p.onDemandPrice(instanceType)
	if ok {
		return price
	}
	return math.MaxFloat64
}

func (p *DefaultProvider) Refresh(ctx context.Context) error {
	instanceTypes, err := p.fetchInstanceTypes()
	if err != nil {
		return err
	}

	p.muInstanceTypes.Lock()
	defer p.muInstanceTypes.Unlock()

	p.updateInstanceTypesLocked(ctx, instanceTypes)
	p.updateInstanceTypeOfferingsLocked(ctx, instanceTypes)
	return nil
}

func (p *DefaultProvider) UpdateInstanceTypes(ctx context.Context) error {
	instanceTypes, err := p.fetchInstanceTypes()
	if err != nil {
		return err
	}

	p.muInstanceTypes.Lock()
	defer p.muInstanceTypes.Unlock()
	p.updateInstanceTypesLocked(ctx, instanceTypes)
	return nil
}

func (p *DefaultProvider) updateInstanceTypesLocked(ctx context.Context, instanceTypes []ecsMdl.Flavor) {
	if p.cm.HasChanged("instance-types", instanceTypes) {
		// Only update instanceTypesSeqNum with the instance types have been changed
		// This is to not create new keys with duplicate instance types option
		p.instanceTypesCache.Flush() // None of the cached instance type info is valid when the instance type info changes
		log.FromContext(ctx).WithValues("count", len(instanceTypes)).V(1).Info("discovered instance types")
	}
	p.instanceTypesInfo = lo.SliceToMap(instanceTypes, func(i ecsMdl.Flavor) (sdk.InstanceType, ecsMdl.Flavor) {
		return sdk.InstanceType(i.Name), i
	})
}

func (p *DefaultProvider) UpdateInstanceTypeOfferings(ctx context.Context) error {
	instanceTypes, err := p.fetchInstanceTypes()
	if err != nil {
		return err
	}

	p.muInstanceTypes.Lock()
	defer p.muInstanceTypes.Unlock()
	p.updateInstanceTypeOfferingsLocked(ctx, instanceTypes)
	return nil
}

func (p *DefaultProvider) updateInstanceTypeOfferingsLocked(ctx context.Context, instanceTypes []ecsMdl.Flavor) {
	// Get offerings from ECS
	instanceTypeOfferings := map[sdk.InstanceType]sets.Set[string]{}

	zoneUniverse := sets.New[string]()
	for _, instanceType := range instanceTypes {
		if instanceType.OsExtraSpecs == nil || instanceType.OsExtraSpecs.Condoperationaz == nil {
			continue
		}
		for zone := range parseCondOperationAZ(*instanceType.OsExtraSpecs.Condoperationaz) {
			zoneUniverse.Insert(zone)
		}
	}

	for _, instanceType := range instanceTypes {
		instanceTypeOfferings[sdk.InstanceType(instanceType.Name)] = resolveOfferingZones(zoneUniverse, instanceType.OsExtraSpecs)
	}

	if p.cm.HasChanged("instance-type-offering", instanceTypeOfferings) {
		// Only update instanceTypesSeqNun with the instance type offerings  have been changed
		// This is to not create new keys with duplicate instance type offerings option
		p.instanceTypesCache.Flush() // None of the cached instance type info is valid when the instance type offerings info changes
		log.FromContext(ctx).WithValues("instance-type-count", len(instanceTypeOfferings)).V(1).Info("discovered offerings for instance types")
	}
	p.instanceTypesOfferings = instanceTypeOfferings

	allZones := sets.New[string]()
	for _, offeringZones := range instanceTypeOfferings {
		for zone := range offeringZones {
			allZones.Insert(zone)
		}
	}

	if p.cm.HasChanged("zones", allZones) {
		log.FromContext(ctx).WithValues("zones", allZones.UnsortedList()).V(1).Info("discovered zones")
	}
	p.allZones = allZones
}

func resolveOfferingZones(zoneUniverse sets.Set[string], extraSpecs *ecsMdl.FlavorExtraSpec) sets.Set[string] {
	defaultStatus := "normal"
	if extraSpecs != nil && extraSpecs.Condoperationstatus != nil && strings.TrimSpace(*extraSpecs.Condoperationstatus) != "" {
		defaultStatus = *extraSpecs.Condoperationstatus
	}
	defaultAvailable := condOperationStatusAvailable(defaultStatus)

	azOverrides := map[string]string{}
	if extraSpecs != nil && extraSpecs.Condoperationaz != nil && strings.TrimSpace(*extraSpecs.Condoperationaz) != "" {
		azOverrides = parseCondOperationAZ(*extraSpecs.Condoperationaz)
	}

	zones := sets.New[string]()
	if defaultAvailable {
		for zone := range zoneUniverse {
			zones.Insert(zone)
		}
	}
	for zone, status := range azOverrides {
		if condOperationStatusAvailable(status) {
			zones.Insert(zone)
			continue
		}
		zones.Delete(zone)
	}
	return zones
}

func condOperationStatusAvailable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "normal", "promotion", "obt":
		return true
	case "abandon", "sellout":
		return false
	default:
		// Be permissive by default to avoid accidentally filtering out valid offerings.
		return true
	}
}

func parseCondOperationAZ(condOperationAZ string) map[string]string {
	normalized := strings.NewReplacer("，", ",", "；", ",", ";", ",", "（", "(", "）", ")").Replace(condOperationAZ)

	out := map[string]string{}
	for _, part := range strings.Split(normalized, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		zone := part
		status := ""
		if openParen := strings.Index(part, "("); openParen != -1 {
			if closeParen := strings.LastIndex(part, ")"); closeParen > openParen {
				zone = strings.TrimSpace(part[:openParen])
				status = strings.TrimSpace(part[openParen+1 : closeParen])
			}
		}
		if zone == "" {
			continue
		}
		out[zone] = status
	}
	return out
}

func (p *DefaultProvider) UpdateInstanceTypeCapacityFromNode(ctx context.Context, node *corev1.Node, nodeClaim *karpv1.NodeClaim, nodeClass NodeClass) error {
	// Get mappings for most recent AMIs
	instanceTypeName := node.Labels[corev1.LabelInstanceTypeStable]

	key := discoveredCapacityCacheKey(instanceTypeName, nodeClass)
	actualCapacity := node.Status.Capacity.Memory()
	if cachedCapacity, ok := p.discoveredCapacityCache.Get(key); !ok || actualCapacity.Cmp(cachedCapacity.(resource.Quantity)) < 1 {
		// Update the capacity in the cache if it is less than or equal to the current cached capacity. We update when it's equal to refresh the TTL.
		p.discoveredCapacityCache.SetDefault(key, *actualCapacity)
		// Only log if we haven't discovered the capacity for the instance type yet or the discovered capacity is **less** than the cached capacity
		if !ok || actualCapacity.Cmp(cachedCapacity.(resource.Quantity)) < 0 {
			log.FromContext(ctx).WithValues("memory-capacity", actualCapacity, "instance-type", instanceTypeName).V(1).Info("updating discovered capacity cache")
		}
	}
	return nil
}

func (p *DefaultProvider) fetchInstanceTypes() ([]ecsMdl.Flavor, error) {
	p.muFetch.Lock()
	defer p.muFetch.Unlock()

	request := &ecsMdl.ListFlavorsRequest{
		Limit: lo.ToPtr(listFlavorsPageSize),
	}
	instanceTypes := make([]ecsMdl.Flavor, 0, listFlavorsPageSize)
	for {
		flavorsResponse, err := p.ecsapi.ListFlavors(request)
		if err != nil {
			return nil, serrors.Wrap(fmt.Errorf("list flavors, %w", err))
		}

		pageFlavors := lo.FromPtr(flavorsResponse.Flavors)
		instanceTypes = append(instanceTypes, pageFlavors...)
		if int32(len(pageFlavors)) < listFlavorsPageSize {
			break
		}

		lastFlavorID := pageFlavors[len(pageFlavors)-1].Id
		if lastFlavorID == "" {
			return nil, serrors.Wrap(fmt.Errorf("list flavors pagination, empty flavor id at page boundary"))
		}
		if request.Marker != nil && *request.Marker == lastFlavorID {
			return nil, serrors.Wrap(fmt.Errorf("list flavors pagination, marker did not advance"))
		}
		request.Marker = lo.ToPtr(lastFlavorID)
	}
	return instanceTypes, nil
}

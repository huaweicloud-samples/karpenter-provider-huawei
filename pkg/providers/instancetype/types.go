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
	"strconv"
	"strings"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
)

const (
	ChargeTypeSpot     string = "spot"
	ChargeTypeOnDemand string = "on-demand"
	MemoryAvailable           = "memory.available"
	NodeFSAvailable           = "nodefs.available"
)

type Resolver interface {
	// CacheKey tells the InstanceType cache if something changes about the InstanceTypes or Offerings based on the NodeClass.
	CacheKey(NodeClass) string
	// Resolve generates an InstanceType based on raw Flavor and NodeClass setting data
	Resolve(ctx context.Context, info ecsMdl.Flavor, zones []string, nodeClass NodeClass) *cloudprovider.InstanceType
}

type DefaultResolver struct {
	region string
}

func NewDefaultResolver(region string) *DefaultResolver {
	return &DefaultResolver{
		region: region,
	}
}

func (d *DefaultResolver) CacheKey(nodeClass NodeClass) string {
	kc := &v1alpha1.KubeletConfiguration{}
	if resolved := nodeClass.KubeletConfiguration(); resolved != nil {
		kc = resolved
	}
	kcHash, _ := hashstructure.Hash(kc, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	return fmt.Sprintf("%016x", kcHash)
}

func (d *DefaultResolver) Resolve(ctx context.Context, info ecsMdl.Flavor, zones []string, nodeClass NodeClass) *cloudprovider.InstanceType {
	// !!! Important !!!
	// Any changes to the values passed into the NewInstanceType method will require making updates to the cache key
	// so that Karpenter is able to cache the set of InstanceTypes based on values that alter the set of instance types
	// !!! Important !!!
	kc := &v1alpha1.KubeletConfiguration{}
	if resolved := nodeClass.KubeletConfiguration(); resolved != nil {
		kc = resolved
	}
	return NewInstanceType(
		info,
		d.region,
		zones,
		nodeClass.Zones(),
		kc.MaxPods,
		kc.PodsPerCore,
		kc.KubeReserved,
		kc.SystemReserved,
		kc.EvictionHard,
		kc.EvictionSoft,
	)
}

func NewInstanceType(
	info ecsMdl.Flavor,
	region string,
	offeringZones []string,
	subnetZones []string,
	maxPods *int32,
	podsPerCore *int32,
	kubeReserved map[string]string,
	systemReserved map[string]string,
	evictionHard map[string]string,
	evictionSoft map[string]string,
) *cloudprovider.InstanceType {
	it := &cloudprovider.InstanceType{
		Name:         info.Name,
		Requirements: computeRequirements(info, region, offeringZones, subnetZones),
		Capacity:     computeCapacity(info, maxPods, podsPerCore),
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      kubeReservedResources(cpu(info), pods(info, maxPods, podsPerCore), kubeReserved),
			SystemReserved:    systemReservedResources(systemReserved),
			EvictionThreshold: evictionThreshold(memory(info), evictionHard, evictionSoft),
		},
	}
	return it
}

func computeRequirements(info ecsMdl.Flavor, region string, offeringZones []string, subnetZones []string) scheduling.Requirements {
	capacityTypes := []string{ChargeTypeOnDemand}
	// availableZones is the set of zones where the instance type is offered. subnetZones are informational and only
	// used as a fallback when offerings don't include explicit zone information.
	availableZones := sets.New(offeringZones...)
	if availableZones.Len() == 0 {
		availableZones = sets.New(subnetZones...)
	}

	requirements := scheduling.NewRequirements(
		// Well Known Upstream
		scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, info.Name),
		scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, getArchitecture(info)),
		scheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, string(corev1.Linux)),
		scheduling.NewRequirement(corev1.LabelTopologyRegion, corev1.NodeSelectorOpIn, region),
		scheduling.NewRequirement(corev1.LabelWindowsBuild, corev1.NodeSelectorOpDoesNotExist),
		// Well Known to Karpenter
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityTypes...),
		// Well Known to Huawei
		scheduling.NewRequirement(v1alpha1.LabelInstanceCPU, corev1.NodeSelectorOpIn, info.Vcpus),
		scheduling.NewRequirement(v1alpha1.LabelInstanceMemory, corev1.NodeSelectorOpIn, strconv.Itoa(int(info.Ram))),
		scheduling.NewRequirement(v1alpha1.LabelInstanceNetworkBandwidth, corev1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceSize, corev1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPUName, corev1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPUManufacturer, corev1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPUCount, corev1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPUMemory, corev1.NodeSelectorOpDoesNotExist),
	)
	if availableZones.Len() != 0 {
		requirements.Add(scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, availableZones.UnsortedList()...))
	}
	return requirements
}

func computeCapacity(info ecsMdl.Flavor, maxPods *int32, podsPerCore *int32) corev1.ResourceList {
	resourceList := corev1.ResourceList{
		corev1.ResourceCPU:    *cpu(info),
		corev1.ResourceMemory: *memory(info),
		corev1.ResourcePods:   *pods(info, maxPods, podsPerCore),
	}
	return resourceList
}

func getArchitecture(info ecsMdl.Flavor) string {
	if info.OsExtraSpecs == nil || info.OsExtraSpecs.EcsinstanceArchitecture == nil {
		return "amd64"
	}
	return *info.OsExtraSpecs.EcsinstanceArchitecture
}

func cpu(info ecsMdl.Flavor) *resource.Quantity {
	return resources.Quantity(info.Vcpus)
}

func memory(info ecsMdl.Flavor) *resource.Quantity {
	return resources.Quantity(fmt.Sprintf("%dMi", info.Ram))
}

func pods(info ecsMdl.Flavor, maxPods *int32, podsPerCore *int32) *resource.Quantity {
	var count int64
	switch {
	case maxPods != nil:
		count = int64(lo.FromPtr(maxPods))
	default:
		count = defaultMaxPods(info)
	}
	if lo.FromPtr(podsPerCore) > 0 {
		vcpus, _ := strconv.Atoi(info.Vcpus)
		count = lo.Min([]int64{int64(lo.FromPtr(podsPerCore)) * int64(vcpus), count})
	}
	return resources.Quantity(fmt.Sprint(count))
}

func defaultMaxPods(info ecsMdl.Flavor) int64 {
	count := defaultMaxPodsByMemoryMiB(int64(info.Ram))
	if info.OsExtraSpecs == nil {
		return count
	}
	// CCE Turbo's default maxPods is additionally capped by the flavor's
	// supplementary NIC capacity, which matches the observed node pod capacity.
	if nicCap, ok := parsePositiveInt64(info.OsExtraSpecs.QuotasubNetworkInterfaceMaxNum); ok {
		return minInt64(count, nicCap)
	}
	return count
}

func defaultMaxPodsByMemoryMiB(memoryMiB int64) int64 {
	switch {
	case memoryMiB >= 65536:
		return 110
	case memoryMiB >= 32768:
		return 80
	case memoryMiB >= 16384:
		return 60
	case memoryMiB >= 8192:
		return 40
	default:
		return 20
	}
}

func parsePositiveInt64(v *string) (int64, bool) {
	if v == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(*v), 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func systemReservedResources(systemReserved map[string]string) corev1.ResourceList {
	return lo.MapEntries(systemReserved, func(k string, v string) (corev1.ResourceName, resource.Quantity) {
		return corev1.ResourceName(k), resource.MustParse(v)
	})
}

func evictionThreshold(memory *resource.Quantity, evictionHard map[string]string, evictionSoft map[string]string) corev1.ResourceList {
	overhead := corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse("100Mi"),
	}

	override := corev1.ResourceList{}
	var evictionSignals []map[string]string
	if evictionHard != nil {
		evictionSignals = append(evictionSignals, evictionHard)
	}
	if evictionSoft != nil {
		evictionSignals = append(evictionSignals, evictionSoft)
	}

	for _, m := range evictionSignals {
		temp := corev1.ResourceList{}
		if v, ok := m[MemoryAvailable]; ok {
			temp[corev1.ResourceMemory] = computeEvictionSignal(*memory, v)
		}
		override = resources.MaxResources(override, temp)
	}
	// Assign merges maps from left to right so overrides will always be taken last
	return lo.Assign(overhead, override)
}

// computeEvictionSignal computes the resource quantity value for an eviction signal value, computed off the
// base capacity value if the signal value is a percentage or as a resource quantity if the signal value isn't a percentage
func computeEvictionSignal(capacity resource.Quantity, signalValue string) resource.Quantity {
	if strings.HasSuffix(signalValue, "%") {
		p := mustParsePercentage(signalValue)

		// Calculation is node.capacity * signalValue if percentage
		// From https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/#eviction-signals
		return resource.MustParse(fmt.Sprint(math.Ceil(capacity.AsApproximateFloat64() / 100 * p)))
	}
	return resource.MustParse(signalValue)
}

func mustParsePercentage(v string) float64 {
	p, err := strconv.ParseFloat(strings.Trim(v, "%"), 64)
	if err != nil {
		panic(fmt.Sprintf("expected percentage value to be a float but got %s, %v", v, err))
	}
	// Setting percentage value to 100% is considered disabling the threshold according to
	// https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/
	if p == 100 {
		p = 0
	}
	return p
}

func kubeReservedResources(cpus, pods *resource.Quantity, kubeReserved map[string]string) corev1.ResourceList {
	resources := corev1.ResourceList{
		corev1.ResourceMemory:           resource.MustParse(fmt.Sprintf("%dMi", (11*pods.Value())+255)),
		corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"), // default kube-reserved ephemeral-storage
	}
	// kube-reserved Computed from
	// https://github.com/bottlerocket-os/bottlerocket/pull/1388/files#diff-bba9e4e3e46203be2b12f22e0d654ebd270f0b478dd34f40c31d7aa695620f2fR611
	for _, cpuRange := range []struct {
		start      int64
		end        int64
		percentage float64
	}{
		{start: 0, end: 1000, percentage: 0.06},
		{start: 1000, end: 2000, percentage: 0.01},
		{start: 2000, end: 4000, percentage: 0.005},
		{start: 4000, end: 1 << 31, percentage: 0.0025},
	} {
		if cpu := cpus.MilliValue(); cpu >= cpuRange.start {
			r := float64(cpuRange.end - cpuRange.start)
			if cpu < cpuRange.end {
				r = float64(cpu - cpuRange.start)
			}
			cpuOverhead := resources.Cpu()
			cpuOverhead.Add(*resource.NewMilliQuantity(int64(r*cpuRange.percentage), resource.DecimalSI))
			resources[corev1.ResourceCPU] = *cpuOverhead
		}
	}
	return lo.Assign(resources, lo.MapEntries(kubeReserved, func(k string, v string) (corev1.ResourceName, resource.Quantity) {
		return corev1.ResourceName(k), resource.MustParse(v)
	}))
}

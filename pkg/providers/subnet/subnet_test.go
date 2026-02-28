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
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
)

func TestUpdateInflightIPs_ReleasesSingleReservationBackToBaseline(t *testing.T) {
	ctx := context.Background()

	const (
		subnetID      = "subnet-123"
		zone          = "zone-a"
		capacityType  = "on-demand"
		baselineIPs   = int32(100)
		instancePods  = int64(10)
		expectedAfter = int32(90)
	)

	p := newTestProvider()
	p.availableIPAddressCache.SetDefault(subnetID, baselineIPs)

	instanceTypes := []*cloudprovider.InstanceType{
		newTestInstanceType(zone, capacityType, instancePods),
	}
	nodeClass := &v1alpha1.ECSNodeClass{
		Status: v1alpha1.ECSNodeClassStatus{
			Subnets: []v1alpha1.Subnet{
				{ID: subnetID, Zone: zone},
			},
		},
	}

	zonalSubnets, err := p.ZonalSubnetsForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("ZonalSubnetsForLaunch() error = %v", err)
	}
	chosen := zonalSubnets[zone]
	if chosen == nil || chosen.ID != subnetID {
		t.Fatalf("expected chosen subnet %q in zone %q, got %#v", subnetID, zone, chosen)
	}
	if got := p.inflightIPs[subnetID]; got != expectedAfter {
		t.Fatalf("expected inflightIPs[%q]=%d after reservation, got %d", subnetID, expectedAfter, got)
	}

	p.UpdateInflightIPs(nil, nil, instanceTypes, []*Subnet{chosen}, capacityType)
	if _, ok := p.inflightIPs[subnetID]; ok {
		t.Fatalf("expected inflightIPs[%q] cleared after release, got %d", subnetID, p.inflightIPs[subnetID])
	}
}

func TestUpdateInflightIPs_ReleasesReservationsInSteps(t *testing.T) {
	ctx := context.Background()

	const (
		subnetID     = "subnet-123"
		zone         = "zone-a"
		capacityType = "on-demand"
		baselineIPs  = int32(100)
		instancePods = int64(10)
	)

	p := newTestProvider()
	p.availableIPAddressCache.SetDefault(subnetID, baselineIPs)

	instanceTypes := []*cloudprovider.InstanceType{
		newTestInstanceType(zone, capacityType, instancePods),
	}
	nodeClass := &v1alpha1.ECSNodeClass{
		Status: v1alpha1.ECSNodeClassStatus{
			Subnets: []v1alpha1.Subnet{
				{ID: subnetID, Zone: zone},
			},
		},
	}

	_, err := p.ZonalSubnetsForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("ZonalSubnetsForLaunch() error = %v", err)
	}
	zonalSubnets, err := p.ZonalSubnetsForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("ZonalSubnetsForLaunch() error = %v", err)
	}
	chosen := zonalSubnets[zone]
	if got := p.inflightIPs[subnetID]; got != 80 {
		t.Fatalf("expected inflightIPs[%q]=80 after two reservations, got %d", subnetID, got)
	}

	p.UpdateInflightIPs(nil, nil, instanceTypes, []*Subnet{chosen}, capacityType)
	if got := p.inflightIPs[subnetID]; got != 90 {
		t.Fatalf("expected inflightIPs[%q]=90 after releasing one reservation, got %d", subnetID, got)
	}

	p.UpdateInflightIPs(nil, nil, instanceTypes, []*Subnet{chosen}, capacityType)
	if _, ok := p.inflightIPs[subnetID]; ok {
		t.Fatalf("expected inflightIPs[%q] cleared after releasing two reservations, got %d", subnetID, p.inflightIPs[subnetID])
	}
}

func newTestProvider() *DefaultProvider {
	c := cache.New(5*time.Minute, 10*time.Minute)
	available := cache.New(5*time.Minute, 10*time.Minute)
	return NewDefaultProvider(nil, c, available).(*DefaultProvider)
}

func newTestInstanceType(zone, capacityType string, pods int64) *cloudprovider.InstanceType {
	reqs := scheduling.NewRequirements(
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityType),
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
	)
	return &cloudprovider.InstanceType{
		Name: "test.it",
		Offerings: cloudprovider.Offerings{
			&cloudprovider.Offering{
				Available:    true,
				Requirements: reqs,
			},
		},
		Capacity: corev1.ResourceList{
			corev1.ResourcePods: *resource.NewQuantity(pods, resource.DecimalSI),
		},
	}
}

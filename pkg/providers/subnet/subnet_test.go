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

	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

func TestReleaseInflightIPsReleasesSingleReservationBackToBaseline(t *testing.T) {
	ctx := context.Background()

	const (
		subnetID      = "subnet-123"
		capacityType  = "on-demand"
		baselineIPs   = int32(100)
		instancePods  = int64(10)
		expectedAfter = int32(90)
	)

	p := newTestProvider()
	p.availableIPAddressCache.SetDefault(subnetID, baselineIPs)

	instanceTypes := []*cloudprovider.InstanceType{
		newTestInstanceType(instancePods),
	}
	nodeClass := &v1alpha1.CCENodeClass{
		Status: v1alpha1.CCENodeClassStatus{
			Subnets: []v1alpha1.Subnet{{ID: subnetID}},
		},
	}

	chosen, err := p.SelectForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("SelectForLaunch() error = %v", err)
	}
	if chosen == nil || chosen.ID != subnetID {
		t.Fatalf("expected chosen subnet %q, got %#v", subnetID, chosen)
	}
	if got := p.inflightIPs[subnetID]; got != expectedAfter {
		t.Fatalf("expected inflightIPs[%q]=%d after reservation, got %d", subnetID, expectedAfter, got)
	}

	p.ReleaseInflightIPs(chosen)
	if _, ok := p.inflightIPs[subnetID]; ok {
		t.Fatalf("expected inflightIPs[%q] cleared after release, got %d", subnetID, p.inflightIPs[subnetID])
	}
}

func TestReleaseInflightIPsReleasesReservationsInSteps(t *testing.T) {
	ctx := context.Background()

	const (
		subnetID     = "subnet-123"
		capacityType = "on-demand"
		baselineIPs  = int32(100)
		instancePods = int64(10)
	)

	p := newTestProvider()
	p.availableIPAddressCache.SetDefault(subnetID, baselineIPs)

	instanceTypes := []*cloudprovider.InstanceType{
		newTestInstanceType(instancePods),
	}
	nodeClass := &v1alpha1.CCENodeClass{
		Status: v1alpha1.CCENodeClassStatus{
			Subnets: []v1alpha1.Subnet{{ID: subnetID}},
		},
	}

	firstReservation, err := p.SelectForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("SelectForLaunch() error = %v", err)
	}
	secondReservation, err := p.SelectForLaunch(ctx, nodeClass, instanceTypes, capacityType)
	if err != nil {
		t.Fatalf("SelectForLaunch() error = %v", err)
	}
	if got := p.inflightIPs[subnetID]; got != 80 {
		t.Fatalf("expected inflightIPs[%q]=80 after two reservations, got %d", subnetID, got)
	}

	p.ReleaseInflightIPs(firstReservation)
	if got := p.inflightIPs[subnetID]; got != 90 {
		t.Fatalf("expected inflightIPs[%q]=90 after releasing one reservation, got %d", subnetID, got)
	}

	p.ReleaseInflightIPs(secondReservation)
	if _, ok := p.inflightIPs[subnetID]; ok {
		t.Fatalf("expected inflightIPs[%q] cleared after releasing two reservations, got %d", subnetID, p.inflightIPs[subnetID])
	}
}

func TestReleaseInflightIPsReleasesMatchingReservationOutOfOrder(t *testing.T) {
	const (
		subnetID     = "subnet-123"
		capacityType = "on-demand"
		baselineIPs  = int32(100)
	)
	p := newTestProvider()
	p.availableIPAddressCache.SetDefault(subnetID, baselineIPs)
	nodeClass := &v1alpha1.CCENodeClass{
		Status: v1alpha1.CCENodeClassStatus{Subnets: []v1alpha1.Subnet{{ID: subnetID}}},
	}
	smallInstanceTypes := []*cloudprovider.InstanceType{newTestInstanceType(10)}
	largeInstanceTypes := []*cloudprovider.InstanceType{newTestInstanceType(20)}

	smallReservation, err := p.SelectForLaunch(context.Background(), nodeClass, smallInstanceTypes, capacityType)
	if err != nil {
		t.Fatalf("selecting subnet for small instance: %v", err)
	}
	largeReservation, err := p.SelectForLaunch(context.Background(), nodeClass, largeInstanceTypes, capacityType)
	if err != nil {
		t.Fatalf("selecting subnet for large instance: %v", err)
	}
	if got := p.inflightIPs[subnetID]; got != 70 {
		t.Fatalf("expected 70 IPs after both reservations, got %d", got)
	}

	p.ReleaseInflightIPs(largeReservation)
	if got := p.inflightIPs[subnetID]; got != 90 {
		t.Fatalf("expected releasing the 20-IP reservation first to restore 90 IPs, got %d", got)
	}
	p.ReleaseInflightIPs(smallReservation)
	if _, ok := p.inflightIPs[subnetID]; ok {
		t.Fatalf("expected all reservations to be released")
	}
}

func TestSelectForLaunchReturnsOneSubnetAcrossOfferingZones(t *testing.T) {
	p := newTestProvider()
	p.availableIPAddressCache.SetDefault("subnet-123", int32(100))
	nodeClass := &v1alpha1.CCENodeClass{
		Status: v1alpha1.CCENodeClassStatus{
			Subnets: []v1alpha1.Subnet{{ID: "subnet-123"}},
		},
	}
	instanceTypes := []*cloudprovider.InstanceType{
		{
			Name: "test.it",
			Offerings: cloudprovider.Offerings{
				{Available: true, Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, "on-demand"),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-a"),
				)},
				{Available: true, Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, "on-demand"),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-b"),
				)},
			},
			Capacity: corev1.ResourceList{
				corev1.ResourcePods: *resource.NewQuantity(10, resource.DecimalSI),
			},
		},
	}

	selected, err := p.SelectForLaunch(context.Background(), nodeClass, instanceTypes, "on-demand")
	if err != nil {
		t.Fatalf("SelectForLaunch() error = %v", err)
	}
	if selected == nil || selected.ID != "subnet-123" {
		t.Fatalf("expected subnet-123 to be selected, got %#v", selected)
	}
}

func TestList_MatchesIDOnly(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-a", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
			{Id: "subnet-456", Name: "subnet-b", AvailabilityZone: "zone-b", AvailableIpAddressCount: 20},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{ID: "subnet-123"},
			},
		},
	}

	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got, "subnet-123")
	if vpcapi.lastListSubnetsRequest == nil {
		t.Fatalf("expected ListSubnets request to be recorded")
	}
	if vpcapi.lastListSubnetsRequest.VpcId != nil {
		t.Fatalf("expected ListSubnetsRequest.VpcId to be nil, got %#v", vpcapi.lastListSubnetsRequest)
	}
}

func TestList_MatchesNameOnly(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-a", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
			{Id: "subnet-456", Name: "subnet-b", AvailabilityZone: "zone-b", AvailableIpAddressCount: 20},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{Name: "subnet-b"},
			},
		},
	}

	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got, "subnet-456")
}

func TestList_MatchesIDAndName(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-a", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
			{Id: "subnet-456", Name: "subnet-a", AvailabilityZone: "zone-b", AvailableIpAddressCount: 20},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{ID: "subnet-123", Name: "subnet-a"},
			},
		},
	}

	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got, "subnet-123")
}

func TestList_MatchesIDAndNameRequiresBoth(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-b", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{ID: "subnet-123", Name: "subnet-a"},
			},
		},
	}

	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got)
}

func TestList_MatchesMultipleTermsOR(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-a", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
			{Id: "subnet-456", Name: "subnet-b", AvailabilityZone: "zone-b", AvailableIpAddressCount: 20},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{ID: "subnet-123"},
				{Name: "subnet-b"},
			},
		},
	}

	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got, "subnet-123", "subnet-456")
}

func TestList_DedupesWhenMultipleTermsMatchSameSubnet(t *testing.T) {
	ctx := context.Background()

	vpcapi := &fakeVPCAPI{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", Name: "subnet-a", AvailabilityZone: "zone-a", AvailableIpAddressCount: 10},
		},
	}
	p := newTestProviderWithVPC(vpcapi)
	nodeClass := &v1alpha1.CCENodeClass{
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{
				{ID: "subnet-123"},
				{Name: "subnet-a"},
			},
		},
	}

	_, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got, err := p.List(ctx, nodeClass)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertSubnetIDs(t, got, "subnet-123")
	if vpcapi.listSubnetsCalls != 1 {
		t.Fatalf("expected ListSubnets called once due to caching, got %d", vpcapi.listSubnetsCalls)
	}
}

func newTestProvider() *DefaultProvider {
	return newTestProviderWithVPC(nil)
}

func newTestProviderWithVPC(vpcapi sdk.VPCAPI) *DefaultProvider {
	c := cache.New(5*time.Minute, 10*time.Minute)
	available := cache.New(5*time.Minute, 10*time.Minute)
	return NewDefaultProvider(vpcapi, c, available).(*DefaultProvider)
}

func newTestInstanceType(pods int64) *cloudprovider.InstanceType {
	reqs := scheduling.NewRequirements(
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, "on-demand"),
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-a"),
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

type fakeVPCAPI struct {
	subnets                []vpcMdl.Subnet
	listSubnetsCalls       int
	lastListSubnetsRequest *vpcMdl.ListSubnetsRequest
}

func (f *fakeVPCAPI) ListSubnets(request *vpcMdl.ListSubnetsRequest) (*vpcMdl.ListSubnetsResponse, error) {
	f.listSubnetsCalls++
	f.lastListSubnetsRequest = request
	subnetsCopy := append([]vpcMdl.Subnet{}, f.subnets...)
	return &vpcMdl.ListSubnetsResponse{Subnets: &subnetsCopy}, nil
}

func (f *fakeVPCAPI) ListSecurityGroups(_ *vpcMdl.ListSecurityGroupsRequest) (*vpcMdl.ListSecurityGroupsResponse, error) {
	return &vpcMdl.ListSecurityGroupsResponse{}, nil
}

func assertSubnetIDs(t *testing.T, subnets []vpcMdl.Subnet, wantIDs ...string) {
	t.Helper()

	gotIDs := map[string]struct{}{}
	for _, s := range subnets {
		gotIDs[s.Id] = struct{}{}
	}
	if len(gotIDs) != len(subnets) {
		t.Fatalf("expected deduped subnets by ID, got %d items with %d unique IDs", len(subnets), len(gotIDs))
	}
	if len(subnets) != len(wantIDs) {
		t.Fatalf("expected %d subnets, got %d (%v)", len(wantIDs), len(subnets), subnets)
	}
	for _, id := range wantIDs {
		if _, ok := gotIDs[id]; !ok {
			t.Fatalf("expected subnet ID %q in results, got %v", id, subnets)
		}
	}
}

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
	"testing"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
)

func TestFetchInstanceTypes_PaginatesListFlavors(t *testing.T) {
	firstPage := make([]ecsMdl.Flavor, listFlavorsPageSize)
	for i := range firstPage {
		id := fmt.Sprintf("flavor-%04d", i)
		firstPage[i] = ecsMdl.Flavor{Id: id, Name: id}
	}
	secondPage := []ecsMdl.Flavor{
		{Id: "flavor-1000", Name: "flavor-1000"},
		{Id: "flavor-1001", Name: "flavor-1001"},
	}

	fakeAPI := &fakeECSAPI{
		listFlavorsFunc: func(request *ecsMdl.ListFlavorsRequest) (*ecsMdl.ListFlavorsResponse, error) {
			if request.Limit == nil || *request.Limit != listFlavorsPageSize {
				t.Fatalf("expected list flavors limit %d, got %+v", listFlavorsPageSize, request.Limit)
			}
			switch {
			case request.Marker == nil:
				return &ecsMdl.ListFlavorsResponse{Flavors: &firstPage}, nil
			case *request.Marker == firstPage[len(firstPage)-1].Id:
				return &ecsMdl.ListFlavorsResponse{Flavors: &secondPage}, nil
			default:
				t.Fatalf("unexpected marker %q", *request.Marker)
				return nil, nil
			}
		},
	}

	p := NewDefaultProvider(fakeAPI, nil, nil, nil, func(sdk.InstanceType) (float64, bool) {
		return 0, false
	})

	flavors, err := p.fetchInstanceTypes()
	if err != nil {
		t.Fatalf("expected no error fetching instance types, got %v", err)
	}
	if len(flavors) != len(firstPage)+len(secondPage) {
		t.Fatalf("expected %d flavors, got %d", len(firstPage)+len(secondPage), len(flavors))
	}
	if fakeAPI.listFlavorsCalls != 2 {
		t.Fatalf("expected 2 list flavors calls, got %d", fakeAPI.listFlavorsCalls)
	}
	if len(fakeAPI.listFlavorsRequests) != 2 {
		t.Fatalf("expected 2 captured requests, got %d", len(fakeAPI.listFlavorsRequests))
	}
	if fakeAPI.listFlavorsRequests[0].Marker != nil {
		t.Fatalf("expected first request marker to be nil, got %q", *fakeAPI.listFlavorsRequests[0].Marker)
	}
	if fakeAPI.listFlavorsRequests[1].Marker == nil || *fakeAPI.listFlavorsRequests[1].Marker != firstPage[len(firstPage)-1].Id {
		t.Fatalf("expected second request marker %q, got %+v", firstPage[len(firstPage)-1].Id, fakeAPI.listFlavorsRequests[1].Marker)
	}

	cachedFlavors, err := p.fetchInstanceTypes()
	if err != nil {
		t.Fatalf("expected cached fetch to succeed, got %v", err)
	}
	if len(cachedFlavors) != len(flavors) {
		t.Fatalf("expected cached flavors length %d, got %d", len(flavors), len(cachedFlavors))
	}
	if fakeAPI.listFlavorsCalls != 2 {
		t.Fatalf("expected cached fetch not to call list flavors again, got %d calls", fakeAPI.listFlavorsCalls)
	}
}

func TestParseCondOperationAZ(t *testing.T) {
	got := parseCondOperationAZ("cn-south-2b(normal), cn-south-1c(sellout),cn-south-1e(obt)，cn-south-1f(promotion); cn-south-1g(abandon)")

	assertMapEntry(t, got, "cn-south-2b", "normal")
	assertMapEntry(t, got, "cn-south-1c", "sellout")
	assertMapEntry(t, got, "cn-south-1e", "obt")
	assertMapEntry(t, got, "cn-south-1f", "promotion")
	assertMapEntry(t, got, "cn-south-1g", "abandon")
}

func TestResolveOfferingZones_DefaultAbandon_WithOverrides(t *testing.T) {
	universe := sets.New[string]("cn-south-2b", "cn-south-1c", "cn-south-1e", "cn-south-1f", "cn-south-1g")
	extra := &ecsMdl.FlavorExtraSpec{
		Condoperationstatus: stringPtr("abandon"),
		Condoperationaz:     stringPtr("cn-south-2b(normal), cn-south-1c(sellout), cn-south-1e(obt), cn-south-1f(promotion)"),
	}

	zones := resolveOfferingZones(universe, extra)
	assertZones(t, zones, "cn-south-2b", "cn-south-1e", "cn-south-1f")
}

func TestResolveOfferingZones_DefaultNormal_WithSelloutException(t *testing.T) {
	universe := sets.New[string]("cn-south-2b", "cn-south-1c", "cn-south-1e")
	extra := &ecsMdl.FlavorExtraSpec{
		Condoperationstatus: stringPtr("normal"),
		Condoperationaz:     stringPtr("cn-south-1c(sellout)"),
	}

	zones := resolveOfferingZones(universe, extra)
	assertZones(t, zones, "cn-south-2b", "cn-south-1e")
}

func TestResolveOfferingZones_DefaultSellout_WithNormalOverride(t *testing.T) {
	universe := sets.New[string]("cn-south-1c", "cn-south-1e")
	extra := &ecsMdl.FlavorExtraSpec{
		Condoperationstatus: stringPtr("sellout"),
		Condoperationaz:     stringPtr("cn-south-1e(normal)"),
	}

	zones := resolveOfferingZones(universe, extra)
	assertZones(t, zones, "cn-south-1e")
}

func TestComputeRequirements_UsesSubnetZonesWhenOfferingZonesEmpty(t *testing.T) {
	flavor := ecsMdl.Flavor{
		Name:  "c3.large",
		Ram:   8192,
		Vcpus: "2",
	}

	subnetZones := []string{"cn-south-2b", "cn-south-1c", "cn-south-1e", "cn-south-1f", "cn-south-1g"}
	reqs := computeRequirements(flavor, "cn-south-1", nil, subnetZones)
	gotZones := sets.New(reqs.Get(corev1.LabelTopologyZone).Values()...)
	if gotZones.Len() != 5 || !gotZones.HasAll(subnetZones...) {
		t.Fatalf("expected zones %v, got %v", subnetZones, gotZones.UnsortedList())
	}

	gotRegions := sets.New(reqs.Get(corev1.LabelTopologyRegion).Values()...)
	if gotRegions.Len() != 1 || !gotRegions.Has("cn-south-1") {
		t.Fatalf("expected region {cn-south-1}, got %v", gotRegions.UnsortedList())
	}

	gotOS := sets.New(reqs.Get(corev1.LabelOSStable).Values()...)
	if gotOS.Len() != 1 || !gotOS.Has(string(corev1.Linux)) {
		t.Fatalf("expected os {linux}, got %v", gotOS.UnsortedList())
	}
}

func TestCreateOfferings_InjectsOnDemandPrice(t *testing.T) {
	p := NewDefaultProvider(nil, nil, nil, nil, func(instanceType sdk.InstanceType) (float64, bool) {
		if instanceType != "c6.large.2" {
			return 0, false
		}
		return 0.42, true
	})
	p.instanceTypesOfferings = map[sdk.InstanceType]sets.Set[string]{
		"c6.large.2": sets.New[string]("cn-north-4a"),
	}

	offerings := p.createOfferings(context.Background(), &cloudprovider.InstanceType{
		Name: "c6.large.2",
		Requirements: computeRequirements(ecsMdl.Flavor{
			Name:  "c6.large.2",
			Ram:   4096,
			Vcpus: "2",
		}, "cn-north-4", []string{"cn-north-4a"}, []string{"cn-north-4a"}),
	}, fakeNodeClass{zones: []string{"cn-north-4a"}}, sets.New[string]("cn-north-4a"))

	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	if offerings[0].Price != 0.42 {
		t.Fatalf("expected price 0.42, got %f", offerings[0].Price)
	}
}

func TestCreateOfferings_UnknownOnDemandPriceUsesMaxFloat(t *testing.T) {
	p := NewDefaultProvider(nil, nil, nil, nil, func(sdk.InstanceType) (float64, bool) {
		return 0, false
	})
	p.instanceTypesOfferings = map[sdk.InstanceType]sets.Set[string]{
		"c6.large.2": sets.New[string]("cn-north-4a"),
	}

	offerings := p.createOfferings(context.Background(), &cloudprovider.InstanceType{
		Name: "c6.large.2",
		Requirements: computeRequirements(ecsMdl.Flavor{
			Name:  "c6.large.2",
			Ram:   4096,
			Vcpus: "2",
		}, "cn-north-4", []string{"cn-north-4a"}, []string{"cn-north-4a"}),
	}, fakeNodeClass{zones: []string{"cn-north-4a"}}, sets.New[string]("cn-north-4a"))

	if len(offerings) != 1 {
		t.Fatalf("expected 1 offering, got %d", len(offerings))
	}
	if offerings[0].Price != math.MaxFloat64 {
		t.Fatalf("expected price %f, got %f", math.MaxFloat64, offerings[0].Price)
	}
}

type fakeNodeClass struct {
	zones []string
}

type fakeECSAPI struct {
	listFlavorsFunc     func(*ecsMdl.ListFlavorsRequest) (*ecsMdl.ListFlavorsResponse, error)
	listFlavorsCalls    int
	listFlavorsRequests []*ecsMdl.ListFlavorsRequest
}

func (f fakeNodeClass) KubeletConfiguration() *v1alpha1.KubeletConfiguration {
	return nil
}

func (f fakeNodeClass) Zones() []string {
	return f.zones
}

func (f *fakeECSAPI) ListServersDetails(*ecsMdl.ListServersDetailsRequest) (*ecsMdl.ListServersDetailsResponse, error) {
	return &ecsMdl.ListServersDetailsResponse{}, nil
}

func (f *fakeECSAPI) BatchCreateServerTags(*ecsMdl.BatchCreateServerTagsRequest) (*ecsMdl.BatchCreateServerTagsResponse, error) {
	return &ecsMdl.BatchCreateServerTagsResponse{}, nil
}

func (f *fakeECSAPI) ListFlavors(request *ecsMdl.ListFlavorsRequest) (*ecsMdl.ListFlavorsResponse, error) {
	f.listFlavorsCalls++
	requestCopy := *request
	f.listFlavorsRequests = append(f.listFlavorsRequests, &requestCopy)
	if f.listFlavorsFunc == nil {
		return &ecsMdl.ListFlavorsResponse{}, nil
	}
	return f.listFlavorsFunc(request)
}

func stringPtr(v string) *string {
	return &v
}

func assertMapEntry(t *testing.T, m map[string]string, key, want string) {
	t.Helper()
	if got, ok := m[key]; !ok || got != want {
		t.Fatalf("expected %q=%q, got %q (present=%v)", key, want, got, ok)
	}
}

func assertZones(t *testing.T, zones sets.Set[string], want ...string) {
	t.Helper()
	wantSet := sets.New[string](want...)
	if zones.Len() != wantSet.Len() || !zones.HasAll(want...) {
		t.Fatalf("expected zones=%v, got %v", wantSet.UnsortedList(), zones.UnsortedList())
	}
}

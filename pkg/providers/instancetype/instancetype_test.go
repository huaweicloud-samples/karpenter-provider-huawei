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
	"testing"

	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

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

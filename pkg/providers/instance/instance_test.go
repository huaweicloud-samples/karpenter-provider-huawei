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

package instance

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
)

func TestNodeIDFromProviderID(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		want       string
		wantErr    bool
	}{
		{
			name:       "RawUUID",
			providerID: "123e4567-e89b-12d3-a456-426614174000",
			want:       "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:       "RawUUIDWithSpaces",
			providerID: "  123e4567-e89b-12d3-a456-426614174000  ",
			want:       "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:       "Empty",
			providerID: "",
			wantErr:    true,
		},
		{
			name:       "WhitespaceOnly",
			providerID: "   ",
			wantErr:    true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nodeIDFromProviderID(tt.providerID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("nodeIDFromProviderID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBuildCreateCandidates_SortedStable(t *testing.T) {
	onDemandReqs := scheduling.NewRequirements(
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
	)

	newOffering := func(zone string) *cloudprovider.Offering {
		return &cloudprovider.Offering{
			Available: true,
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
			),
		}
	}

	itB := &cloudprovider.InstanceType{
		Name: "it-b",
		Offerings: cloudprovider.Offerings{
			newOffering("zone-b"),
			newOffering("zone-a"),
		},
	}
	itA := &cloudprovider.InstanceType{
		Name: "it-a",
		Offerings: cloudprovider.Offerings{
			newOffering("zone-b"),
			newOffering("zone-a"),
		},
	}

	zonalSubnets := map[string]*subnet.Subnet{
		"zone-a": {ID: "subnet-a", Zone: "zone-a", AvailableIPAddressCount: 100},
		"zone-b": {ID: "subnet-b", Zone: "zone-b", AvailableIPAddressCount: 100},
	}

	candidates := buildCreateCandidates([]*cloudprovider.InstanceType{itB, itA}, onDemandReqs, zonalSubnets)
	if len(candidates) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(candidates))
	}
	got := []string{
		candidates[0].zone + "/" + candidates[0].instanceType.Name,
		candidates[1].zone + "/" + candidates[1].instanceType.Name,
		candidates[2].zone + "/" + candidates[2].instanceType.Name,
		candidates[3].zone + "/" + candidates[3].instanceType.Name,
	}
	want := []string{"zone-a/it-a", "zone-a/it-b", "zone-b/it-a", "zone-b/it-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
	if candidates[0].subnetID != "subnet-a" || candidates[2].subnetID != "subnet-b" {
		t.Fatalf("expected subnet mapping zone-a->subnet-a, zone-b->subnet-b, got %#v", candidates)
	}
}

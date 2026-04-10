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

package v1alpha1

import "testing"

func TestECSNodeClassHash(t *testing.T) {
	rootSize40 := int32(40)
	rootSize80 := int32(80)
	volumeTypeSSD := "SSD"
	volumeTypeGPSSD := "GPSSD"

	base := &ECSNodeClass{
		Spec: ECSNodeClassSpec{
			SubnetSelectorTerms: []SubnetSelectorTerm{{ID: "123e4567-e89b-12d3-a456-426614174000"}},
			HMISelectorTerms:    []HMISelectorTerm{{Alias: "Huawei Cloud EulerOS 2.0"}},
			RootVolume: RootVolume{
				Size:       &rootSize40,
				VolumeType: &volumeTypeSSD,
			},
		},
	}

	t.Run("root volume size changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.RootVolume.Size = &rootSize80

		if base.Hash() == other.Hash() {
			t.Fatalf("expected root volume size change to alter hash")
		}
	})

	t.Run("root volume type changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.RootVolume.VolumeType = &volumeTypeGPSSD

		if base.Hash() == other.Hash() {
			t.Fatalf("expected root volume type change to alter hash")
		}
	})

	t.Run("omitted root volume defaults hash same as explicit defaults", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.RootVolume = RootVolume{}

		if base.Hash() != other.Hash() {
			t.Fatalf("expected omitted root volume to hash same as explicit defaults")
		}
	})

	t.Run("first hmi selector changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.HMISelectorTerms[0].Alias = "Huawei Cloud EulerOS 3.0"

		if base.Hash() == other.Hash() {
			t.Fatalf("expected first hmi selector change to alter hash")
		}
	})

	t.Run("swapping first selected hmi changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.HMISelectorTerms = []HMISelectorTerm{
			{ID: "123e4567-e89b-12d3-a456-426614174999"},
			{Alias: "Huawei Cloud EulerOS 2.0"},
		}
		baseWithTwo := base.DeepCopy()
		baseWithTwo.Spec.HMISelectorTerms = []HMISelectorTerm{
			{Alias: "Huawei Cloud EulerOS 2.0"},
			{ID: "123e4567-e89b-12d3-a456-426614174999"},
		}

		if baseWithTwo.Hash() == other.Hash() {
			t.Fatalf("expected swapping selected hmi term to alter hash")
		}
	})

	t.Run("changing non selected hmi does not change hash", func(t *testing.T) {
		withTwo := base.DeepCopy()
		withTwo.Spec.HMISelectorTerms = []HMISelectorTerm{
			{Alias: "Huawei Cloud EulerOS 2.0"},
			{ID: "123e4567-e89b-12d3-a456-426614174999"},
		}
		other := withTwo.DeepCopy()
		other.Spec.HMISelectorTerms[1].ID = "123e4567-e89b-12d3-a456-426614174998"

		if withTwo.Hash() != other.Hash() {
			t.Fatalf("expected non-selected hmi selector change not to alter hash")
		}
	})

	t.Run("subnet selector changes do not change hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.SubnetSelectorTerms = []SubnetSelectorTerm{{ID: "123e4567-e89b-12d3-a456-426614174111"}}

		if base.Hash() != other.Hash() {
			t.Fatalf("expected subnet selector changes not to alter hash")
		}
	})
}

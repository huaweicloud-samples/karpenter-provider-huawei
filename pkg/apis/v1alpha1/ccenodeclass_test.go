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

func TestCCENodeClassHash(t *testing.T) {
	rootIOPS := int32(3000)
	rootThroughput := int32(125)
	userVolumeSize := int32(100)
	ecsGroupID := "46ebaf04-ca42-48ca-8057-0b96e6126e1b"

	base := &CCENodeClass{
		Spec: CCENodeClassSpec{
			ECSGroupID:          &ecsGroupID,
			SubnetSelectorTerms: []SubnetSelectorTerm{{ID: "123e4567-e89b-12d3-a456-426614174000"}},
			IMSSelector:         IMSSelector{IMSFamily: "HCE OS 2.0"},
			BlockDeviceMappings: BlockDeviceMappings{
				K8S: &BlockDevice{
					VolumeSize: 120,
					VolumeType: "SAS",
				},
				Root: BlockDevice{
					VolumeSize: 120,
					VolumeType: "GPSSD2",
					IOPS:       &rootIOPS,
					Throughput: &rootThroughput,
				},
				Users: []BlockDevice{{
					VolumeSize: userVolumeSize,
					VolumeType: "SATA",
				}},
			},
			RuntimeConfiguration: &RuntimeConfiguration{Type: "docker"},
			Login: Login{
				UserPassword: UserPassword{
					Password: "ciphertext",
				},
			},
		},
	}

	t.Run("root block device changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.BlockDeviceMappings.Root.VolumeSize = 160

		if base.Hash() == other.Hash() {
			t.Fatalf("expected root block device change to alter hash")
		}
	})

	t.Run("runtime type changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.RuntimeConfiguration.Type = "containerd"

		if base.Hash() == other.Hash() {
			t.Fatalf("expected runtime type change to alter hash")
		}
	})

	t.Run("login username default hashes same as explicit root", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.Login.UserPassword.Username = "root"

		if base.Hash() != other.Hash() {
			t.Fatalf("expected default login username to hash same as explicit root")
		}
	})

	t.Run("ims family changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.IMSSelector.IMSFamily = "Ubuntu 22.04"

		if base.Hash() == other.Hash() {
			t.Fatalf("expected ims family change to alter hash")
		}
	})

	t.Run("user data volume changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.BlockDeviceMappings.Users[0].VolumeType = "SAS"

		if base.Hash() == other.Hash() {
			t.Fatalf("expected user data volume change to alter hash")
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

func TestCCENodeClassSpecValidateForCreateNode(t *testing.T) {
	valid := CCENodeClassSpec{
		IMSSelector: IMSSelector{IMSFamily: "Huawei Cloud EulerOS 2.0"},
		BlockDeviceMappings: BlockDeviceMappings{
			Root: BlockDevice{
				VolumeSize: 40,
				VolumeType: "SSD",
			},
			K8S: &BlockDevice{
				VolumeSize: 100,
				VolumeType: "SAS",
			},
			Users: []BlockDevice{{
				VolumeSize: 100,
				VolumeType: "SATA",
			}},
		},
		Login: Login{
			UserPassword: UserPassword{Password: "ciphertext"},
		},
	}

	t.Run("accepts valid data volumes", func(t *testing.T) {
		if err := valid.ValidateForCreateNode(); err != nil {
			t.Fatalf("expected validation to succeed, got %v", err)
		}
	})

	t.Run("rejects root volume smaller than 40Gi", func(t *testing.T) {
		spec := valid
		spec.BlockDeviceMappings.Root.VolumeSize = 39

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized root volume")
		}
	})

	t.Run("rejects k8s volume smaller than 100Gi", func(t *testing.T) {
		spec := valid
		spec.BlockDeviceMappings.K8S.VolumeSize = 99

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized k8s volume")
		}
	})

	t.Run("rejects user volume smaller than 100Gi", func(t *testing.T) {
		spec := valid
		spec.BlockDeviceMappings.Users[0].VolumeSize = 99

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized user volume")
		}
	})
}

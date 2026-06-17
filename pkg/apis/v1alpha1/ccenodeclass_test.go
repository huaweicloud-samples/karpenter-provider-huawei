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
				UserPassword: &UserPassword{
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

	t.Run("switching from password login to ssh key changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.Login.UserPassword = nil
		other.Spec.Login.SSHKey = "cluster-keypair"

		if base.Hash() == other.Hash() {
			t.Fatalf("expected login mode change to alter hash")
		}
	})

	t.Run("ssh key changes hash", func(t *testing.T) {
		other := base.DeepCopy()
		other.Spec.Login.UserPassword = nil
		other.Spec.Login.SSHKey = "cluster-keypair-a"
		baseWithSSH := other.DeepCopy()
		other.Spec.Login.SSHKey = "cluster-keypair-b"

		if baseWithSSH.Hash() == other.Hash() {
			t.Fatalf("expected ssh key change to alter hash")
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
	validSpec := func() CCENodeClassSpec {
		return CCENodeClassSpec{
			IMSSelector: IMSSelector{IMSFamily: "Huawei Cloud EulerOS 2.0"},
			BlockDeviceMappings: BlockDeviceMappings{
				Root: BlockDevice{
					VolumeSize: MinRootVolumeSizeGiB,
					VolumeType: "SSD",
				},
				K8S: &BlockDevice{
					VolumeSize: MinKubernetesDataVolumeSizeGiB,
					VolumeType: "SAS",
				},
				Users: []BlockDevice{{
					VolumeSize: MinUserDataVolumeSizeGiB,
					VolumeType: "SATA",
				}},
			},
			Login: Login{
				UserPassword: &UserPassword{Password: "ciphertext"},
			},
		}
	}

	t.Run("accepts valid data volumes", func(t *testing.T) {
		spec := validSpec()
		if err := spec.ValidateForCreateNode(); err != nil {
			t.Fatalf("expected validation to succeed, got %v", err)
		}
	})

	t.Run("accepts ssh key login", func(t *testing.T) {
		spec := validSpec()
		spec.Login.UserPassword = nil
		spec.Login.SSHKey = "cluster-keypair"

		if err := spec.ValidateForCreateNode(); err != nil {
			t.Fatalf("expected ssh key validation to succeed, got %v", err)
		}
	})

	t.Run("rejects missing login method", func(t *testing.T) {
		spec := validSpec()
		spec.Login.UserPassword = nil

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail when no login method is set")
		}
	})

	t.Run("rejects both login methods", func(t *testing.T) {
		spec := validSpec()
		spec.Login.SSHKey = "cluster-keypair"

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail when both login methods are set")
		}
	})

	t.Run("rejects root volume smaller than 40Gi", func(t *testing.T) {
		spec := validSpec()
		spec.BlockDeviceMappings.Root.VolumeSize = MinRootVolumeSizeGiB - 1

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized root volume")
		}
	})

	t.Run("rejects k8s volume smaller than 20Gi", func(t *testing.T) {
		spec := validSpec()
		spec.BlockDeviceMappings.K8S.VolumeSize = MinKubernetesDataVolumeSizeGiB - 1

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized k8s volume")
		}
	})

	t.Run("rejects user volume smaller than 10Gi", func(t *testing.T) {
		spec := validSpec()
		spec.BlockDeviceMappings.Users[0].VolumeSize = MinUserDataVolumeSizeGiB - 1

		if err := spec.ValidateForCreateNode(); err == nil {
			t.Fatalf("expected validation to fail for undersized user volume")
		}
	})
}

func TestCCENodeClassBlockDeviceMappings(t *testing.T) {
	t.Run("returns zero mappings for nil nodeclass", func(t *testing.T) {
		var nodeClass *CCENodeClass
		if got := nodeClass.BlockDeviceMappings(); got.K8S != nil || got.Root != (BlockDevice{}) || len(got.Users) != 0 {
			t.Fatalf("expected zero block device mappings, got %+v", got)
		}
	})

	t.Run("returns spec block device mappings", func(t *testing.T) {
		nodeClass := &CCENodeClass{
			Spec: CCENodeClassSpec{
				BlockDeviceMappings: BlockDeviceMappings{
					K8S:  &BlockDevice{VolumeSize: 120, VolumeType: "SAS"},
					Root: BlockDevice{VolumeSize: 40, VolumeType: "SSD"},
				},
			},
		}
		got := nodeClass.BlockDeviceMappings()
		if got.K8S == nil || got.K8S.VolumeSize != 120 {
			t.Fatalf("expected explicit k8s block device mappings, got %+v", got)
		}
		if got.Root.VolumeSize != 40 {
			t.Fatalf("expected root block device mappings to be preserved, got %+v", got)
		}
	})
}

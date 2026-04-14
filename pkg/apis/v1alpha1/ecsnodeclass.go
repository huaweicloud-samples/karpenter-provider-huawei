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

import (
	"fmt"
	"strings"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ECSNodeClassSpec defines the desired state of ECSNodeClass
type ECSNodeClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// SubnetSelectorTerms is a list of subnet selector terms. The terms are ORed.
	// +kubebuilder:validation:XValidation:message="subnetSelectorTerms cannot be empty",rule="self.size() != 0"
	// +kubebuilder:validation:XValidation:message="expected at least one, got none, ['name', 'id']",rule="self.all(x, has(x.name) || has(x.id))"
	// +kubebuilder:validation:MaxItems:=30
	// +required
	SubnetSelectorTerms []SubnetSelectorTerm `json:"subnetSelectorTerms" hash:"ignore"`

	// Kubelet defines args to be used when configuring kubelet on provisioned nodes.
	// They are a subset of the upstream types, recognizing not all options may be supported.
	// Wherever possible, the types and names should reflect the upstream kubelet types.
	// +kubebuilder:validation:XValidation:message="imageGCHighThresholdPercent must be greater than imageGCLowThresholdPercent",rule="has(self.imageGCHighThresholdPercent) && has(self.imageGCLowThresholdPercent) ?  self.imageGCHighThresholdPercent > self.imageGCLowThresholdPercent  : true"
	// +kubebuilder:validation:XValidation:message="evictionSoft OwnerKey does not have a matching evictionSoftGracePeriod",rule="has(self.evictionSoft) ? self.evictionSoft.all(e, (e in self.evictionSoftGracePeriod)):true"
	// +kubebuilder:validation:XValidation:message="evictionSoftGracePeriod OwnerKey does not have a matching evictionSoft",rule="has(self.evictionSoftGracePeriod) ? self.evictionSoftGracePeriod.all(e, (e in self.evictionSoft)):true"
	// +optional
	Kubelet *KubeletConfiguration `json:"kubelet,omitempty"`

	// ECSGroupID is the ECS server group ID used when creating CCE nodes.
	// +optional
	ECSGroupID *string `json:"ecsGroupId,omitempty"`

	// IMSSelector selects the node operating system family.
	// +required
	IMSSelector IMSSelector `json:"imsSelector" hash:"ignore"`

	// BlockDeviceMappings defines the root, k8s, and user data volumes used for CCE CreateNode.
	// +required
	BlockDeviceMappings BlockDeviceMappings `json:"blockDeviceMappings" hash:"ignore"`

	// RuntimeConfiguration configures the container runtime used on the node.
	// +optional
	RuntimeConfiguration *RuntimeConfiguration `json:"runtimeConfiguration,omitempty" hash:"ignore"`

	// Login defines the node login configuration.
	// +required
	Login Login `json:"login" hash:"ignore"`
}

type normalizedIMSSelection struct {
	Family string
}

type normalizedVolume struct {
	Size       int32
	VolumeType string
	Iops       int32
	Throughput int32
}

type normalizedRuntimeConfiguration struct {
	Type string
}

type normalizedUserPassword struct {
	Username string
	Password string
}

type normalizedLogin struct {
	UserPassword *normalizedUserPassword
}

type normalizedBlockDeviceMappings struct {
	Root  normalizedVolume
	K8S   *normalizedVolume
	Users []normalizedVolume
}

// IMSSelector defines the node operating system family used by CCE CreateNode.
type IMSSelector struct {
	// IMSFamily is the node operating system family.
	// +kubebuilder:validation:MinLength=1
	IMSFamily string `json:"imsFamily"`
}

// BlockDeviceMappings defines disk configuration for root, k8s, and user data volumes.
type BlockDeviceMappings struct {
	// K8S is the data volume used by the container runtime and kubelet.
	// +optional
	K8S *BlockDevice `json:"k8s,omitempty"`

	// Root is the system disk.
	// +required
	Root BlockDevice `json:"root"`

	// Users are additional user data disks.
	// +optional
	Users []BlockDevice `json:"users,omitempty"`
}

// BlockDevice defines a CCE volume.
type BlockDevice struct {
	// VolumeSize is the disk size in GiB.
	// +kubebuilder:validation:Minimum:=10
	VolumeSize int32 `json:"volumeSize"`

	// VolumeType is the disk type.
	// +kubebuilder:validation:MinLength=1
	VolumeType string `json:"volumeType"`

	// IOPS is required for GPSSD2 and ESSD2.
	// +optional
	IOPS *int32 `json:"iops,omitempty"`

	// Throughput is required for GPSSD2.
	// +optional
	Throughput *int32 `json:"throughput,omitempty"`
}

// RuntimeConfiguration defines the node container runtime.
type RuntimeConfiguration struct {
	// Type is the container runtime type.
	// +kubebuilder:validation:Enum=docker;containerd
	// +optional
	Type string `json:"type,omitempty"`
}

// Login defines the node login configuration.
type Login struct {
	// UserPassword defines username/password login.
	// +required
	UserPassword UserPassword `json:"userPassword"`
}

// UserPassword defines the node login credentials.
type UserPassword struct {
	// Username defaults to root.
	// +kubebuilder:validation:Enum=root
	// +optional
	Username string `json:"username,omitempty"`

	// Password is the salted and encrypted node password.
	// +kubebuilder:validation:MinLength=1
	Password string `json:"password"`
}

// ResolveIMSForCreateNode resolves the node OS selection for CCE CreateNode.
func (s *ECSNodeClassSpec) ResolveIMSForCreateNode() (osAlias string, err error) {
	if s == nil {
		return "", fmt.Errorf("nodeClass.spec is nil")
	}
	osAlias = strings.TrimSpace(s.IMSSelector.IMSFamily)
	if osAlias == "" {
		return "", fmt.Errorf("nodeClass.spec.imsSelector.imsFamily is required")
	}
	return osAlias, nil
}

func (s *ECSNodeClassSpec) normalizedIMSSelection() normalizedIMSSelection {
	if s == nil {
		return normalizedIMSSelection{}
	}
	return normalizedIMSSelection{
		Family: strings.TrimSpace(s.IMSSelector.IMSFamily),
	}
}

func normalizeBlockDevice(device *BlockDevice) normalizedVolume {
	if device == nil {
		return normalizedVolume{}
	}
	size := device.VolumeSize
	if size <= 0 {
		size = 40
	}
	volumeType := strings.TrimSpace(device.VolumeType)
	if volumeType == "" {
		volumeType = "SSD"
	}
	return normalizedVolume{
		Size:       size,
		VolumeType: volumeType,
		Iops:       lo.FromPtrOr(device.IOPS, 0),
		Throughput: lo.FromPtrOr(device.Throughput, 0),
	}
}

func (s *ECSNodeClassSpec) normalizedBlockDeviceMappings() normalizedBlockDeviceMappings {
	if s == nil {
		return normalizedBlockDeviceMappings{}
	}
	users := make([]normalizedVolume, 0, len(s.BlockDeviceMappings.Users))
	for _, user := range s.BlockDeviceMappings.Users {
		users = append(users, normalizeBlockDevice(&user))
	}
	var k8s *normalizedVolume
	if s.BlockDeviceMappings.K8S != nil {
		n := normalizeBlockDevice(s.BlockDeviceMappings.K8S)
		k8s = &n
	}
	return normalizedBlockDeviceMappings{
		Root:  normalizeBlockDevice(&s.BlockDeviceMappings.Root),
		K8S:   k8s,
		Users: users,
	}
}

func (s *ECSNodeClassSpec) normalizedRuntimeConfiguration() normalizedRuntimeConfiguration {
	runtimeType := "containerd"
	if s != nil && s.RuntimeConfiguration != nil && strings.TrimSpace(s.RuntimeConfiguration.Type) != "" {
		runtimeType = strings.TrimSpace(s.RuntimeConfiguration.Type)
	}
	return normalizedRuntimeConfiguration{Type: runtimeType}
}

func (s *ECSNodeClassSpec) normalizedLogin() normalizedLogin {
	if s == nil {
		return normalizedLogin{}
	}
	username := strings.TrimSpace(s.Login.UserPassword.Username)
	if username == "" {
		username = "root"
	}
	return normalizedLogin{
		UserPassword: &normalizedUserPassword{
			Username: username,
			Password: s.Login.UserPassword.Password,
		},
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// ECSNodeClass is the Schema for the ecsnodeclasses API
type ECSNodeClass struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ECSNodeClass
	// +required
	Spec ECSNodeClassSpec `json:"spec"`

	// status defines the observed state of ECSNodeClass
	// +optional
	Status ECSNodeClassStatus `json:"status,omitempty"`
}

// We need to bump the ECSNodeClassHashVersion when we make an update to the ECSNodeClass CRD under these conditions:
// 1. A field changes its default value for an existing field that is already hashed
// 2. A field is added to the hash calculation with an already-set value
// 3. A field is removed from the hash calculations
const ECSNodeClassHashVersion = "v2"

func (in *ECSNodeClass) Hash() string {
	return fmt.Sprint(lo.Must(hashstructure.Hash(struct {
		Spec                         ECSNodeClassSpec
		EffectiveIMS                 normalizedIMSSelection
		EffectiveBlockDeviceMappings normalizedBlockDeviceMappings
		EffectiveRuntime             normalizedRuntimeConfiguration
		EffectiveLogin               normalizedLogin
	}{
		Spec:                         in.Spec,
		EffectiveIMS:                 in.Spec.normalizedIMSSelection(),
		EffectiveBlockDeviceMappings: in.Spec.normalizedBlockDeviceMappings(),
		EffectiveRuntime:             in.Spec.normalizedRuntimeConfiguration(),
		EffectiveLogin:               in.Spec.normalizedLogin(),
	}, hashstructure.FormatV2, &hashstructure.HashOptions{
		SlicesAsSets:    true,
		IgnoreZeroValue: true,
		ZeroNil:         true,
	})))
}

// +kubebuilder:object:root=true

// ECSNodeClassList contains a list of ECSNodeClass
type ECSNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ECSNodeClass `json:"items"`
}

// SubnetSelectorTerm defines selection logic for a subnet used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SubnetSelectorTerm struct {
	// ID is the subnet id in ECS
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"
	// +optional
	ID string `json:"id,omitempty"`
	// Name is the subnet id in ECS
	// +kubebuilder:validation:MinLength=1
	// +optional
	Name string `json:"name,omitempty"`
}

// SecurityGroupSelectorTerm defines selection logic for a security group used by Karpenter to launch nodes.
// If multiple fields are used for selection, the requirements are ANDed.
type SecurityGroupSelectorTerm struct {
	// Tags is a map of key/value tags used to select security groups.
	// Specifying '*' for a value selects all values for a given tag key.
	// +kubebuilder:validation:XValidation:message="empty tag keys or values aren't supported",rule="self.all(k, k != '' && self[k] != '')"
	// +kubebuilder:validation:MaxProperties:=20
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
	// ID is the security group id in ECS
	// +kubebuilder:validation:Pattern:="sg-[0-9a-z]+"
	// +optional
	ID string `json:"id,omitempty"`
	// Name is the security group name in ECS.
	// This value is the name field, which is different from the name tag.
	Name string `json:"name,omitempty"`
}

// KubeletConfiguration defines args to be used when configuring kubelet on provisioned nodes.
// They are a subset of the upstream types, recognizing not all options may be supported.
// Wherever possible, the types and names should reflect the upstream kubelet types.
// https://pkg.go.dev/k8s.io/kubelet/config/v1beta1#KubeletConfiguration
// https://github.com/kubernetes/kubernetes/blob/9f82d81e55cafdedab619ea25cabf5d42736dacf/cmd/kubelet/app/options/options.go#L53
type KubeletConfiguration struct {
	// clusterDNS is a list of IP addresses for the cluster DNS server.
	// Note that not all providers may use all addresses.
	//+optional
	ClusterDNS []string `json:"clusterDNS,omitempty"`
	// MaxPods is an override for the maximum number of pods that can run on
	// a worker node instance.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MaxPods *int32 `json:"maxPods,omitempty"`
	// PodsPerCore is an override for the number of pods that can run on a worker node
	// instance based on the number of cpu cores. This value cannot exceed MaxPods, so, if
	// MaxPods is a lower value, that value will be used.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	PodsPerCore *int32 `json:"podsPerCore,omitempty"`
	// SystemReserved contains resources reserved for OS system daemons and kernel memory.
	// +kubebuilder:validation:XValidation:message="valid keys for systemReserved are ['cpu','memory','ephemeral-storage','pid']",rule="self.all(x, x=='cpu' || x=='memory' || x=='ephemeral-storage' || x=='pid')"
	// +kubebuilder:validation:XValidation:message="systemReserved value cannot be a negative resource quantity",rule="self.all(x, !self[x].startsWith('-'))"
	// +optional
	SystemReserved map[string]string `json:"systemReserved,omitempty"`
	// KubeReserved contains resources reserved for Kubernetes system components.
	// +kubebuilder:validation:XValidation:message="valid keys for kubeReserved are ['cpu','memory','ephemeral-storage','pid']",rule="self.all(x, x=='cpu' || x=='memory' || x=='ephemeral-storage' || x=='pid')"
	// +kubebuilder:validation:XValidation:message="kubeReserved value cannot be a negative resource quantity",rule="self.all(x, !self[x].startsWith('-'))"
	// +optional
	KubeReserved map[string]string `json:"kubeReserved,omitempty"`
	// EvictionHard is the map of signal names to quantities that define hard eviction thresholds
	// +kubebuilder:validation:XValidation:message="valid keys for evictionHard are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionHard map[string]string `json:"evictionHard,omitempty"`
	// EvictionSoft is the map of signal names to quantities that define soft eviction thresholds
	// +kubebuilder:validation:XValidation:message="valid keys for evictionSoft are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionSoft map[string]string `json:"evictionSoft,omitempty"`
	// EvictionSoftGracePeriod is the map of signal names to quantities that define grace periods for each eviction signal
	// +kubebuilder:validation:XValidation:message="valid keys for evictionSoftGracePeriod are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionSoftGracePeriod map[string]metav1.Duration `json:"evictionSoftGracePeriod,omitempty"`
	// EvictionMaxPodGracePeriod is the maximum allowed grace period (in seconds) to use when terminating pods in
	// response to soft eviction thresholds being met.
	// +optional
	EvictionMaxPodGracePeriod *int32 `json:"evictionMaxPodGracePeriod,omitempty"`
	// ImageGCHighThresholdPercent is the percent of disk usage after which image
	// garbage collection is always run. The percent is calculated by dividing this
	// field value by 100, so this field must be between 0 and 100, inclusive.
	// When specified, the value must be greater than ImageGCLowThresholdPercent.
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=100
	// +optional
	ImageGCHighThresholdPercent *int32 `json:"imageGCHighThresholdPercent,omitempty"`
	// ImageGCLowThresholdPercent is the percent of disk usage before which image
	// garbage collection is never run. Lowest disk usage to garbage collect to.
	// The percent is calculated by dividing this field value by 100,
	// so the field value must be between 0 and 100, inclusive.
	// When specified, the value must be less than imageGCHighThresholdPercent
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=100
	// +optional
	ImageGCLowThresholdPercent *int32 `json:"imageGCLowThresholdPercent,omitempty"`
	// CPUCFSQuota enables CPU CFS quota enforcement for containers that specify CPU limits.
	// +optional
	CPUCFSQuota *bool `json:"cpuCFSQuota,omitempty"`
}

func (in *ECSNodeClass) KubeletConfiguration() *KubeletConfiguration {
	return in.Spec.Kubelet
}

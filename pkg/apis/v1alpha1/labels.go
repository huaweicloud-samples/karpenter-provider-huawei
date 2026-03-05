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
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

var (
	RestrictedLabelDomains = []string{
		apis.Group,
	}

	LabelInstanceSize             = apis.Group + "/instance-size"
	LabelInstanceCPU              = apis.Group + "/instance-cpu"
	LabelInstanceMemory           = apis.Group + "/instance-memory"
	LabelInstanceNetworkBandwidth = apis.Group + "/instance-network-bandwidth"
	LabelInstanceGPUName          = apis.Group + "/instance-gpu-name"
	LabelInstanceGPUManufacturer  = apis.Group + "/instance-gpu-manufacturer"
	LabelInstanceGPUCount         = apis.Group + "/instance-gpu-count"
	LabelInstanceGPUMemory        = apis.Group + "/instance-gpu-memory"
)

func init() {
	karpv1.RestrictedLabelDomains = karpv1.RestrictedLabelDomains.Insert(RestrictedLabelDomains...)
	karpv1.WellKnownLabels = karpv1.WellKnownLabels.Insert(
		LabelInstanceSize,
		LabelInstanceCPU,
		LabelInstanceMemory,
		LabelInstanceNetworkBandwidth,
		LabelInstanceGPUName,
		LabelInstanceGPUManufacturer,
		LabelInstanceGPUCount,
		LabelInstanceGPUMemory,
		corev1.LabelWindowsBuild,
	)
}

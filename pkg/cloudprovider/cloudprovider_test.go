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

package cloudprovider

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpcloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	karpscheduling "sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	instanceprovider "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instance"
)

func TestResolvedNodeClaimLabels_IncludesRestrictedWellKnownLabels(t *testing.T) {
	instanceType := &karpcloudprovider.InstanceType{
		Name: "c9.large.2",
		Requirements: karpscheduling.NewRequirements(
			karpscheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
			karpscheduling.NewRequirement(corev1.LabelOSStable, corev1.NodeSelectorOpIn, string(corev1.Linux)),
			karpscheduling.NewRequirement(corev1.LabelTopologyRegion, corev1.NodeSelectorOpIn, "ap-southeast-3"),
			karpscheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "ap-southeast-3a", "ap-southeast-3b"),
		),
	}
	createdInstance := &instanceprovider.Instance{
		Flavor: "c9.large.2",
		Zone:   "ap-southeast-3a",
	}

	labels := resolvedNodeClaimLabels(instanceType, createdInstance)

	if got := labels[corev1.LabelInstanceTypeStable]; got != "c9.large.2" {
		t.Fatalf("expected instance-type label %q, got %q", "c9.large.2", got)
	}
	if got := labels[corev1.LabelTopologyZone]; got != "ap-southeast-3a" {
		t.Fatalf("expected zone label %q, got %q", "ap-southeast-3a", got)
	}
	if got := labels[karpv1.CapacityTypeLabelKey]; got != karpv1.CapacityTypeOnDemand {
		t.Fatalf("expected capacity-type label %q, got %q", karpv1.CapacityTypeOnDemand, got)
	}
	if got := labels[corev1.LabelArchStable]; got != "amd64" {
		t.Fatalf("expected arch label %q, got %q", "amd64", got)
	}
	if got := labels[corev1.LabelOSStable]; got != string(corev1.Linux) {
		t.Fatalf("expected os label %q, got %q", corev1.Linux, got)
	}
	if got := labels[corev1.LabelTopologyRegion]; got != "ap-southeast-3" {
		t.Fatalf("expected region label %q, got %q", "ap-southeast-3", got)
	}
}

func TestAreStaticFieldsDrifted_ReturnsNodeClassDriftWhenHashesDiffer(t *testing.T) {
	provider := &CloudProvider{}
	nodeClaim := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1alpha1.AnnotationECSNodeClassHash:        "hash-a",
				v1alpha1.AnnotationECSNodeClassHashVersion: "v1",
			},
		},
	}
	nodeClass := &v1alpha1.ECSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1alpha1.AnnotationECSNodeClassHash:        "hash-b",
				v1alpha1.AnnotationECSNodeClassHashVersion: "v1",
			},
		},
	}

	if got := provider.areStaticFieldsDrifted(nodeClaim, nodeClass); got != NodeClassDrift {
		t.Fatalf("expected drift reason %q, got %q", NodeClassDrift, got)
	}
}

func TestIsSubnetDrifted_ReturnsExpectedDriftReason(t *testing.T) {
	provider := &CloudProvider{}
	nodeClass := &v1alpha1.ECSNodeClass{
		Status: v1alpha1.ECSNodeClassStatus{
			Subnets: []v1alpha1.Subnet{{ID: "subnet-123"}},
		},
	}

	testCases := []struct {
		name     string
		subnetID string
		want     karpcloudprovider.DriftReason
	}{
		{
			name:     "matching subnet",
			subnetID: "subnet-123",
			want:     "",
		},
		{
			name:     "missing subnet",
			subnetID: "subnet-456",
			want:     SubnetDrift,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := provider.isSubnetDrifted(&instanceprovider.Instance{SubnetID: tc.subnetID}, nodeClass)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected drift reason %q, got %q", tc.want, got)
			}
		})
	}
}

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
	"context"
	"net/http"
	"testing"

	cceMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	cms "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cms/v1/model"
	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
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

func TestNodeSpecForCandidate_AddsDefaultDataVolume(t *testing.T) {
	rootSize := int32(50)
	rootType := "GPSSD"
	provider := &DefaultProvider{}
	nodeClass := &v1alpha1.ECSNodeClass{
		Spec: v1alpha1.ECSNodeClassSpec{
			RootVolume: v1alpha1.RootVolume{
				Size:       &rootSize,
				VolumeType: &rootType,
			},
		},
	}
	nodeClaim := &karpv1.NodeClaim{}
	spec := provider.nodeSpecForCandidate(
		nodeClass,
		nodeClaim,
		nil,
		createCandidate{
			instanceType: &cloudprovider.InstanceType{Name: "c9.large.2"},
			zone:         "ap-southeast-3a",
			subnetID:     "subnet-123",
		},
		"Huawei Cloud EulerOS 2.0",
		"",
	)
	if spec.RootVolume == nil {
		t.Fatalf("expected root volume to be set")
	}
	if spec.RootVolume.Size != 50 || spec.RootVolume.Volumetype != "GPSSD" {
		t.Fatalf("expected root volume 50/GPSSD, got %#v", spec.RootVolume)
	}
	if spec.DataVolumes == nil || len(*spec.DataVolumes) != 1 {
		t.Fatalf("expected one default data volume, got %#v", spec.DataVolumes)
	}
	if got := (*spec.DataVolumes)[0]; got.Size != 100 || got.Volumetype != "GPSSD" {
		t.Fatalf("expected default data volume 100/GPSSD, got %#v", got)
	}
}

func TestToCCETaints_ExcludesKarpenterUnregisteredTaint(t *testing.T) {
	nodeClaim := &karpv1.NodeClaim{
		Spec: karpv1.NodeClaimSpec{
			StartupTaints: []corev1.Taint{
				{
					Key:    "example.com/startup",
					Effect: corev1.TaintEffectNoSchedule,
				},
				karpv1.UnregisteredNoExecuteTaint,
			},
			Taints: []corev1.Taint{
				{
					Key:    "example.com/dedicated",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
	}

	got := toCCETaints(nodeClaim)
	if got == nil {
		t.Fatalf("expected taints to be returned")
	}
	if len(*got) != 2 {
		t.Fatalf("expected 2 taints after filtering, got %#v", *got)
	}

	keys := map[string]struct{}{}
	for _, taint := range *got {
		keys[taint.Key+"/"+taint.Effect.Value()] = struct{}{}
	}
	if _, ok := keys["example.com/startup/NoSchedule"]; !ok {
		t.Fatalf("expected startup taint to be preserved, got %#v", *got)
	}
	if _, ok := keys["example.com/dedicated/NoSchedule"]; !ok {
		t.Fatalf("expected regular taint to be preserved, got %#v", *got)
	}
	if _, ok := keys[karpv1.UnregisteredTaintKey+"/NoExecute"]; ok {
		t.Fatalf("expected karpenter unregistered taint to be filtered, got %#v", *got)
	}
}

type stubCCEAPI struct {
	createNodeResp *cceMdl.CreateNodeResponse
	createNodeErr  error
}

func (s *stubCCEAPI) ShowCluster(*cceMdl.ShowClusterRequest) (*cceMdl.ShowClusterResponse, error) {
	return nil, nil
}

func (s *stubCCEAPI) CreateNode(*cceMdl.CreateNodeRequest) (*cceMdl.CreateNodeResponse, error) {
	return s.createNodeResp, s.createNodeErr
}

func (s *stubCCEAPI) DeleteNode(*cceMdl.DeleteNodeRequest) (*cceMdl.DeleteNodeResponse, error) {
	return nil, nil
}

func (s *stubCCEAPI) ListNodes(*cceMdl.ListNodesRequest) (*cceMdl.ListNodesResponse, error) {
	return nil, nil
}

func (s *stubCCEAPI) ShowNode(*cceMdl.ShowNodeRequest) (*cceMdl.ShowNodeResponse, error) {
	return nil, nil
}

func (s *stubCCEAPI) ShowJob(*cceMdl.ShowJobRequest) (*cceMdl.ShowJobResponse, error) {
	return nil, nil
}

type stubSubnetProvider struct {
	zonalSubnets map[string]*subnet.Subnet
}

func (s *stubSubnetProvider) LivenessProbe(*http.Request) error {
	return nil
}

func (s *stubSubnetProvider) List(context.Context, *v1alpha1.ECSNodeClass) ([]vpcMdl.Subnet, error) {
	return nil, nil
}

func (s *stubSubnetProvider) ZonalSubnetsForLaunch(context.Context, *v1alpha1.ECSNodeClass, []*cloudprovider.InstanceType, string) (map[string]*subnet.Subnet, error) {
	return s.zonalSubnets, nil
}

func (s *stubSubnetProvider) UpdateInflightIPs(*cms.CreateAutoLaunchGroupRequest, *cms.CreateAutoLaunchGroupResponse, []*cloudprovider.InstanceType, []*subnet.Subnet, string) {
}

var (
	_ sdk.CCEAPI      = (*stubCCEAPI)(nil)
	_ subnet.Provider = (*stubSubnetProvider)(nil)
)

func TestCreate_AllowsEmptyServerIDInCreateNodeResponse(t *testing.T) {
	provider := &DefaultProvider{
		clusterID: "cluster-id",
		cceapi: &stubCCEAPI{
			createNodeResp: &cceMdl.CreateNodeResponse{
				Metadata: &cceMdl.NodeMetadata{Uid: lo.ToPtr("node-123")},
				Status:   &cceMdl.NodeStatus{},
			},
		},
		subnetProvider: &stubSubnetProvider{
			zonalSubnets: map[string]*subnet.Subnet{
				"zone-a": {ID: "subnet-a", Zone: "zone-a", AvailableIPAddressCount: 100},
			},
		},
	}
	nodeClass := &v1alpha1.ECSNodeClass{
		Spec: v1alpha1.ECSNodeClassSpec{
			HMISelectorTerms: []v1alpha1.HMISelectorTerm{{Alias: "Huawei Cloud EulerOS 2.0"}},
		},
		Status: v1alpha1.ECSNodeClassStatus{
			Subnets: []v1alpha1.Subnet{{ID: "subnet-a", Zone: "zone-a"}},
		},
	}
	nodeClaim := &karpv1.NodeClaim{}
	instanceTypes := []*cloudprovider.InstanceType{{
		Name: "c9.large.2",
		Offerings: cloudprovider.Offerings{
			{
				Available: true,
				Requirements: scheduling.NewRequirements(
					scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "zone-a"),
				),
			},
		},
	}}

	instance, err := provider.Create(context.Background(), nodeClass, nodeClaim, nil, instanceTypes)
	if err != nil {
		t.Fatalf("expected create to succeed without server id, got %v", err)
	}
	if instance.NodeID != "node-123" {
		t.Fatalf("expected node id %q, got %q", "node-123", instance.NodeID)
	}
	if instance.ServerID != "" {
		t.Fatalf("expected empty server id, got %q", instance.ServerID)
	}
}

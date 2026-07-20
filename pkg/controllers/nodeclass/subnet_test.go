package nodeclass

import (
	"context"
	"net/http"
	"testing"

	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	providersubnet "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
)

type stubSubnetProvider struct {
	subnets []vpcMdl.Subnet
	err     error
}

func (s *stubSubnetProvider) LivenessProbe(_ *http.Request) error {
	return nil
}

func (s *stubSubnetProvider) List(context.Context, *v1alpha1.CCENodeClass) ([]vpcMdl.Subnet, error) {
	return s.subnets, s.err
}

func (s *stubSubnetProvider) SelectForLaunch(context.Context, *v1alpha1.CCENodeClass, []*cloudprovider.InstanceType, string) (*providersubnet.Subnet, error) {
	return nil, nil
}

func (s *stubSubnetProvider) ReleaseInflightIPs(*providersubnet.Subnet) {
}

func TestSubnetReconcilerMarksSubnetsReadyWithoutAvailabilityZone(t *testing.T) {
	reconciler := NewSubnetReconciler(&stubSubnetProvider{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", AvailabilityZone: "", AvailableIpAddressCount: 10},
		},
	})
	nodeClass := &v1alpha1.CCENodeClass{}

	if _, err := reconciler.Reconcile(context.Background(), nodeClass); err != nil {
		t.Fatalf("reconciling subnet status: %v", err)
	}
	if len(nodeClass.Status.Subnets) != 1 {
		t.Fatalf("expected 1 subnet, got %d", len(nodeClass.Status.Subnets))
	}
	if nodeClass.Status.Subnets[0].ID != "subnet-123" {
		t.Fatalf("expected subnet ID %q, got %q", "subnet-123", nodeClass.Status.Subnets[0].ID)
	}
	if condition := nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady); !condition.IsTrue() {
		t.Fatalf("expected subnets ready condition to be true, got %#v", condition)
	}
}

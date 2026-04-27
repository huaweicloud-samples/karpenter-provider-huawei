package nodeclass

import (
	"context"
	"net/http"
	"testing"
	"time"

	cms "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cms/v1/model"
	vpcMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func (s *stubSubnetProvider) ZonalSubnetsForLaunch(context.Context, *v1alpha1.CCENodeClass, []*cloudprovider.InstanceType, string) (map[string]*providersubnet.Subnet, error) {
	return nil, nil
}

func (s *stubSubnetProvider) UpdateInflightIPs(*cms.CreateAutoLaunchGroupRequest, *cms.CreateAutoLaunchGroupResponse, []*cloudprovider.InstanceType, []*providersubnet.Subnet, string) {
}

func TestSubnetReconcilerFallsBackToNodeZone(t *testing.T) {
	if err := corev1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("adding core scheme: %v", err)
	}
	kubeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
			Labels: map[string]string{
				subnetIDLabelKey:         "subnet-123",
				corev1.LabelTopologyZone: "zone-a",
			},
		},
	}).Build()
	reconciler := NewSubnetReconciler(kubeClient, &stubSubnetProvider{
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
	if nodeClass.Status.Subnets[0].Zone != "zone-a" {
		t.Fatalf("expected subnet zone %q, got %q", "zone-a", nodeClass.Status.Subnets[0].Zone)
	}
	if condition := nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady); !condition.IsTrue() {
		t.Fatalf("expected subnets ready condition to be true, got %#v", condition)
	}
}

func TestSubnetReconcilerMarksSubnetsNotReadyWhenZoneCannotBeResolved(t *testing.T) {
	if err := corev1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("adding core scheme: %v", err)
	}
	kubeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	reconciler := NewSubnetReconciler(kubeClient, &stubSubnetProvider{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", AvailabilityZone: "", AvailableIpAddressCount: 10},
		},
	})
	nodeClass := &v1alpha1.CCENodeClass{}

	result, err := reconciler.Reconcile(context.Background(), nodeClass)
	if err != nil {
		t.Fatalf("reconciling subnet status: %v", err)
	}
	if result != (reconcile.Result{RequeueAfter: time.Minute}) {
		t.Fatalf("expected requeue after %s, got %#v", time.Minute, result)
	}
	if condition := nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady); condition.IsTrue() || condition.Reason != "SubnetZonesNotResolved" {
		t.Fatalf("expected unresolved zone condition, got %#v", condition)
	}
}

func TestSubnetReconcilerExpandsSubnetAcrossObservedZones(t *testing.T) {
	if err := corev1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("adding core scheme: %v", err)
	}
	kubeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-a",
				Labels: map[string]string{
					subnetIDLabelKey:         "subnet-123",
					corev1.LabelTopologyZone: "zone-a",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-b",
				Labels: map[string]string{
					subnetIDLabelKey:         "subnet-123",
					corev1.LabelTopologyZone: "zone-b",
				},
			},
		},
	).Build()
	reconciler := NewSubnetReconciler(kubeClient, &stubSubnetProvider{
		subnets: []vpcMdl.Subnet{
			{Id: "subnet-123", AvailabilityZone: "", AvailableIpAddressCount: 10},
		},
	})
	nodeClass := &v1alpha1.CCENodeClass{}

	if _, err := reconciler.Reconcile(context.Background(), nodeClass); err != nil {
		t.Fatalf("reconciling subnet status: %v", err)
	}
	if len(nodeClass.Status.Subnets) != 2 {
		t.Fatalf("expected 2 resolved subnet entries, got %d", len(nodeClass.Status.Subnets))
	}
	if got := nodeClass.Status.Subnets[0]; got.ID != "subnet-123" || got.Zone != "zone-a" {
		t.Fatalf("expected first subnet entry subnet-123/zone-a, got %#v", got)
	}
	if got := nodeClass.Status.Subnets[1]; got.ID != "subnet-123" || got.Zone != "zone-b" {
		t.Fatalf("expected second subnet entry subnet-123/zone-b, got %#v", got)
	}
	if condition := nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady); !condition.IsTrue() {
		t.Fatalf("expected subnets ready condition to be true, got %#v", condition)
	}
}

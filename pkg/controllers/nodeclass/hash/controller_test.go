package hash

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
)

func TestReconcile_PatchesMissingHashAnnotations(t *testing.T) {
	nodeClass := &v1alpha1.CCENodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: v1alpha1.CCENodeClassSpec{
			SubnetSelectorTerms: []v1alpha1.SubnetSelectorTerm{{ID: "123e4567-e89b-12d3-a456-426614174000"}},
			IMSSelector:         v1alpha1.IMSSelector{IMSFamily: "Huawei Cloud EulerOS 2.0"},
			BlockDeviceMappings: v1alpha1.BlockDeviceMappings{
				Root: v1alpha1.BlockDevice{VolumeSize: 120, VolumeType: "SAS"},
			},
			Login: v1alpha1.Login{
				UserPassword: v1alpha1.UserPassword{Password: "ciphertext"},
			},
		},
	}

	kubeClient := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		WithIndex(&karpv1.NodeClaim{}, "spec.nodeClassRef.group", func(obj client.Object) []string {
			nc := obj.(*karpv1.NodeClaim)
			if nc.Spec.NodeClassRef == nil {
				return nil
			}
			return []string{nc.Spec.NodeClassRef.Group}
		}).
		WithIndex(&karpv1.NodeClaim{}, "spec.nodeClassRef.kind", func(obj client.Object) []string {
			nc := obj.(*karpv1.NodeClaim)
			if nc.Spec.NodeClassRef == nil {
				return nil
			}
			return []string{nc.Spec.NodeClassRef.Kind}
		}).
		WithIndex(&karpv1.NodeClaim{}, "spec.nodeClassRef.name", func(obj client.Object) []string {
			nc := obj.(*karpv1.NodeClaim)
			if nc.Spec.NodeClassRef == nil {
				return nil
			}
			return []string{nc.Spec.NodeClassRef.Name}
		}).
		WithObjects(nodeClass).
		Build()
	controller := NewController(kubeClient)

	if _, err := controller.Reconcile(context.Background(), nodeClass); err != nil {
		t.Fatalf("reconciling nodeclass hash: %v", err)
	}

	updated := &v1alpha1.CCENodeClass{}
	if err := kubeClient.Get(context.Background(), client.ObjectKey{Name: "default"}, updated); err != nil {
		t.Fatalf("getting updated nodeclass: %v", err)
	}
	if got := updated.Annotations[v1alpha1.AnnotationCCENodeClassHash]; got != nodeClass.Hash() {
		t.Fatalf("expected ccenodeclass hash annotation %q, got %q", nodeClass.Hash(), got)
	}
	if got := updated.Annotations[v1alpha1.AnnotationCCENodeClassHashVersion]; got != v1alpha1.CCENodeClassHashVersion {
		t.Fatalf("expected ccenodeclass hash version %q, got %q", v1alpha1.CCENodeClassHashVersion, got)
	}
}

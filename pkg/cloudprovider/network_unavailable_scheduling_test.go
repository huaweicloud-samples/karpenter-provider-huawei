package cloudprovider

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	provisioningscheduling "sigs.k8s.io/karpenter/pkg/controllers/provisioning/scheduling"
	karpstate "sigs.k8s.io/karpenter/pkg/controllers/state"
	karpscheduling "sigs.k8s.io/karpenter/pkg/scheduling"
)

func TestManagedUninitializedNodeTreatsNetworkUnavailableAsKnownEphemeralTaint(t *testing.T) {
	originalKnownEphemeralTaints := append([]corev1.Taint(nil), karpscheduling.KnownEphemeralTaints...)
	t.Cleanup(func() {
		karpscheduling.KnownEphemeralTaints = originalKnownEphemeralTaints
	})

	networkUnavailableTaint := corev1.Taint{
		Key:    corev1.TaintNodeNetworkUnavailable,
		Effect: corev1.TaintEffectNoSchedule,
	}
	withoutNetworkUnavailable := make([]corev1.Taint, 0, len(originalKnownEphemeralTaints))
	for _, t := range originalKnownEphemeralTaints {
		if !t.MatchTaint(&networkUnavailableTaint) {
			withoutNetworkUnavailable = append(withoutNetworkUnavailable, t)
		}
	}

	podRequests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100m"),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.9",
				Resources: corev1.ResourceRequirements{
					Requests: podRequests,
				},
			}},
		},
	}

	testCases := []struct {
		name    string
		prepare func()
		wantErr bool
	}{
		{
			name: "without cce ephemeral taint registration",
			prepare: func() {
				karpscheduling.KnownEphemeralTaints = withoutNetworkUnavailable
			},
			wantErr: true,
		},
		{
			name: "with cce ephemeral taint registration",
			prepare: func() {
				karpscheduling.KnownEphemeralTaints = append(withoutNetworkUnavailable, corev1.Taint{
					Key:    corev1.TaintNodeNetworkUnavailable,
					Effect: corev1.TaintEffectNoSchedule,
				})
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepare != nil {
				tc.prepare()
			}
			stateNode := newManagedUninitializedStateNode()
			existingNode := provisioningscheduling.NewExistingNode(
				stateNode,
				&provisioningscheduling.Topology{},
				stateNode.Taints(),
				corev1.ResourceList{},
			)

			_, err := existingNode.CanAdd(pod, &provisioningscheduling.PodData{
				Requests:           podRequests,
				Requirements:       karpscheduling.NewRequirements(),
				StrictRequirements: karpscheduling.NewRequirements(),
			}, karpscheduling.Volumes{})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected pod to be rejected by network-unavailable taint")
				}
				if !strings.Contains(err.Error(), corev1.TaintNodeNetworkUnavailable) {
					t.Fatalf("expected taint rejection to mention %q, got %v", corev1.TaintNodeNetworkUnavailable, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected pod to fit once network-unavailable is treated as known ephemeral taint, got %v", err)
			}
		})
	}
}

func newManagedUninitializedStateNode() *karpstate.StateNode {
	allocatable := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
		corev1.ResourcePods:   resource.MustParse("16"),
	}
	node := karpstate.NewNode()
	node.NodeClaim = &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "nodeclaim-a"},
		Status: karpv1.NodeClaimStatus{
			Capacity:    allocatable,
			Allocatable: allocatable,
		},
	}
	node.Node = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-a",
			Labels: map[string]string{
				karpv1.NodeRegisteredLabelKey: "true",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{
				Key:    corev1.TaintNodeNetworkUnavailable,
				Effect: corev1.TaintEffectNoSchedule,
			}},
		},
		Status: corev1.NodeStatus{
			Capacity:    allocatable,
			Allocatable: allocatable,
		},
	}
	return node
}

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

package unregisteredtaint

import (
	"testing"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestIsKarpenterRegisteredWithStaleTaint(t *testing.T) {
	tests := []struct {
		name   string
		node   *corev1.Node
		expect bool
	}{
		{
			name: "RegisteredWithUnregisteredTaint",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{karpv1.NodeRegisteredLabelKey: "true"},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{karpv1.UnregisteredNoExecuteTaint},
				},
			},
			expect: true,
		},
		{
			name: "RegisteredWithoutTaint",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{karpv1.NodeRegisteredLabelKey: "true"},
				},
			},
			expect: false,
		},
		{
			name: "NotRegisteredWithTaint",
			node: &corev1.Node{
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{karpv1.UnregisteredNoExecuteTaint},
				},
			},
			expect: false,
		},
		{
			name:   "NotRegisteredWithoutTaint",
			node:   &corev1.Node{},
			expect: false,
		},
		{
			name: "RegisteredWithMixedTaints",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{karpv1.NodeRegisteredLabelKey: "true"},
				},
				Spec: corev1.NodeSpec{
					Taints: []corev1.Taint{
						{Key: "example.com/custom", Effect: corev1.TaintEffectNoSchedule},
						karpv1.UnregisteredNoExecuteTaint,
					},
				},
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKarpenterRegisteredWithStaleTaint(tt.node)
			if got != tt.expect {
				t.Fatalf("expected %v, got %v for node labels=%v taints=%v",
					tt.expect, got, tt.node.Labels,
					lo.Map(tt.node.Spec.Taints, func(t corev1.Taint, _ int) string {
						return t.Key + ":" + string(t.Effect)
					}))
			}
		})
	}
}

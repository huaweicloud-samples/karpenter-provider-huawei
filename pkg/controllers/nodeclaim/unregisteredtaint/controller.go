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
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// Controller removes the karpenter.sh/unregistered:NoExecute taint from
// Nodes that karpenter has already marked as registered.
//
// CCE converts CreateNode taints into kubelet --register-with-taints,
// so karpenter's registration patch (which removes the taint) races
// with CCE's node-controller Synced operation. When CCE wins the race,
// the taint survives even though registration completed. This controller
// acts as a reliable second pass to clean it up.
type Controller struct {
	kubeClient client.Client
}

func NewController(kubeClient client.Client) *Controller {
	return &Controller{kubeClient: kubeClient}
}

func (c *Controller) Name() string {
	return "node.unregisteredtaint"
}

func (c *Controller) Reconcile(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	if !isKarpenterRegisteredWithStaleTaint(node) {
		return reconcile.Result{}, nil
	}

	stored := node.DeepCopy()
	node.Spec.Taints = lo.Reject(node.Spec.Taints, func(t corev1.Taint, _ int) bool {
		return t.MatchTaint(&karpv1.UnregisteredNoExecuteTaint)
	})

	if equality.Semantic.DeepEqual(stored.Spec.Taints, node.Spec.Taints) {
		return reconcile.Result{}, nil
	}

	if err := c.kubeClient.Patch(ctx, node, client.MergeFromWithOptions(stored, client.MergeFromWithOptimisticLock{})); err != nil {
		if errors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("removing stale unregistered taint, %w", err)
	}
	log.FromContext(ctx).Info("removed stale karpenter.sh/unregistered taint")
	return reconcile.Result{}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(c.Name()).
		For(&corev1.Node{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(o client.Object) bool {
				return isKarpenterRegisteredWithStaleTaint(o.(*corev1.Node))
			}),
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return true },
				DeleteFunc: func(e event.DeleteEvent) bool { return false },
			},
		)).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}

func isKarpenterRegisteredWithStaleTaint(node *corev1.Node) bool {
	if node.Labels[karpv1.NodeRegisteredLabelKey] != "true" {
		return false
	}
	_, hasTaint := lo.Find(node.Spec.Taints, func(t corev1.Taint) bool {
		return t.MatchTaint(&karpv1.UnregisteredNoExecuteTaint)
	})
	return hasTaint
}

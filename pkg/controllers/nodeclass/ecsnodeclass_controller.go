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

package nodeclass

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	karpenterv1alpha1 "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
)

// ECSNodeClassReconciler reconciles a ECSNodeClass object
type ECSNodeClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=karpenter.k8s.huawei,resources=ecsnodeclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=karpenter.k8s.huawei,resources=ecsnodeclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=karpenter.k8s.huawei,resources=ecsnodeclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;nodes;persistentvolumes;persistentvolumeclaims;replicationcontrollers;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=create;patch;delete;update
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=pods,verbs=delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets;deployments;replicasets;statefulsets,verbs=list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses;csinodes;volumeattachments,verbs=get;list;watch
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodepools;nodepools/status;nodeclaims;nodeclaims/status;nodeoverlays;nodeoverlays/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodeclaims;nodeclaims/status;nodeclaims/finalizers,verbs=create;delete;update;patch
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodepools;nodepools/status;nodepools/finalizers;nodeoverlays/status,verbs=update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ECSNodeClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.0/pkg/reconcile
func (r *ECSNodeClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ECSNodeClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karpenterv1alpha1.ECSNodeClass{}).
		Named("ecsnodeclass").
		Complete(r)
}

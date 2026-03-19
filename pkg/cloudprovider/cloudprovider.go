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
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instance"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instancetype"
)

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

// CloudProvider implements Karpenter's CloudProvider interface for Huawei Cloud.
type CloudProvider struct {
	kubeClient           client.Client
	recorder             events.Recorder
	instanceTypeProvider instancetype.Provider
	instanceProvider     instance.Provider
}

// New creates a Huawei CloudProvider implementation.
func New(kubeClient client.Client, recorder events.Recorder, instanceTypeProvider instancetype.Provider, instanceProvider instance.Provider) *CloudProvider {
	return &CloudProvider{
		kubeClient:           kubeClient,
		recorder:             recorder,
		instanceTypeProvider: instanceTypeProvider,
		instanceProvider:     instanceProvider,
	}
}

// Create is called by Karpenter to provision capacity for a NodeClaim.
func (c *CloudProvider) Create(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*karpv1.NodeClaim, error) {
	nodeClass, err := c.resolveNodeClassFromNodeClaim(ctx, nodeClaim)
	if err != nil {
		if errors.IsNotFound(err) {
			c.recorder.Publish(events.Event{
				InvolvedObject: nodeClaim,
				Type:           corev1.EventTypeWarning,
				Reason:         "NodeClassNotFound",
				Message:        "Failed To Resolve NodeClass",
				DedupeValues:   []string{string(nodeClaim.UID)},
			})
			return nil, cloudprovider.NewNodeClassNotReadyError(err)
		}
		return nil, fmt.Errorf("resolving nodeclass, %w", err)
	}
	subnetsReady := nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady)
	if !subnetsReady.IsTrue() || len(nodeClass.Status.Subnets) == 0 {
		return nil, cloudprovider.NewNodeClassNotReadyError(fmt.Errorf("nodeclass subnets not ready: %s", subnetsReady.Message))
	}
	osAlias, nodeImageID, err := nodeClass.Spec.ResolveHMIForCreateNode()
	if err != nil {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("resolving nodeclass hmiSelectorTerms, %w", err), "InvalidNodeClass", "Invalid NodeClass hmiSelectorTerms")
	}
	imageID := nodeImageID
	if imageID == "" {
		imageID = osAlias
	}
	instanceTypes, err := c.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("resolving instance types, %w", err), "InstanceTypeResolutionFailed", "Error resolving instance types")
	}

	if c.instanceProvider == nil {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("instance provider is nil"), "InstanceProviderNotConfigured", "Instance provider is not configured")
	}

	createdInstance, err := c.instanceProvider.Create(ctx, nodeClass, nodeClaim, nil, instanceTypes)
	if err != nil {
		if cloudprovider.IsInsufficientCapacityError(err) || cloudprovider.IsNodeClassNotReadyError(err) {
			return nil, err
		}
		return nil, cloudprovider.NewCreateError(fmt.Errorf("creating instance, %w", err), "InstanceCreationFailed", "Error creating instance")
	}
	providerID := createdInstance.NodeID
	if providerID == "" {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("CreateNode succeeded but nodeID is empty"), "ProviderIDResolutionFailed", "Error resolving providerID")
	}
	instanceType, found := lo.Find(instanceTypes, func(it *cloudprovider.InstanceType) bool {
		return it.Name == createdInstance.Flavor
	})
	if !found {
		return nil, cloudprovider.NewCreateError(fmt.Errorf("selected instance type %q not found", createdInstance.Flavor), "InstanceTypeNotFound", "Selected instance type not found")
	}

	nc := &karpv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelInstanceTypeStable: createdInstance.Flavor,
				corev1.LabelTopologyZone:       createdInstance.Zone,
				karpv1.CapacityTypeLabelKey:    karpv1.CapacityTypeOnDemand,
			},
			Annotations: map[string]string{
				"karpenter.k8s.huawei/cce-node-id":   createdInstance.NodeID,
				"karpenter.k8s.huawei/ecs-server-id": createdInstance.ServerID,
			},
		},
		Status: karpv1.NodeClaimStatus{
			ProviderID:  providerID,
			ImageID:     imageID,
			Capacity:    instanceType.Capacity,
			Allocatable: instanceType.Allocatable(),
		},
	}
	if createdInstance.JobID != "" {
		if nc.Annotations == nil {
			nc.Annotations = map[string]string{}
		}
		nc.Annotations["karpenter.k8s.huawei/cce-job-id"] = createdInstance.JobID
	}
	return nc, nil
}

// Delete is called by Karpenter to deprovision capacity for a NodeClaim.
func (c *CloudProvider) Delete(ctx context.Context, nodeClaim *karpv1.NodeClaim) error {
	if c.instanceProvider == nil {
		return fmt.Errorf("instance provider is nil")
	}
	if nodeClaim == nil || nodeClaim.Status.ProviderID == "" {
		return nil
	}
	return c.instanceProvider.Delete(ctx, nodeClaim.Status.ProviderID)
}

// Get returns the NodeClaim by provider ID.
func (c *CloudProvider) Get(ctx context.Context, providerID string) (*karpv1.NodeClaim, error) {
	if c.instanceProvider == nil {
		return nil, fmt.Errorf("instance provider is nil")
	}
	if _, err := c.instanceProvider.Get(ctx, providerID); err != nil {
		return nil, err
	}
	return &karpv1.NodeClaim{
		Status: karpv1.NodeClaimStatus{
			ProviderID: providerID,
		},
	}, nil
}

// List returns all NodeClaims managed by this provider.
func (c *CloudProvider) List(ctx context.Context) ([]*karpv1.NodeClaim, error) {
	if c.instanceProvider == nil {
		return nil, fmt.Errorf("instance provider is nil")
	}
	instances, err := c.instanceProvider.List(ctx)
	if err != nil {
		return nil, err
	}
	nodeClaims := make([]*karpv1.NodeClaim, 0, len(instances))
	for _, inst := range instances {
		if inst == nil || inst.NodeID == "" {
			continue
		}
		nodeClaims = append(nodeClaims, &karpv1.NodeClaim{
			Status: karpv1.NodeClaimStatus{
				ProviderID: inst.NodeID,
			},
		})
	}
	return nodeClaims, nil
}

// GetInstanceTypes returns the instance types available for a given NodePool.
func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *karpv1.NodePool) ([]*cloudprovider.InstanceType, error) {
	nodeClass, err := c.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if errors.IsNotFound(err) {
			c.recorder.Publish(events.Event{
				InvolvedObject: nodePool,
				Type:           corev1.EventTypeWarning,
				Reason:         "NodeClassNotFound",
				Message:        "Failed To Resolve NodeClass",
				DedupeValues:   []string{string(nodePool.UID)},
			})
			return nil, nil
		}
		return nil, fmt.Errorf("resolving nodeclass, %w", err)
	}
	instanceTypes, err := c.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}
	return instanceTypes, nil
}

// IsDrifted indicates whether the NodeClaim is drifted from desired state.
func (c *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim) (cloudprovider.DriftReason, error) {
	//TODO implement me
	return cloudprovider.DriftReason(""), nil
}

// RepairPolicies returns the node repair policies supported by this provider.
func (c *CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	//TODO implement me
	return nil
}

// Name returns the provider name.
func (c *CloudProvider) Name() string {
	return "huawei"
}

// GetSupportedNodeClasses returns the NodeClass types supported by this provider.
func (c *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{&v1alpha1.ECSNodeClass{}}
}

func (c *CloudProvider) resolveNodeClassFromNodeClaim(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*v1alpha1.ECSNodeClass, error) {
	nodeClass := &v1alpha1.ECSNodeClass{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodeClaim.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
	return nodeClass, nil
}

func (c *CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *karpv1.NodePool) (*v1alpha1.ECSNodeClass, error) {
	nodeClass := &v1alpha1.ECSNodeClass{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
	return nodeClass, nil
}

// newTerminatingNodeClassError returns a NotFound error for handling by
func newTerminatingNodeClassError(name string) *errors.StatusError {
	qualifiedResource := schema.GroupResource{Group: apis.Group, Resource: "ecsnodeclasses"}
	err := errors.NewNotFound(qualifiedResource, name)
	err.ErrStatus.Message = fmt.Sprintf("%s %q is terminating, treating as not found", qualifiedResource.String(), name)
	return err
}

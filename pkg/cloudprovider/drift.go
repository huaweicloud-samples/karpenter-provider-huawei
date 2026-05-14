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

	"github.com/samber/lo"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/instance"
)

const (
	SubnetDrift    cloudprovider.DriftReason = "SubnetDrift"
	NodeClassDrift cloudprovider.DriftReason = "NodeClassDrift"
)

func (c *CloudProvider) isNodeClassDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim, nodeClass *v1alpha1.CCENodeClass) (cloudprovider.DriftReason, error) {
	if drifted := c.areStaticFieldsDrifted(nodeClaim, nodeClass); drifted != "" {
		return drifted, nil
	}
	instance, err := c.getInstance(ctx, nodeClaim.Status.ProviderID)
	if err != nil {
		return "", err
	}
	subnetDrifted, err := c.isSubnetDrifted(instance, nodeClass)
	if err != nil {
		return "", fmt.Errorf("calculating subnet drift, %w", err)
	}
	return subnetDrifted, nil
}

func (c *CloudProvider) areStaticFieldsDrifted(nodeClaim *karpv1.NodeClaim, nodeClass *v1alpha1.CCENodeClass) cloudprovider.DriftReason {
	nodeClassHash, foundNodeClassHash := nodeClass.Annotations[v1alpha1.AnnotationCCENodeClassHash]
	nodeClassHashVersion, foundNodeClassHashVersion := nodeClass.Annotations[v1alpha1.AnnotationCCENodeClassHashVersion]
	nodeClaimHash, foundNodeClaimHash := nodeClaim.Annotations[v1alpha1.AnnotationCCENodeClassHash]
	nodeClaimHashVersion, foundNodeClaimHashVersion := nodeClaim.Annotations[v1alpha1.AnnotationCCENodeClassHashVersion]

	if !foundNodeClassHash || !foundNodeClaimHash || !foundNodeClassHashVersion || !foundNodeClaimHashVersion {
		return ""
	}
	// validate that the hash version for the CCENodeClass is the same as the NodeClaim before evaluating for static drift
	if nodeClassHashVersion != nodeClaimHashVersion {
		return ""
	}
	return lo.Ternary(nodeClassHash != nodeClaimHash, NodeClassDrift, "")
}

func (c *CloudProvider) getInstance(ctx context.Context, providerID string) (*instance.Instance, error) {
	instance, err := c.instanceProvider.Get(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("getting instance, %w", err)
	}
	return instance, nil
}

func (c *CloudProvider) isSubnetDrifted(instance *instance.Instance, nodeClass *v1alpha1.CCENodeClass) (cloudprovider.DriftReason, error) {
	// subnets need to be found to check for drift
	if len(nodeClass.Status.Subnets) == 0 {
		return "", fmt.Errorf("no subnets are discovered")
	}

	_, found := lo.Find(nodeClass.Status.Subnets, func(subnet v1alpha1.Subnet) bool {
		return subnet.ID == instance.SubnetID
	})

	if !found {
		return SubnetDrift, nil
	}
	return "", nil
}

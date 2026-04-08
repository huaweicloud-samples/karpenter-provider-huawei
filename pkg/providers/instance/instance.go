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
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cceMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	ecsMdl "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"
	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
)

const (
	cceFlavorInsufficientErrorCode   = "cce_cm.0021"
	cceUnsupportedNetworkErrorCode   = "cce.01400025"
	insufficientInSpecifiedAZMessage = "insufficient in specified az"
	insufficientCapacityMessage      = "insufficient capacity"
	eniNetworkNotSupportedMessage    = "eni network is not supported"
	outOfStockMessage                = "out of stock"
	soldOutMessage                   = "sold out"
	sellOutMessage                   = "sell out"
)

type Provider interface {
	Create(context.Context, *v1alpha1.ECSNodeClass, *karpv1.NodeClaim, map[string]string, []*cloudprovider.InstanceType) (*Instance, error)
	Get(context.Context, string) (*Instance, error)
	List(context.Context) ([]*Instance, error)
	Delete(context.Context, string) error
	CreateTags(context.Context, string, map[string]string) error
}

type DefaultProvider struct {
	clusterID      string
	cceapi         sdk.CCEAPI
	ecsapi         sdk.ECSAPI
	subnetProvider subnet.Provider
}

func NewDefaultProvider(clusterID string, cceapi sdk.CCEAPI, ecsapi sdk.ECSAPI, subnetProvider subnet.Provider) Provider {
	return &DefaultProvider{
		clusterID:      clusterID,
		cceapi:         cceapi,
		ecsapi:         ecsapi,
		subnetProvider: subnetProvider,
	}
}

type createCandidate struct {
	instanceType *cloudprovider.InstanceType
	price        float64
	zone         string
	subnetID     string
}

func (p *DefaultProvider) Create(ctx context.Context, nodeClass *v1alpha1.ECSNodeClass, nodeClaim *karpv1.NodeClaim, tags map[string]string, instanceTypes []*cloudprovider.InstanceType) (*Instance, error) {
	if p.clusterID == "" {
		return nil, fmt.Errorf("CCE clusterID is empty")
	}
	osAlias, nodeImageID, err := nodeClass.Spec.ResolveHMIForCreateNode()
	if err != nil {
		return nil, err
	}

	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	// MVP: on-demand only
	if reqs.Has(karpv1.CapacityTypeLabelKey) && !reqs[karpv1.CapacityTypeLabelKey].Has(karpv1.CapacityTypeOnDemand) {
		return nil, fmt.Errorf("only %q is supported, got %q requirement", karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeLabelKey)
	}
	reqs.Add(scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand))

	allowedInstanceTypes := sets.New[string]()
	if req, ok := lo.Find(nodeClaim.Spec.Requirements, func(r karpv1.NodeSelectorRequirementWithMinValues) bool {
		return r.Key == corev1.LabelInstanceTypeStable
	}); ok {
		allowedInstanceTypes.Insert(req.Values...)
	}

	compatibleInstanceTypes := lo.Filter(instanceTypes, func(it *cloudprovider.InstanceType, _ int) bool {
		if len(allowedInstanceTypes) > 0 && !allowedInstanceTypes.Has(it.Name) {
			return false
		}
		return it.Offerings.Available().HasCompatible(reqs)
	})
	if len(compatibleInstanceTypes) == 0 {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("no compatible instance types for requirements"))
	}

	zonalSubnets, err := p.subnetProvider.ZonalSubnetsForLaunch(ctx, nodeClass, compatibleInstanceTypes, karpv1.CapacityTypeOnDemand)
	if err != nil {
		return nil, err
	}
	reservedSubnets := lo.Values(zonalSubnets)
	defer p.subnetProvider.UpdateInflightIPs(nil, nil, compatibleInstanceTypes, reservedSubnets, karpv1.CapacityTypeOnDemand)

	candidates := buildCreateCandidates(compatibleInstanceTypes, reqs, zonalSubnets)
	if len(candidates) == 0 {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("no compatible (instanceType, zone, subnet) candidates"))
	}

	var lastUnavailableErr error
	for _, c := range candidates {
		resp, err := p.cceapi.CreateNode(&cceMdl.CreateNodeRequest{
			ClusterId: p.clusterID,
			Body: &cceMdl.NodeCreateRequest{
				Kind:       "Node",
				ApiVersion: "v3",
				Spec:       p.nodeSpecForCandidate(nodeClass, nodeClaim, tags, c, osAlias, nodeImageID),
			},
		})
		if err != nil {
			if isInsufficientCapacityError(err) || isUnsupportedNetworkError(err) {
				lastUnavailableErr = err
				continue
			}
			return nil, err
		}
		if resp == nil || resp.Metadata == nil || resp.Metadata.Uid == nil {
			return nil, fmt.Errorf("CreateNode succeeded but response metadata.uid is empty")
		}
		instance := &Instance{
			NodeID: lo.FromPtr(resp.Metadata.Uid),
			Flavor: c.instanceType.Name,
			Zone:   c.zone,
		}
		if resp.Status != nil {
			instance.ServerID = lo.FromPtrOr(resp.Status.ServerId, "")
		}
		return instance, nil
	}

	if lastUnavailableErr != nil {
		if isInsufficientCapacityError(lastUnavailableErr) {
			return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("CreateNode failed for all candidates, last error: %w", lastUnavailableErr))
		}
		return nil, fmt.Errorf("CreateNode failed for all candidates, last error: %w", lastUnavailableErr)
	}
	return nil, fmt.Errorf("CreateNode failed for all candidates")
}

func buildCreateCandidates(instanceTypes []*cloudprovider.InstanceType, reqs scheduling.Requirements, zonalSubnets map[string]*subnet.Subnet) []createCandidate {
	seen := map[string]struct{}{}
	candidates := make([]createCandidate, 0, len(instanceTypes))
	for _, it := range instanceTypes {
		for _, of := range it.Offerings.Available().Compatible(reqs) {
			zoneReq, ok := of.Requirements[corev1.LabelTopologyZone]
			if !ok || zoneReq.Len() == 0 {
				continue
			}
			zone := zoneReq.Values()[0]
			zSubnet, ok := zonalSubnets[zone]
			if !ok || zSubnet == nil || zSubnet.ID == "" {
				continue
			}
			key := zone + "/" + it.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, createCandidate{
				instanceType: it,
				price:        of.Price,
				zone:         zone,
				subnetID:     zSubnet.ID,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].price != candidates[j].price {
			return candidates[i].price < candidates[j].price
		}
		if candidates[i].zone != candidates[j].zone {
			return candidates[i].zone < candidates[j].zone
		}
		return candidates[i].instanceType.Name < candidates[j].instanceType.Name
	})
	return candidates
}

func (p *DefaultProvider) nodeSpecForCandidate(nodeClass *v1alpha1.ECSNodeClass, nodeClaim *karpv1.NodeClaim, tags map[string]string, c createCandidate, osAlias, nodeImageID string) *cceMdl.NodeSpec {
	rootVolumeSize, rootVolumeType := resolveRootVolume(nodeClass)
	dataVolumes := defaultDataVolumes(rootVolumeType)

	k8sTags := map[string]string{}
	for k, v := range tags {
		k8sTags[k] = v
	}
	k8sTags["karpenter.k8s.huawei/nodeclaim-uid"] = string(nodeClaim.UID)

	subnetID := c.subnetID
	var os *string
	if strings.TrimSpace(osAlias) != "" {
		os = lo.ToPtr(strings.TrimSpace(osAlias))
	}
	spec := &cceMdl.NodeSpec{
		Flavor: c.instanceType.Name,
		Az:     c.zone,
		Os:     os,
		Count:  lo.ToPtr(int32(1)),
		RootVolume: &cceMdl.Volume{
			Size:       rootVolumeSize,
			Volumetype: rootVolumeType,
		},
		// CCE requires a data disk for standard ECS-backed worker nodes.
		DataVolumes: dataVolumes,
		OffloadNode: lo.ToPtr(true),
		NodeNicSpec: &cceMdl.NodeNicSpec{
			PrimaryNic: &cceMdl.NicSpec{SubnetId: &subnetID},
		},
		Taints:  toCCETaints(nodeClaim),
		K8sTags: k8sTags,
	}
	if strings.TrimSpace(nodeImageID) != "" {
		spec.ExtendParam = &cceMdl.NodeExtendParam{
			AlphaCceNodeImageID: lo.ToPtr(strings.TrimSpace(nodeImageID)),
		}
	}
	return spec
}

func resolveRootVolume(nodeClass *v1alpha1.ECSNodeClass) (int32, string) {
	size := int32(40)
	if nodeClass.Spec.RootVolume.Size != nil && *nodeClass.Spec.RootVolume.Size > 0 {
		size = *nodeClass.Spec.RootVolume.Size
	}
	volumeType := "SSD"
	if nodeClass.Spec.RootVolume.VolumeType != nil && strings.TrimSpace(*nodeClass.Spec.RootVolume.VolumeType) != "" {
		volumeType = strings.TrimSpace(*nodeClass.Spec.RootVolume.VolumeType)
	}
	return size, volumeType
}

func defaultDataVolumes(volumeType string) *[]cceMdl.Volume {
	volumes := []cceMdl.Volume{{
		Size:       100,
		Volumetype: volumeType,
	}}
	return lo.ToPtr(volumes)
}

func toCCETaints(nodeClaim *karpv1.NodeClaim) *[]cceMdl.Taint {
	var taints []corev1.Taint
	taints = append(taints, karpv1.UnregisteredNoExecuteTaint)
	taints = append(taints, nodeClaim.Spec.StartupTaints...)
	taints = append(taints, nodeClaim.Spec.Taints...)
	// CCE manages node.kubernetes.io/network-unavailable internally and rejects CreateNode
	// requests that try to set it explicitly.
	taints = lo.Reject(taints, func(t corev1.Taint, _ int) bool {
		return t.MatchTaint(&corev1.Taint{
			Key:    corev1.TaintNodeNetworkUnavailable,
			Effect: corev1.TaintEffectNoSchedule,
		})
	})

	// Deduplicate by (key,effect)
	deduped := map[string]corev1.Taint{}
	for _, t := range taints {
		key := t.Key + "/" + string(t.Effect)
		if key == karpv1.UnregisteredTaintKey+"/"+string(corev1.TaintEffectNoExecute) {
			deduped[key] = karpv1.UnregisteredNoExecuteTaint
			continue
		}
		deduped[key] = t
	}

	out := make([]cceMdl.Taint, 0, len(deduped))
	for _, t := range deduped {
		var v *string
		if t.Value != "" {
			v = lo.ToPtr(t.Value)
		}
		out = append(out, cceMdl.Taint{
			Key:    t.Key,
			Value:  v,
			Effect: toCCETaintEffect(t.Effect),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return lo.ToPtr(out)
}

func toCCETaintEffect(effect corev1.TaintEffect) cceMdl.TaintEffect {
	enums := cceMdl.GetTaintEffectEnum()
	switch effect {
	case corev1.TaintEffectPreferNoSchedule:
		return enums.PREFER_NO_SCHEDULE
	case corev1.TaintEffectNoExecute:
		return enums.NO_EXECUTE
	case corev1.TaintEffectNoSchedule:
		fallthrough
	default:
		return enums.NO_SCHEDULE
	}
}

func (p *DefaultProvider) Get(ctx context.Context, providerID string) (*Instance, error) {
	nodeID, err := nodeIDFromProviderID(providerID)
	if err != nil {
		return nil, err
	}
	resp, err := p.cceapi.ShowNode(&cceMdl.ShowNodeRequest{
		ClusterId: p.clusterID,
		NodeId:    nodeID,
	})
	if err != nil {
		if isNotFoundError(err) {
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return nil, err
	}
	if resp == nil {
		return nil, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("CCE node not found for nodeId=%q", nodeID))
	}
	return instanceFromCCEShowNodeResponse(resp), nil
}

func (p *DefaultProvider) List(ctx context.Context) ([]*Instance, error) {
	resp, err := p.cceapi.ListNodes(&cceMdl.ListNodesRequest{ClusterId: p.clusterID})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Items == nil {
		return nil, nil
	}
	out := make([]*Instance, 0, len(lo.FromPtr(resp.Items)))
	for _, n := range lo.FromPtr(resp.Items) {
		node := n
		inst := instanceFromCCENode(&node)
		if inst == nil || inst.NodeID == "" {
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

func (p *DefaultProvider) Delete(ctx context.Context, providerID string) error {
	nodeID, err := nodeIDFromProviderID(providerID)
	if err != nil {
		return err
	}
	_, err = p.cceapi.DeleteNode(&cceMdl.DeleteNodeRequest{
		ClusterId: p.clusterID,
		NodeId:    nodeID,
	})
	if err != nil {
		if isNotFoundError(err) {
			return cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return err
	}
	return nil
}

func (p *DefaultProvider) CreateTags(ctx context.Context, serverID string, tags map[string]string) error {
	_ = ctx

	if p.ecsapi == nil {
		return fmt.Errorf("ecsapi is nil")
	}
	if len(tags) == 0 {
		return nil
	}
	tagList := make([]ecsMdl.BatchAddServerTag, 0, len(tags))
	for k, v := range tags {
		var value *string
		if v != "" {
			value = lo.ToPtr(v)
		}
		tagList = append(tagList, ecsMdl.BatchAddServerTag{
			Key:   k,
			Value: value,
		})
	}
	_, err := p.ecsapi.BatchCreateServerTags(&ecsMdl.BatchCreateServerTagsRequest{
		ServerId: serverID,
		Body: &ecsMdl.BatchCreateServerTagsRequestBody{
			Action: ecsMdl.GetBatchCreateServerTagsRequestBodyActionEnum().CREATE,
			Tags:   tagList,
		},
	})
	return err
}

func instanceFromCCENodeParts(metadata *cceMdl.NodeMetadata, spec *cceMdl.NodeSpec, status *cceMdl.NodeStatus) *Instance {
	out := &Instance{}
	if metadata != nil && metadata.Uid != nil {
		out.NodeID = lo.FromPtr(metadata.Uid)
	}
	if status != nil && status.ServerId != nil {
		out.ServerID = lo.FromPtr(status.ServerId)
	}
	if spec != nil {
		out.Flavor = spec.Flavor
		out.Zone = spec.Az
		if spec.NodeNicSpec != nil && spec.NodeNicSpec.PrimaryNic != nil && spec.NodeNicSpec.PrimaryNic.SubnetId != nil {
			out.SubnetID = lo.FromPtr(spec.NodeNicSpec.PrimaryNic.SubnetId)
		}
	}
	return out
}

func instanceFromCCENode(node *cceMdl.Node) *Instance {
	if node == nil {
		return nil
	}
	return instanceFromCCENodeParts(node.Metadata, node.Spec, node.Status)
}

func instanceFromCCEShowNodeResponse(resp *cceMdl.ShowNodeResponse) *Instance {
	if resp == nil {
		return nil
	}
	return instanceFromCCENodeParts(resp.Metadata, resp.Spec, resp.Status)
}

func nodeIDFromProviderID(providerID string) (string, error) {
	nodeID := strings.TrimSpace(providerID)
	if nodeID == "" {
		return "", fmt.Errorf("providerID is empty")
	}
	return nodeID, nil
}

func isInsufficientCapacityError(err error) bool {
	var serviceErr *sdkerr.ServiceResponseError
	if !errors.As(err, &serviceErr) {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(serviceErr.ErrorCode))
	msg := strings.ToLower(strings.TrimSpace(serviceErr.ErrorMessage))

	if code != "" {
		return code == cceFlavorInsufficientErrorCode
	}
	for _, s := range []string{
		insufficientInSpecifiedAZMessage,
		insufficientCapacityMessage,
		outOfStockMessage,
		soldOutMessage,
		sellOutMessage,
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func isUnsupportedNetworkError(err error) bool {
	var serviceErr *sdkerr.ServiceResponseError
	if !errors.As(err, &serviceErr) {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(serviceErr.ErrorCode))
	msg := strings.ToLower(strings.TrimSpace(serviceErr.ErrorMessage))
	if code == cceUnsupportedNetworkErrorCode {
		return true
	}
	return strings.Contains(msg, eniNetworkNotSupportedMessage)
}

func isNotFoundError(err error) bool {
	var serviceErr *sdkerr.ServiceResponseError
	if errors.As(err, &serviceErr) {
		return serviceErr.StatusCode == 404
	}
	return false
}

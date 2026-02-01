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

	"github.com/awslabs/operatorpkg/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"
)

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

type CloudProvider struct {
	kubeClient client.Client
	recorder   events.Recorder
}

func New(kubeClient client.Client, recorder events.Recorder) cloudprovider.CloudProvider {
	return CloudProvider{
		kubeClient: kubeClient,
		recorder:   recorder,
	}
}

func (c CloudProvider) Create(ctx context.Context, claim *v1.NodeClaim) (*v1.NodeClaim, error) {
	//TODO implement me
	return nil, nil
}

func (c CloudProvider) Delete(ctx context.Context, claim *v1.NodeClaim) error {
	//TODO implement me
	return nil
}

func (c CloudProvider) Get(ctx context.Context, s string) (*v1.NodeClaim, error) {
	//TODO implement me
	return nil, nil
}

func (c CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	//TODO implement me
	return nil, nil
}

func (c CloudProvider) GetInstanceTypes(ctx context.Context, pool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	//TODO implement me
	return nil, nil
}

func (c CloudProvider) IsDrifted(ctx context.Context, claim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
	//TODO implement me
	return cloudprovider.DriftReason(""), nil
}

func (c CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	//TODO implement me
	return nil
}

func (c CloudProvider) Name() string {
	return "huawei"
}

func (c CloudProvider) GetSupportedNodeClasses() []status.Object {
	//TODO implement me
	return nil
}

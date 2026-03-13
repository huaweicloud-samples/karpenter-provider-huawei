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

package main

import (
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/overlay"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"

	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/cloudprovider"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/controllers"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/operator"
)

func main() {
	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	huaweicloudProvider := cloudprovider.New(
		op.GetClient(),
		op.EventRecorder,
		op.InstanceTypeProvider,
	)

	overlayUndecoratedCloudProvider := metrics.Decorate(huaweicloudProvider)
	cloudProvider := overlay.Decorate(overlayUndecoratedCloudProvider, op.GetClient(), op.InstanceTypeStore)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			overlayUndecoratedCloudProvider,
			clusterState,
			op.InstanceTypeStore,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.Manager,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			op.VersionProvider,
			op.SubnetProvider,
			op.InstanceTypeProvider,
		)...).
		Start(ctx)
}

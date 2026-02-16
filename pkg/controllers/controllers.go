/*
Copyright 2024 The CloudPilot AI Authors.

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

package controllers

import (
	"context"

	"github.com/awslabs/operatorpkg/controller"
	"github.com/awslabs/operatorpkg/status"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/apis/v1alpha1"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/version"

	controllersversion "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/controllers/providers/version"
)

// NewControllers returns the list of controllers managed by this provider.
func NewControllers(ctx context.Context, mgr manager.Manager, clk clock.Clock, kubeClient client.Client, recorder events.Recorder, _ cloudprovider.CloudProvider, versionProvider *version.DefaultProvider) []controller.Controller {

	controllers := []controller.Controller{
		controllersversion.NewController(versionProvider, versionProvider.UpdateVersionWithValidation),
		status.NewController[*v1alpha1.ECSNodeClass](kubeClient, mgr.GetEventRecorderFor("karpenter"), status.EmitDeprecatedMetrics),
	}
	return controllers
}

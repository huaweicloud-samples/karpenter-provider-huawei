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

package operator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	vpcRegion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/region"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"sigs.k8s.io/karpenter/pkg/operator"

	sdk "github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/huawei"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/subnet"
	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/version"
)

const (
	// DefaultTTL is the default cache TTL for Huawei Cloud API calls used during
	// setup verification and provisioning. Cache hits enable faster provisioning
	// and reduced API load, which can have a serious impact on performance and
	// scalability. DO NOT CHANGE THIS VALUE WITHOUT DUE CONSIDERATION.
	DefaultTTL = time.Minute
	// DefaultCleanupInterval triggers cache cleanup (lazy eviction) at this interval.
	DefaultCleanupInterval = time.Minute
	// AvailableIPAddressTTL is time to drop AvailableIPAddress data if it is not updated within the TTL
	AvailableIPAddressTTL = 5 * time.Minute
)

// Operator is injected into the HuaweiCloud CloudProvider's factories
type Operator struct {
	*operator.Operator
	VersionProvider *version.DefaultProvider
	SubnetProvider  subnet.Provider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	reg := os.Getenv("HUAWEICLOUD_REGION")
	region, err := vpcRegion.SafeValueOf(reg)
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get region: %w", err))
	}
	ak := os.Getenv("HUAWEICLOUD_AK")
	if ak == "" {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}
	sk := os.Getenv("HUAWEICLOUD_SK")
	if sk == "" {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}
	credentialsBuilder := basic.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk)
	if projectID := os.Getenv("HUAWEICLOUD_PROJECT_ID"); projectID != "" {
		credentialsBuilder.WithProjectId(projectID)
	}
	credentials, err := credentialsBuilder.SafeBuild()
	if err != nil {
		lo.Must0(fmt.Errorf("unable to get credentials"))
	}

	vpcApi := sdk.NewVPCService(region, credentials, config.DefaultHttpConfig())
	subnetProvider := subnet.NewDefaultProvider(vpcApi, cache.New(DefaultTTL, DefaultCleanupInterval), cache.New(AvailableIPAddressTTL, DefaultCleanupInterval))

	versionProvider := version.NewDefaultProvider(operator.KubernetesInterface)
	lo.Must0(versionProvider.UpdateVersionWithValidation(ctx))
	return ctx, &Operator{
		Operator:        operator,
		VersionProvider: versionProvider,
		SubnetProvider:  subnetProvider,
	}
}

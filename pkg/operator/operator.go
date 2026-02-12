package operator

import (
	"context"

	"github.com/samber/lo"
	"sigs.k8s.io/karpenter/pkg/operator"

	"github.com/HuaweiCloudDeveloper/karpenter-provider-huawei/pkg/providers/version"
)

// Operator is injected into the HuaweiCloud CloudProvider's factories
type Operator struct {
	*operator.Operator
	VersionProvider *version.DefaultProvider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	versionProvider := version.NewDefaultProvider(operator.KubernetesInterface)
	lo.Must0(versionProvider.UpdateVersionWithValidation(ctx))
	return ctx, &Operator{
		Operator:        operator,
		VersionProvider: versionProvider,
	}
}

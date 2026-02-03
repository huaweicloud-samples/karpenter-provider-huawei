package operator

import (
	"context"

	"sigs.k8s.io/karpenter/pkg/operator"
)

// Operator is injected into the HuaweiCloud CloudProvider's factories
type Operator struct {
	*operator.Operator
}

// NewOperator wraps the upstream Karpenter operator to add Huawei-specific wiring.
func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {

	return ctx, &Operator{
		Operator: operator,
	}
}

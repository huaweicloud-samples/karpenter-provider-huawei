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

package v1alpha1

import (
	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
)

const (
	ConditionTypeSubnetsReady        = "SubnetsReady"
	ConditionTypeValidationSucceeded = "ValidationSucceeded"
)

// CCENodeClassStatus defines the observed state of CCENodeClass.
type CCENodeClassStatus struct {
	// Subnets contains the current subnet values that are available to the
	// cluster under the subnet selectors.
	// +optional
	Subnets []Subnet `json:"subnets,omitempty"`
	// Conditions contains signals for health and readiness
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`
}

func (in *CCENodeClass) GetConditions() []status.Condition {
	return in.Status.Conditions
}

func (in *CCENodeClass) SetConditions(conditions []status.Condition) {
	in.Status.Conditions = conditions
}

func (in *CCENodeClass) StatusConditions() status.ConditionSet {
	conds := []string{
		ConditionTypeSubnetsReady,
	}
	return status.NewReadyConditions(conds...).For(in)
}

// Subnet contains resolved Subnet selector values utilized for node launch
type Subnet struct {
	// ID of the subnet
	// +required
	ID string `json:"id"`
	// The associated availability zone
	// +required
	Zone string `json:"zone"`
}

func (in *CCENodeClass) Zones() []string {
	return lo.Map(in.Status.Subnets, func(_ Subnet, i int) string {
		return in.Status.Subnets[i].Zone
	})
}

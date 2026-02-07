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

package providers

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/awslabs/operatorpkg/serrors"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/utils/pretty"
)

const (
	// Karpenter's supported version of Kubernetes
	// If a user runs a karpenter image on a k8s version outside the min and max,
	// One error message will be fired to notify
	MinK8sVersion = "1.26"
	MaxK8sVersion = "1.34"
)

type Provider interface {
	Get(ctx context.Context) string
}

// DefaultProvider get the APIServer version. This will be initialized at start up and allows karpenter to have an understanding of the cluster version
// for decision making. The version is cached to help reduce the amount of calls made to the API Server
type DefaultProvider struct {
	cm                  *pretty.ChangeMonitor
	kubernetesInterface kubernetes.Interface
	version             atomic.Pointer[string]
}

func NewDefaultProvider(kubernetesInterface kubernetes.Interface) *DefaultProvider {
	return &DefaultProvider{
		cm:                  pretty.NewChangeMonitor(),
		kubernetesInterface: kubernetesInterface,
	}
}

func (p DefaultProvider) Get(ctx context.Context) string {
	return *p.version.Load()
}

func (p *DefaultProvider) UpdateVersion(ctx context.Context) error {
	var version string
	var err error

	version, err = p.getK8sVersion()
	if err != nil {
		return fmt.Errorf("validating kubernetes version, %w", err)
	}
	p.version.Store(&version)
	return nil
}

func (p *DefaultProvider) UpdateVersionWithValidation(ctx context.Context) error {
	err := p.UpdateVersion(ctx)
	if err != nil {
		return err
	}
	version := p.Get(ctx)
	if p.cm.HasChanged("kubernetes-version", version) {
		log.FromContext(ctx).WithValues("version", version).V(1).Info("discovered kubernetes version")
		if err := validateK8sVersion(version); err != nil {
			return fmt.Errorf("validating kubernetes version, %w", err)
		}
	}
	return nil
}

func (p *DefaultProvider) getK8sVersion() (string, error) {
	output, err := p.kubernetesInterface.Discovery().ServerVersion()
	if err != nil || output == nil {
		return "", fmt.Errorf("getting kubernetes version from the kubernetes API")
	}
	return fmt.Sprintf("%s.%s", output.Major, strings.TrimSuffix(output.Minor, "+")), err
}

func validateK8sVersion(v string) error {
	k8sVersion := version.MustParseGeneric(v)

	// We will only error if the user is running karpenter on a k8s version,
	// that is out of the range of the minK8sVersion and maxK8sVersion
	if k8sVersion.LessThan(version.MustParseGeneric(MinK8sVersion)) ||
		version.MustParseGeneric(MaxK8sVersion).LessThan(k8sVersion) {
		return serrors.Wrap(fmt.Errorf("karpenter is not compatible with kubernetes version"), "version", k8sVersion)
	}

	return nil
}

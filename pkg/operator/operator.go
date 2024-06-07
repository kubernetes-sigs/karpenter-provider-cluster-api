/*
Copyright The Kubernetes Authors.

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

	"github.com/samber/lo"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis"
	api "sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	clusterapi "sigs.k8s.io/karpenter-provider-cluster-api/pkg/cloudprovider"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	"sigs.k8s.io/karpenter/pkg/apis/v1beta1"
	"sigs.k8s.io/karpenter/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/operator/scheme"
)

func init() {
	v1beta1.RestrictedLabelDomains = v1beta1.RestrictedLabelDomains.Insert(api.Group)
	v1beta1.WellKnownLabels = v1beta1.WellKnownLabels.Insert(
		clusterapi.InstanceSizeLabelKey,
		clusterapi.InstanceFamilyLabelKey,
		clusterapi.InstanceCPULabelKey,
		clusterapi.InstanceMemoryLabelKey,
	)
	lo.Must0(apis.AddToScheme(scheme.Scheme))
}

type Operator struct {
	*operator.Operator

	MachineProvider           machine.Provider
	MachineDeploymentProvider machinedeployment.Provider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	machineProvider := machine.NewDefaultProvider(ctx, operator.GetClient())
	machineDeploymentProvider := machinedeployment.NewDefaultProvider(ctx, operator.GetClient())

	return ctx, &Operator{
		Operator:                  operator,
		MachineProvider:           machineProvider,
		MachineDeploymentProvider: machineDeploymentProvider,
	}
}

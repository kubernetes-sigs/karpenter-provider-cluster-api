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
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis"
	api "sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	clusterapi "sigs.k8s.io/karpenter-provider-cluster-api/pkg/cloudprovider"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/operator/options"
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
	clusterAPIKubeConfig, err := clientcmd.BuildConfigFromFlags("", options.FromContext(ctx).ClusterAPIKubeConfigFile)
	if err != nil {
		klog.Fatalf("cannot build cluster-api kube config: %v", err)
	}

	mgmtCluster, err := cluster.New(clusterAPIKubeConfig, func(o *cluster.Options) {
		o.Scheme = operator.GetScheme()
	})
	if err != nil {
		klog.Fatalf("create cluster-api kube client failed: %v", err)
	}

	if err = operator.Add(mgmtCluster); err != nil {
		klog.Fatalf("added cluster-api kube client to operator failed: %v", err)
	}
	machineProvider := machine.NewDefaultProvider(ctx, mgmtCluster.GetClient())
	machineDeploymentProvider := machinedeployment.NewDefaultProvider(ctx, mgmtCluster.GetClient())

	return ctx, &Operator{
		Operator:                  operator,
		MachineProvider:           machineProvider,
		MachineDeploymentProvider: machineDeploymentProvider,
	}
}

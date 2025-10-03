/*
Copyright 2024 The Kubernetes Authors.

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
	"log"

	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	clusterapi "sigs.k8s.io/karpenter-provider-cluster-api/pkg/cloudprovider"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/operator/options"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/operator"
)

func init() {
	karpv1.RestrictedLabelDomains = karpv1.RestrictedLabelDomains.Insert(v1alpha1.Group)
	karpv1.WellKnownLabels = karpv1.WellKnownLabels.Insert(
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
	mgmtCluster, err := buildManagementClusterKubeClient(ctx, operator)
	if err != nil {
		log.Fatalf("unable to build management cluster client: %v", err)
	}

	machineProvider := machine.NewDefaultProvider(ctx, mgmtCluster)
	machineDeploymentProvider := machinedeployment.NewDefaultProvider(ctx, mgmtCluster)

	return ctx, &Operator{
		Operator:                  operator,
		MachineProvider:           machineProvider,
		MachineDeploymentProvider: machineDeploymentProvider,
	}
}

func buildManagementClusterKubeClient(ctx context.Context, operator *operator.Operator) (client.Client, error) {
	clusterAPIKubeConfig, err := buildClusterCAPIKubeConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to build cluster API kube config: %w", err)
	}

	if clusterAPIKubeConfig != nil {
		mgmtCluster, err := cluster.New(clusterAPIKubeConfig, func(o *cluster.Options) {
			o.Scheme = operator.GetScheme()
		})
		if err != nil {
			return nil, fmt.Errorf("unable to create new cluster for management cluster: %w", err)
		}
		if err = operator.Add(mgmtCluster); err != nil {
			return nil, fmt.Errorf("unable to add management cluster to operator: %w", err)
		}
		return mgmtCluster.GetClient(), nil
	}
	return operator.GetClient(), nil
}

func buildClusterCAPIKubeConfig(ctx context.Context) (*rest.Config, error) {
	kubeConfigFile := options.FromContext(ctx).ClusterAPIKubeConfigFile
	if kubeConfigFile != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfigFile)
	}

	url := options.FromContext(ctx).ClusterAPIUrl
	token := options.FromContext(ctx).ClusterAPIToken
	caData := options.FromContext(ctx).ClusterAPICertificateAuthorityData
	skipTLSVerify := options.FromContext(ctx).ClusterAPISkipTlsVerify
	if url != "" {
		return &rest.Config{
			Host:        url,
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData:   []byte(caData),
				Insecure: skipTLSVerify,
			},
		}, nil
	}

	return nil, nil
}

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

package machinedeployment

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

type Provider interface {
	Get(context.Context, string, string) (*capiv1beta1.MachineDeployment, error)
	List(context.Context, *metav1.LabelSelector) ([]*capiv1beta1.MachineDeployment, error)
	Update(context.Context, *capiv1beta1.MachineDeployment) error
}

type DefaultProvider struct {
	kubeClient client.Client
}

func NewDefaultProvider(_ context.Context, kubeClient client.Client) *DefaultProvider {
	return &DefaultProvider{
		kubeClient: kubeClient,
	}
}

func (p *DefaultProvider) Get(ctx context.Context, name string, namespace string) (*capiv1beta1.MachineDeployment, error) {
	machineDeployment := &capiv1beta1.MachineDeployment{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, machineDeployment)
	if err != nil {
		machineDeployment = nil
		return machineDeployment, fmt.Errorf("unable to get MachineDeployment %s in namespace %s: %w", name, namespace, err)
	}
	return machineDeployment, nil
}

func (p *DefaultProvider) List(ctx context.Context, selector *metav1.LabelSelector) ([]*capiv1beta1.MachineDeployment, error) {
	machineDeployments := []*capiv1beta1.MachineDeployment{}

	listOptions := []client.ListOption{
		client.MatchingLabels{
			providers.NodePoolMemberLabel: "",
		},
	}

	if selector != nil {
		sm, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return machineDeployments, fmt.Errorf("unable to convert selector in MachineDeployment List: %w", err)
		}
		listOptions = append(listOptions, &client.ListOptions{LabelSelector: sm})
	}
	machineDeploymentList := &capiv1beta1.MachineDeploymentList{}
	err := p.kubeClient.List(ctx, machineDeploymentList, listOptions...)
	if err != nil {
		return nil, fmt.Errorf("unable to list MachineDeployments with selector: %w", err)
	}

	for _, m := range machineDeploymentList.Items {
		machineDeployments = append(machineDeployments, &m)
	}

	return machineDeployments, nil
}

func (p *DefaultProvider) Update(ctx context.Context, machineDeployment *capiv1beta1.MachineDeployment) error {
	err := p.kubeClient.Update(ctx, machineDeployment)
	if err != nil {
		return fmt.Errorf("unable to update MachineDeployment %q: %w", machineDeployment.Name, err)
	}

	return nil
}

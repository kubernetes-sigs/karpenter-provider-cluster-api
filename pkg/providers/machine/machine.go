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

package machine

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider interface {
	Get(context.Context, string) (*capiv1beta1.Machine, error)
	List(context.Context, *metav1.LabelSelector) ([]*capiv1beta1.Machine, error)
	IsDeleting(*capiv1beta1.Machine) bool
	AddDeleteAnnotation(context.Context, *capiv1beta1.Machine) error
	RemoveDeleteAnnotation(context.Context, *capiv1beta1.Machine) error
	Update(context.Context, *capiv1beta1.Machine) error
}

type DefaultProvider struct {
	kubeClient client.Client
}

func NewDefaultProvider(_ context.Context, kubeClient client.Client) *DefaultProvider {
	return &DefaultProvider{
		kubeClient: kubeClient,
	}
}

// Get returns the Machine indicated by the supplied Provider ID or nil if not found.
// Because Get is used with a provider ID, it may return a Machine that does not have
// a label for node pool membership.
func (p *DefaultProvider) Get(ctx context.Context, providerID string) (*capiv1beta1.Machine, error) {
	machineList := &capiv1beta1.MachineList{}
	err := p.kubeClient.List(ctx, machineList)
	if err != nil {
		return nil, fmt.Errorf("unable to list machines during Machine Provider Get request: %w", err)
	}

	for _, m := range machineList.Items {
		if m.Spec.ProviderID != nil && *m.Spec.ProviderID == providerID {
			return &m, nil
		}
	}

	return nil, nil
}

func (p *DefaultProvider) List(ctx context.Context, selector *metav1.LabelSelector) ([]*capiv1beta1.Machine, error) {
	machines := []*capiv1beta1.Machine{}

	listOptions := []client.ListOption{}
	if selector != nil {
		sm, err := metav1.LabelSelectorAsSelector(selector)
		if err != nil {
			return machines, fmt.Errorf("unable to convert selector in MachineDeployment List: %w", err)
		}
		listOptions = append(listOptions, &client.ListOptions{LabelSelector: sm})
	}

	machineList := &capiv1beta1.MachineList{}
	err := p.kubeClient.List(ctx, machineList, listOptions...)
	if err != nil {
		return nil, err
	}

	for _, m := range machineList.Items {
		machines = append(machines, &m)
	}

	return machines, nil
}

// IsDeleting returns true if the supplied Machine has a non-zero deletion timestamp.
func (p *DefaultProvider) IsDeleting(machine *capiv1beta1.Machine) bool {
	return machine != nil && !machine.GetDeletionTimestamp().IsZero()
}

// AddDeleteAnnotation adds the Cluster API deletion annotation to a Machine resource and updates
// the API server. It returns an error if there is a failure.
func (p *DefaultProvider) AddDeleteAnnotation(ctx context.Context, machine *capiv1beta1.Machine) error {
	if machine == nil {
		return fmt.Errorf("cannot add deletion annotation to Machine, nil value")
	}

	annotations := machine.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	if _, found := annotations[capiv1beta1.DeleteMachineAnnotation]; !found {
		annotations[capiv1beta1.DeleteMachineAnnotation] = time.Now().String()
		machine.SetAnnotations(annotations)
		err := p.kubeClient.Update(ctx, machine)
		if err != nil {
			return fmt.Errorf("unable to add deletion annotation to Machine %q: %w", machine.Name, err)
		}
	}

	return nil
}

// RemoveDeleteAnnotation removes the Cluster API deletion annotation from a Machine resource and
// updates the API server. It returns an error if there is a failure.
func (p *DefaultProvider) RemoveDeleteAnnotation(ctx context.Context, machine *capiv1beta1.Machine) error {
	if machine == nil {
		return fmt.Errorf("cannot remove deletion annotation from Machine, nil value")
	}

	annotations := machine.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	if _, found := annotations[capiv1beta1.DeleteMachineAnnotation]; found {
		delete(annotations, capiv1beta1.DeleteMachineAnnotation)
		machine.SetAnnotations(annotations)
		err := p.kubeClient.Update(ctx, machine)
		if err != nil {
			return fmt.Errorf("unable to remove deletion annotation from Machine %q: %w", machine.Name, err)
		}
	}

	return nil
}

func (p *DefaultProvider) Update(ctx context.Context, machine *capiv1beta1.Machine) error {
	err := p.kubeClient.Update(ctx, machine)
	if err != nil {
		return fmt.Errorf("unable to update Machine%q: %w", machine.Name, err)
	}

	return nil
}

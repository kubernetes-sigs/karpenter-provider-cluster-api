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

package batcher_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// fakeMachineProvider implements machine.Provider for unit tests.
type fakeMachineProvider struct {
	mu       sync.Mutex
	machines map[string]*capiv1beta1.Machine // keyed by "namespace/name"

	GetCallCount              atomic.Int64
	ListCallCount             atomic.Int64
	UpdateCallCount           atomic.Int64
	AddDeleteAnnotationCount  atomic.Int64
	RemoveDeleteAnnotationCount atomic.Int64

	GetError              error
	ListError             error
	UpdateError           error
	AddDeleteAnnotationError    error
	RemoveDeleteAnnotationError error
}

func newFakeMachineProvider() *fakeMachineProvider {
	return &fakeMachineProvider{
		machines: make(map[string]*capiv1beta1.Machine),
	}
}

func (f *fakeMachineProvider) key(ns, name string) string {
	return ns + "/" + name
}

func (f *fakeMachineProvider) AddMachine(m *capiv1beta1.Machine) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machines[f.key(m.Namespace, m.Name)] = m.DeepCopy()
}

func (f *fakeMachineProvider) GetMachine(name, ns string) *capiv1beta1.Machine {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.machines[f.key(ns, name)]; ok {
		return m.DeepCopy()
	}
	return nil
}

func (f *fakeMachineProvider) Get(_ context.Context, name string, namespace string) (*capiv1beta1.Machine, error) {
	f.GetCallCount.Add(1)
	if f.GetError != nil {
		return nil, f.GetError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.machines[f.key(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("machine %s/%s not found", namespace, name)
	}
	return m.DeepCopy(), nil
}

func (f *fakeMachineProvider) GetByProviderID(_ context.Context, _ string) (*capiv1beta1.Machine, error) {
	return nil, fmt.Errorf("not implemented in fake")
}

func (f *fakeMachineProvider) List(_ context.Context, namespace string, selector *metav1.LabelSelector) ([]*capiv1beta1.Machine, error) {
	f.ListCallCount.Add(1)
	if f.ListError != nil {
		return nil, f.ListError
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	var result []*capiv1beta1.Machine
	for _, m := range f.machines {
		if namespace != "" && m.Namespace != namespace {
			continue
		}
		if sel != nil && !sel.Matches(labelSet(m.Labels)) {
			continue
		}
		result = append(result, m.DeepCopy())
	}
	return result, nil
}

func (f *fakeMachineProvider) IsDeleting(m *capiv1beta1.Machine) bool {
	return m != nil && !m.GetDeletionTimestamp().IsZero()
}

func (f *fakeMachineProvider) AddDeleteAnnotation(_ context.Context, m *capiv1beta1.Machine) error {
	f.AddDeleteAnnotationCount.Add(1)
	if f.AddDeleteAnnotationError != nil {
		return f.AddDeleteAnnotationError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.machines[f.key(m.Namespace, m.Name)]
	if !ok {
		return fmt.Errorf("machine %s/%s not found", m.Namespace, m.Name)
	}
	annotations := stored.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[capiv1beta1.DeleteMachineAnnotation] = "true"
	stored.SetAnnotations(annotations)
	return nil
}

func (f *fakeMachineProvider) RemoveDeleteAnnotation(_ context.Context, m *capiv1beta1.Machine) error {
	f.RemoveDeleteAnnotationCount.Add(1)
	if f.RemoveDeleteAnnotationError != nil {
		return f.RemoveDeleteAnnotationError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.machines[f.key(m.Namespace, m.Name)]
	if !ok {
		return fmt.Errorf("machine %s/%s not found", m.Namespace, m.Name)
	}
	annotations := stored.GetAnnotations()
	delete(annotations, capiv1beta1.DeleteMachineAnnotation)
	stored.SetAnnotations(annotations)
	return nil
}

func (f *fakeMachineProvider) Update(_ context.Context, m *capiv1beta1.Machine) error {
	f.UpdateCallCount.Add(1)
	if f.UpdateError != nil {
		return f.UpdateError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machines[f.key(m.Namespace, m.Name)] = m.DeepCopy()
	return nil
}

func (f *fakeMachineProvider) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machines = make(map[string]*capiv1beta1.Machine)
	f.GetCallCount.Store(0)
	f.ListCallCount.Store(0)
	f.UpdateCallCount.Store(0)
	f.AddDeleteAnnotationCount.Store(0)
	f.RemoveDeleteAnnotationCount.Store(0)
	f.GetError = nil
	f.ListError = nil
	f.UpdateError = nil
	f.AddDeleteAnnotationError = nil
	f.RemoveDeleteAnnotationError = nil
}

// fakeMDProvider implements machinedeployment.Provider for unit tests.
type fakeMDProvider struct {
	mu  sync.Mutex
	mds map[string]*capiv1beta1.MachineDeployment // keyed by "namespace/name"

	GetCallCount    atomic.Int64
	UpdateCallCount atomic.Int64

	GetError    error
	UpdateError error
}

func newFakeMDProvider() *fakeMDProvider {
	return &fakeMDProvider{
		mds: make(map[string]*capiv1beta1.MachineDeployment),
	}
}

func (f *fakeMDProvider) key(ns, name string) string {
	return ns + "/" + name
}

func (f *fakeMDProvider) AddMD(md *capiv1beta1.MachineDeployment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mds[f.key(md.Namespace, md.Name)] = md.DeepCopy()
}

func (f *fakeMDProvider) GetMD(name, ns string) *capiv1beta1.MachineDeployment {
	f.mu.Lock()
	defer f.mu.Unlock()
	if md, ok := f.mds[f.key(ns, name)]; ok {
		return md.DeepCopy()
	}
	return nil
}

func (f *fakeMDProvider) Get(_ context.Context, name string, namespace string) (*capiv1beta1.MachineDeployment, error) {
	f.GetCallCount.Add(1)
	if f.GetError != nil {
		return nil, f.GetError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	md, ok := f.mds[f.key(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("machinedeployment %s/%s not found", namespace, name)
	}
	return md.DeepCopy(), nil
}

func (f *fakeMDProvider) List(_ context.Context, _ *metav1.LabelSelector) ([]*capiv1beta1.MachineDeployment, error) {
	return nil, fmt.Errorf("not implemented in fake")
}

func (f *fakeMDProvider) Update(_ context.Context, md *capiv1beta1.MachineDeployment) error {
	f.UpdateCallCount.Add(1)
	if f.UpdateError != nil {
		return f.UpdateError
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mds[f.key(md.Namespace, md.Name)] = md.DeepCopy()
	return nil
}

func (f *fakeMDProvider) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mds = make(map[string]*capiv1beta1.MachineDeployment)
	f.GetCallCount.Store(0)
	f.UpdateCallCount.Store(0)
	f.GetError = nil
	f.UpdateError = nil
}

// labelSet adapts a map[string]string to labels.Labels for selector matching.
type labelSet map[string]string

func (l labelSet) Has(key string) bool {
	_, ok := l[key]
	return ok
}

func (l labelSet) Get(key string) string {
	return l[key]
}

// newMachineDeployment creates a MachineDeployment with the given name, namespace, and replicas.
func newMachineDeployment(name, namespace string, replicas int32) *capiv1beta1.MachineDeployment {
	return &capiv1beta1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: capiv1beta1.MachineDeploymentSpec{
			Replicas: ptr.To(replicas),
		},
	}
}

// newMachineForMD creates a Machine belonging to the given MachineDeployment.
func newMachineForMD(name, namespace, mdName string) *capiv1beta1.Machine {
	return &capiv1beta1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				capiv1beta1.MachineDeploymentNameLabel: mdName,
			},
		},
	}
}

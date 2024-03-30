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

package client

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MultiplexingClient struct{}

// controller-runtime client.Reader interface
// ---------------------------------------------------

func (mc MultiplexingClient) Get(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
	return nil
}

func (mc MultiplexingClient) List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error {
	return nil
}

// controller-runtime client.Writer interface
// ---------------------------------------------------

func (mc MultiplexingClient) Create(ctx context.Context, obj crclient.Object, opts ...crclient.CreateOption) error {
	return nil
}

func (mc MultiplexingClient) Delete(ctx context.Context, obj crclient.Object, opts ...crclient.DeleteOption) error {
	return nil
}

func (mc MultiplexingClient) Update(ctx context.Context, obj crclient.Object, opts ...crclient.UpdateOption) error {
	return nil
}

func (mc MultiplexingClient) Patch(ctx context.Context, obj crclient.Object, patch crclient.Patch, opts ...crclient.PatchOption) error {
	return nil
}

func (mc MultiplexingClient) DeleteAllOf(ctx context.Context, obj crclient.Object, opts ...crclient.DeleteAllOfOption) error {
	return nil
}

// controller-runtime client.StatusClient interface
// ---------------------------------------------------

func (mc MultiplexingClient) Status() crclient.SubResourceWriter {
	return nil
}

// controller-runtime client.SubResourceClientConstructor interface
// ---------------------------------------------------

func (mc MultiplexingClient) SubResource(subResource string) crclient.SubResourceClient {
	return nil
}

// controller-runtime client.Client interface
// ---------------------------------------------------

func (mc MultiplexingClient) Scheme() *runtime.Scheme {
	return nil
}

func (mc MultiplexingClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (mc MultiplexingClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	gvk := schema.GroupVersionKind{}

	return gvk, nil
}

func (mc MultiplexingClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return false, nil
}

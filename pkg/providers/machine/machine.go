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

package machine

import (
	"context"

	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nodePoolOwnedLabel = "cluster-api.karpenter.sh/node-pool-owned"
)

type Provider interface {
	List(context.Context) ([]*capiv1beta1.Machine, error)
}

type DefaultProvider struct {
	kubeClient client.Client
}

func NewDefaultProvider(_ context.Context, kubeClient client.Client) *DefaultProvider {
	return &DefaultProvider{
		kubeClient: kubeClient,
	}
}

func (p *DefaultProvider) List(ctx context.Context) ([]*capiv1beta1.Machine, error) {
	machines := []*capiv1beta1.Machine{}

	return machines, nil
}

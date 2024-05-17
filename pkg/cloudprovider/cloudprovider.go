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

package cloudprovider

import (
	"context"
	_ "embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	"sigs.k8s.io/karpenter/pkg/apis/v1beta1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

const (
	// TODO (elmiko) if we make these exposed constants from the CAS we can import them instead of redifining and risking drift
	cpuKey          = "capacity.cluster-autoscaler.kubernetes.io/cpu"
	memoryKey       = "capacity.cluster-autoscaler.kubernetes.io/memory"
	gpuCountKey     = "capacity.cluster-autoscaler.kubernetes.io/gpu-count"
	gpuTypeKey      = "capacity.cluster-autoscaler.kubernetes.io/gpu-type"
	diskCapacityKey = "capacity.cluster-autoscaler.kubernetes.io/ephemeral-disk"
	labelsKey       = "capacity.cluster-autoscaler.kubernetes.io/labels"
	taintsKey       = "capacity.cluster-autoscaler.kubernetes.io/taints"
	maxPodsKey      = "capacity.cluster-autoscaler.kubernetes.io/maxPods"
)

func NewCloudProvider(ctx context.Context, kubeClient client.Client, machineProvider machine.Provider, machineDeploymentProvider machinedeployment.Provider) *CloudProvider {
	return &CloudProvider{
		kubeClient:                kubeClient,
		machineProvider:           machineProvider,
		machineDeploymentProvider: machineDeploymentProvider,
	}
}

type CloudProvider struct {
	kubeClient                client.Client
	machineProvider           machine.Provider
	machineDeploymentProvider machinedeployment.Provider
}

func (c CloudProvider) Create(ctx context.Context, nodeClaim *v1beta1.NodeClaim) (*v1beta1.NodeClaim, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c CloudProvider) Delete(ctx context.Context, nodeClaim *v1beta1.NodeClaim) error {
	return fmt.Errorf("not implemented")
}

func (c CloudProvider) Get(ctx context.Context, providerID string) (*v1beta1.NodeClaim, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c CloudProvider) List(ctx context.Context) ([]*v1beta1.NodeClaim, error) {
	machines, err := c.machineProvider.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing machines, %w", err)
	}

	var nodeClaims []*v1beta1.NodeClaim
	for _, machine := range machines {
		nodeClaim, err := c.machineToNodeClaim(ctx, machine)
		if err != nil {
			return []*v1beta1.NodeClaim{}, err
		}
		nodeClaims = append(nodeClaims, nodeClaim)
	}

	return nodeClaims, nil
}

// Return the hard-coded instance types.
func (c CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1beta1.NodePool) ([]*cloudprovider.InstanceType, error) {
	instanceTypes := []*cloudprovider.InstanceType{}

	if nodePool == nil {
		return instanceTypes, fmt.Errorf("node pool reference is nil, no way to proceed")
	}

	// otherwise, get the details from the nodepool to inform which named nodeclass (if present) and other options

	// things to do:
	// - get the infra ref from the node pool
	// - look up the records
	// - build the instance types list
	//   - use status.capacity to inform resources

	return instanceTypes, nil
}

// Return nothing since there's no cloud provider drift.
func (c CloudProvider) IsDrifted(ctx context.Context, nodeClaim *v1beta1.NodeClaim) (cloudprovider.DriftReason, error) {
	return "", nil
}

func (c CloudProvider) Name() string {
	return "clusterapi"
}

func (c CloudProvider) machineToNodeClaim(ctx context.Context, machine *capiv1beta1.Machine) (*v1beta1.NodeClaim, error) {
	nodeClaim := v1beta1.NodeClaim{}
	if machine.Spec.ProviderID != nil {
		nodeClaim.Status.ProviderID = *machine.Spec.ProviderID
	}

	// we want to get the MachineDeployment that owns this Machine to read the capacity information.
	// to being this process, we get the MachineDeployment name from the Machine labels.
	mdName, found := machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel]
	if !found {
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, Machine has no MachineDeployment label %q", machine.GetName(), capiv1beta1.MachineDeploymentNameLabel)
	}
	machineDeployment, err := c.machineDeploymentProvider.Get(ctx, mdName, machine.GetNamespace())
	if err != nil {
		return nil, err
	}

	// machine capacity
	// we are using the scale from zero annotations on the MachineDeployment to make this accessible.
	// TODO (elmiko) improve this once upstream has advanced the state of the art, also add a mechanism
	// to lookup the infra machine template similar to how CAS does it.
	capacity := corev1.ResourceList{}
	cpu, found := machineDeployment.GetAnnotations()[cpuKey]
	if !found {
		// if there is no cpu resource we aren't going to get far, return an error
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no cpu capacity found on MachineDeployment %q", machine.GetName(), mdName)
	}
	capacity[corev1.ResourceCPU] = resource.MustParse(cpu)

	memory, found := machineDeployment.GetAnnotations()[memoryKey]
	if !found {
		// if there is no memory resource we aren't going to get far, return an error
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no memory capacity found on MachineDeployment %q", machine.GetName(), mdName)
	}
	capacity[corev1.ResourceMemory] = resource.MustParse(memory)

	// TODO (elmiko) add gpu, maxPods, labels, and taints

	nodeClaim.Status.Capacity = capacity

	return &nodeClaim, nil
}

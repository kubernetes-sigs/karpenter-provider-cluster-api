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

package cloudprovider

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/batcher"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
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
	mdLock := batcher.NewMDLockManager()
	return &CloudProvider{
		kubeClient:                kubeClient,
		machineProvider:           machineProvider,
		machineDeploymentProvider: machineDeploymentProvider,
		createBatcher:             batcher.NewCreateBatcher(ctx, kubeClient, machineProvider, machineDeploymentProvider, mdLock),
		deleteBatcher:             batcher.NewDeleteBatcher(ctx, machineProvider, machineDeploymentProvider, mdLock),
	}
}

type ClusterAPIInstanceType struct {
	cloudprovider.InstanceType

	MachineDeploymentName      string
	MachineDeploymentNamespace string
}

type CloudProvider struct {
	kubeClient                client.Client
	machineProvider           machine.Provider
	machineDeploymentProvider machinedeployment.Provider
	createBatcher             *batcher.CreateBatcher
	deleteBatcher             *batcher.DeleteBatcher
}

func (c *CloudProvider) Create(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*karpv1.NodeClaim, error) {
	if nodeClaim == nil {
		return nil, fmt.Errorf("cannot satisfy create, NodeClaim is nil")
	}

	// If the NodeClaim already has a Machine annotation, just
	// fetch the existing Machine and return it.
	if machineAnno, ok := nodeClaim.Annotations[providers.MachineAnnotation]; ok {
		return c.getExistingMachine(ctx, machineAnno)
	}

	instanceType, err := c.resolveInstanceType(ctx, nodeClaim)
	if err != nil {
		return nil, err
	}

	result := c.createBatcher.Add(ctx, &batcher.CreateInput{
		NodeClaimName:         nodeClaim.Name,
		MachineDeploymentName: instanceType.MachineDeploymentName,
		MachineDeploymentNS:   instanceType.MachineDeploymentNamespace,
	})
	if result.Err != nil {
		return nil, fmt.Errorf("launching nodeclaim: %w", result.Err)
	}

	machine := result.Output.Machine
	if machine.Spec.ProviderID == nil {
		return nil, fmt.Errorf("cannot satisfy create, waiting for Machine %q to have ProviderID", machine.Name)
	}

	//  fill out nodeclaim with details
	createdNodeClaim := createNodeClaimFromMachineDeployment(result.Output.MachineDeployment)
	createdNodeClaim.Status.ProviderID = *machine.Spec.ProviderID

	return createdNodeClaim, nil
}

func (c *CloudProvider) Delete(ctx context.Context, nodeClaim *karpv1.NodeClaim) error {
	machine, err := c.findMachineForNodeClaim(ctx, nodeClaim)
	if err != nil {
		return err
	}

	if machine == nil {
		return cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("unable to find Machine with provider ID %q to Delete NodeClaim %q", nodeClaim.Status.ProviderID, nodeClaim.Name))
	}

	// check if already deleting
	if c.machineProvider.IsDeleting(machine) {
		// Machine is already deleting, we do not need to annotate it or change the scalable resource replicas.
		return nil
	}

	// check if reducing replicas goes below zero
	machineDeployment, err := c.machineDeploymentFromMachine(ctx, machine)
	if err != nil {
		return fmt.Errorf("unable to delete NodeClaim %q, cannot find an owner MachineDeployment for Machine %q: %w", nodeClaim.Name, machine.Name, err)
	}

	if machineDeployment.Spec.Replicas == nil {
		return fmt.Errorf("unable to delete NodeClaim %q, MachineDeployment %q has nil replicas", nodeClaim.Name, machineDeployment.Name)
	}

	if *machineDeployment.Spec.Replicas == 0 {
		return fmt.Errorf("unable to delete NodeClaim %q, MachineDeployment %q is already at zero replicas", nodeClaim.Name, machineDeployment.Name)
	}

	result := c.deleteBatcher.Add(ctx, &batcher.DeleteInput{
		MachineName:           machine.Name,
		MachineNamespace:      machine.Namespace,
		MachineDeploymentName: machineDeployment.Name,
		MachineDeploymentNS:   machineDeployment.Namespace,
	})
	return result.Err
}

// Get returns a NodeClaim for the Machine object with the supplied provider ID, or nil if not found.
func (c *CloudProvider) Get(ctx context.Context, providerID string) (*karpv1.NodeClaim, error) {
	if len(providerID) == 0 {
		return nil, fmt.Errorf("no providerID supplied to Get, cannot continue")
	}

	machine, err := c.machineProvider.GetByProviderID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("error getting Machine: %w", err)
	}
	if machine == nil {
		return nil, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("cannot find Machine with provider ID %q", providerID))
	}

	nodeClaim, err := c.machineToNodeClaim(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert Machine to NodeClaim in CloudProvider.Get: %w", err)
	}

	return nodeClaim, nil
}

// GetInstanceTypes enumerates the known Cluster API scalable resources to generate the list
// of possible instance types.
func (c *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *karpv1.NodePool) ([]*cloudprovider.InstanceType, error) {

	if nodePool == nil {
		return nil, fmt.Errorf("node pool reference is nil, no way to proceed")
	}

	nodeClass, err := c.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve NodeClass from NodePool %s: %w", nodePool.Name, err)
	}

	capiInstanceTypes, err := c.findInstanceTypesForNodeClass(ctx, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("unable to get instance types for NodePool %q: %w", nodePool.Name, err)
	}

	instanceTypes := lo.Map(capiInstanceTypes, func(i *ClusterAPIInstanceType, _ int) *cloudprovider.InstanceType {
		return &cloudprovider.InstanceType{
			Name:         i.Name,
			Requirements: i.Requirements,
			Offerings:    i.Offerings,
			Capacity:     i.Capacity,
			Overhead:     i.Overhead,
		}
	})
	return instanceTypes, nil
}

func (c *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{&v1alpha1.ClusterAPINodeClass{}}
}

// Return nothing since there's no cloud provider drift.
func (c *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim) (cloudprovider.DriftReason, error) {
	return "", nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*karpv1.NodeClaim, error) {
	// select all machines that have the nodepool membership label, this should be all the machines that are registered as nodes
	selector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      providers.NodePoolMemberLabel,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	machines, err := c.machineProvider.List(ctx, "", &selector)
	if err != nil {
		return nil, fmt.Errorf("listing machines, %w", err)
	}

	var nodeClaims []*karpv1.NodeClaim
	for _, machine := range machines {
		nodeClaim, err := c.machineToNodeClaim(ctx, machine)
		if err != nil {
			return []*karpv1.NodeClaim{}, fmt.Errorf("unable to convert Machine %s to NodeClaim: %w", machine.Name, err)
		}
		nodeClaims = append(nodeClaims, nodeClaim)
	}

	return nodeClaims, nil
}

func (c *CloudProvider) Name() string {
	return "clusterapi"
}

func (c *CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	// TODO(elmiko) research what this means for cluster-api, perhaps there are conditions that
	// we could use from cluster-api to determine when repair should be initiated.

	return []cloudprovider.RepairPolicy{}
}

// getExistingMachine handles the resume path when a NodeClaim already has a
// Machine annotation from a previous Create attempt.
func (c *CloudProvider) getExistingMachine(ctx context.Context, machineAnno string) (*karpv1.NodeClaim, error) {
	machineNamespace, machineName, err := providers.ParseMachineAnnotation(machineAnno)
	if err != nil {
		return nil, fmt.Errorf("error parsing machine annotation: %w", err)
	}

	m, err := c.machineProvider.Get(ctx, machineName, machineNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get NodeClaim's Machine %s: %w", machineName, err)
	}

	md, err := c.machineDeploymentFromMachine(ctx, m)
	if err != nil {
		return nil, fmt.Errorf("failed to get NodeClaim's MachineDeployment %s: %w", machineName, err)
	}

	if m.Spec.ProviderID == nil {
		return nil, fmt.Errorf("cannot satisfy create, waiting for Machine %q to have ProviderID", m.Name)
	}

	nc := createNodeClaimFromMachineDeployment(md)
	nc.Status.ProviderID = *m.Spec.ProviderID
	return nc, nil
}

// resolveInstanceType finds the best matching instance type for a NodeClaim.
func (c *CloudProvider) resolveInstanceType(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*ClusterAPIInstanceType, error) {
	nodeClass, err := c.resolveNodeClassFromNodeClaim(ctx, nodeClaim)
	if err != nil {
		return nil, fmt.Errorf("cannot satisfy create, unable to resolve NodeClass from NodeClaim %q: %w", nodeClaim.Name, err)
	}

	instanceTypes, err := c.findInstanceTypesForNodeClass(ctx, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("cannot satisfy create, unable to get instance types for NodeClass %q of NodeClaim %q: %w", nodeClass.Name, nodeClaim.Name, err)
	}

	// identify which fit requirements
	compatibleInstanceTypes := filterCompatibleInstanceTypes(instanceTypes, nodeClaim)
	if len(compatibleInstanceTypes) == 0 {
		return nil, cloudprovider.NewInsufficientCapacityError(fmt.Errorf("cannot satisfy create, no compatible instance types found"))
	}

	// TODO (elmiko) if multiple instance types are found to be compatible we need to select one.
	// for now, we sort by resource name and take the first in the list. In the future, this should
	// be an option or something more useful like minimum size or cost.
	slices.SortFunc(compatibleInstanceTypes, func(a, b *ClusterAPIInstanceType) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return compatibleInstanceTypes[0], nil
}

// findMachineForNodeClaim resolves a CAPI Machine from a NodeClaim's providerID
// or Machine annotation.
func (c *CloudProvider) findMachineForNodeClaim(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*capiv1beta1.Machine, error) {
	if len(nodeClaim.Status.ProviderID) != 0 {
		m, err := c.machineProvider.GetByProviderID(ctx, nodeClaim.Status.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("error finding Machine with provider ID %q to Delete NodeClaim %q: %w", nodeClaim.Status.ProviderID, nodeClaim.Name, err)
		}
		return m, nil
	}

	if machineAnno, ok := nodeClaim.Annotations[providers.MachineAnnotation]; ok {
		machineNamespace, machineName, err := providers.ParseMachineAnnotation(machineAnno)
		if err != nil {
			return nil, fmt.Errorf("error parsing machine annotation: %w", err)
		}
		m, err := c.machineProvider.Get(ctx, machineName, machineNamespace)
		if err != nil {
			return nil, fmt.Errorf("error finding Machine %q in namespace %s to Delete NodeClaim %q: %w", machineName, machineNamespace, nodeClaim.Name, err)
		}
		return m, nil
	}

	return nil, fmt.Errorf("NodeClaim %q does not have a provider ID or Machine annotations, cannot delete", nodeClaim.Name)
}

func (c *CloudProvider) machineDeploymentFromMachine(ctx context.Context, machine *capiv1beta1.Machine) (*capiv1beta1.MachineDeployment, error) {
	mdName, found := machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel]
	if !found {
		return nil, fmt.Errorf("unable to find MachineDeployment for Machine %q, has no MachineDeployment label %q", machine.GetName(), capiv1beta1.MachineDeploymentNameLabel)
	}

	machineDeployment, err := c.machineDeploymentProvider.Get(ctx, mdName, machine.GetNamespace())
	if err != nil {
		return nil, fmt.Errorf("unable to get MachineDeployment %s for Machine %s: %w", mdName, machine.GetName(), err)
	}

	return machineDeployment, nil
}

func (c *CloudProvider) findInstanceTypesForNodeClass(ctx context.Context, nodeClass *v1alpha1.ClusterAPINodeClass) ([]*ClusterAPIInstanceType, error) {
	instanceTypes := []*ClusterAPIInstanceType{}

	if nodeClass == nil {
		return instanceTypes, fmt.Errorf("unable to find instance types for nil NodeClass")
	}

	machineDeployments, err := c.machineDeploymentProvider.List(ctx, nodeClass.Spec.ScalableResourceSelector)
	if err != nil {
		return instanceTypes, fmt.Errorf("unable to list MachineDeployments for NodeClass %s: %w", nodeClass.Name, err)
	}

	for _, md := range machineDeployments {
		it := machineDeploymentToInstanceType(md)
		instanceTypes = append(instanceTypes, it)
	}

	return instanceTypes, nil
}

func (c *CloudProvider) machineToNodeClaim(ctx context.Context, machine *capiv1beta1.Machine) (*karpv1.NodeClaim, error) {
	nodeClaim := karpv1.NodeClaim{}
	if machine.Spec.ProviderID != nil {
		nodeClaim.Status.ProviderID = *machine.Spec.ProviderID
	}

	// we want to get the MachineDeployment that owns this Machine to read the capacity information.
	// to being this process, we get the MachineDeployment name from the Machine labels.
	machineDeployment, err := c.machineDeploymentFromMachine(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, cannot find MachineDeployment: %w", machine.Name, err)
	}

	// machine capacity
	// we are using the scale from zero annotations on the MachineDeployment to make this accessible.
	// TODO (elmiko) improve this once upstream has advanced the state of the art for getting capacity,
	// also add a mechanism to lookup the infra machine template similar to how CAS does it.
	capacity := capacityResourceListFromAnnotations(machineDeployment.GetAnnotations())
	_, found := capacity[corev1.ResourceCPU]
	if !found {
		// if there is no cpu resource we aren't going to get far, return an error
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no cpu capacity found on MachineDeployment %q", machine.GetName(), machineDeployment.Name)
	}

	_, found = capacity[corev1.ResourceMemory]
	if !found {
		// if there is no memory resource we aren't going to get far, return an error
		return nil, fmt.Errorf("unable to convert Machine %q to a NodeClaim, no memory capacity found on MachineDeployment %q", machine.GetName(), machineDeployment.Name)
	}

	// Set NodeClaim labels from the MachineDeployment
	nodeClaim.Labels = nodeLabelsFromMachineDeployment(machineDeployment)

	// TODO (elmiko) add taints

	nodeClaim.Status.Capacity = capacity

	return &nodeClaim, nil
}

func (c *CloudProvider) resolveNodeClassFromNodeClaim(ctx context.Context, nodeClaim *karpv1.NodeClaim) (*v1alpha1.ClusterAPINodeClass, error) {
	nodeClass := &v1alpha1.ClusterAPINodeClass{}

	if nodeClaim == nil {
		return nil, fmt.Errorf("NodeClaim is nil, cannot resolve NodeClass")
	}

	if nodeClaim.Spec.NodeClassRef == nil {
		return nil, fmt.Errorf("NodeClass reference is nil for NodeClaim %q, cannot resolve NodeClass", nodeClaim.Name)
	}

	name := nodeClaim.Spec.NodeClassRef.Name
	if name == "" {
		return nil, fmt.Errorf("NodeClass reference name is empty for NodeClaim %q, cannot resolve NodeClass", nodeClaim.Name)
	}

	// TODO (elmiko) add extra logic to get different resources from the class ref
	// if the kind and version differ from the included api then we will need to load differently.
	// NodeClass and NodeClaim are not namespace scoped, this call can probably be changed.
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nodeClaim.Namespace}, nodeClass); err != nil {
		return nil, fmt.Errorf("unable to get NodeClass %s for NodeClaim %s: %w", name, nodeClaim.Name, err)
	}

	return nodeClass, nil
}

func (c *CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *karpv1.NodePool) (*v1alpha1.ClusterAPINodeClass, error) {
	nodeClass := &v1alpha1.ClusterAPINodeClass{}

	if nodePool == nil {
		return nil, fmt.Errorf("NodePool is nil, cannot resolve NodeClass")
	}

	if nodePool.Spec.Template.Spec.NodeClassRef == nil {
		return nil, fmt.Errorf("node class reference is nil, no way to proceed")
	}

	name := nodePool.Spec.Template.Spec.NodeClassRef.Name
	if name == "" {
		return nil, fmt.Errorf("node class reference name is empty, no way to proceed")
	}

	// TODO (elmiko) add extra logic to get different resources from the class ref
	// if the kind and version differ from the included api then we will need to load differently.
	if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: nodePool.Namespace}, nodeClass); err != nil {
		return nil, fmt.Errorf("unable to get NodeClass %s for NodePool %s: %w", name, nodePool.Name, err)
	}

	return nodeClass, nil
}

func capacityResourceListFromAnnotations(annotations map[string]string) corev1.ResourceList {
	capacity := corev1.ResourceList{}

	if annotations == nil {
		return capacity
	}

	cpu, found := annotations[cpuKey]
	if found {
		capacity[corev1.ResourceCPU] = resource.MustParse(cpu)
	}

	memory, found := annotations[memoryKey]
	if found {
		capacity[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	gpuCount, found := annotations[gpuCountKey]
	if found {
		// if there is a count there must also be a type
		gpuType, found := annotations[gpuTypeKey]
		if found {
			capacity[corev1.ResourceName(gpuType)] = resource.MustParse(gpuCount)
		}
	}

	ephStorage, found := annotations[diskCapacityKey]
	if found {
		capacity[corev1.ResourceEphemeralStorage] = resource.MustParse(ephStorage)
	}

	maxPods, found := annotations[maxPodsKey]
	if found {
		capacity[corev1.ResourcePods] = resource.MustParse(maxPods)
	}

	return capacity
}

func createNodeClaimFromMachineDeployment(machineDeployment *capiv1beta1.MachineDeployment) *karpv1.NodeClaim {
	nodeClaim := &karpv1.NodeClaim{}

	instanceType := machineDeploymentToInstanceType(machineDeployment)
	nodeClaim.Status.Capacity = instanceType.Capacity
	nodeClaim.Status.Allocatable = instanceType.Allocatable()

	// Set NodeClaim labels from the MachineDeployment
	nodeClaim.Labels = nodeLabelsFromMachineDeployment(machineDeployment)

	// TODO (elmiko) add taints

	return nodeClaim
}

func filterCompatibleInstanceTypes(instanceTypes []*ClusterAPIInstanceType, nodeClaim *karpv1.NodeClaim) []*ClusterAPIInstanceType {
	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...)
	filteredInstances := lo.Filter(instanceTypes, func(i *ClusterAPIInstanceType, _ int) bool {
		// TODO (elmiko) if/when we have offering availability, this is a good place to filter out unavailable instance types
		return reqs.Compatible(i.Requirements, scheduling.AllowUndefinedWellKnownLabels) == nil &&
			resources.Fits(nodeClaim.Spec.Resources.Requests, i.Allocatable()) &&
			i.Offerings.Available().HasCompatible(reqs)
	})

	return filteredInstances
}

func labelsFromScaleFromZeroAnnotation(annotation string) map[string]string {
	labels := map[string]string{}

	labelStrings := strings.Split(annotation, ",")
	for _, label := range labelStrings {
		split := strings.SplitN(label, "=", 2)
		if len(split) == 2 {
			labels[split[0]] = split[1]
		}
	}

	return labels
}

// isBelowMaxSize checks if the MachineDeployment's current replicas are below the max size
// defined by the Cluster Autoscaler annotation. If the annotation is not present or cannot
// be parsed, it returns true (no limit enforced).
func isBelowMaxSize(machineDeployment *capiv1beta1.MachineDeployment) bool {
	if machineDeployment == nil {
		return false
	}
	maxSizeStr, ok := machineDeployment.Annotations[capiv1beta1.AutoscalerMaxSizeAnnotation]
	if !ok {
		return true
	}
	maxSize, err := strconv.Atoi(maxSizeStr)
	if err != nil {
		return true
	}
	replicas := int(ptr.Deref(machineDeployment.Spec.Replicas, 0))
	return replicas < maxSize
}

func machineDeploymentToInstanceType(machineDeployment *capiv1beta1.MachineDeployment) *ClusterAPIInstanceType {
	instanceType := &ClusterAPIInstanceType{}

	labels := nodeLabelsFromMachineDeployment(machineDeployment)
	requirements := []*scheduling.Requirement{}
	for k, v := range labels {
		requirements = append(requirements, scheduling.NewRequirement(k, corev1.NodeSelectorOpIn, v))
	}
	instanceType.Requirements = scheduling.NewRequirements(requirements...)

	capacity := capacityResourceListFromAnnotations(machineDeployment.GetAnnotations())
	instanceType.Capacity = capacity

	// TODO (elmiko) add offerings info, TBD of where this would come from
	// start with zone, read from the label and add to offering
	// initial price is 0
	// there is a single offering, and it is available
	zone := zoneLabelFromLabels(labels)
	requirements = []*scheduling.Requirement{
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
		scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, karpv1.CapacityTypeOnDemand),
	}
	offerings := cloudprovider.Offerings{
		&cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(requirements...),
			Price:        0.0,
			Available:    isBelowMaxSize(machineDeployment),
		},
	}

	instanceType.Offerings = offerings

	// TODO (elmiko) find a better way to learn the instance type. The instance name needs to be the same
	// as the well-known `node.kubernetes.io/instance-type` that will be on resulting nodes. Usually, a
	// cloud controller, or similar, mechanism is used to apply this label.
	// For now, we check the labels we know about from the MachineDeployment, if the instance type label
	// is not there, leave it blank.
	// to the v1.LabelInstanceTypeStable. if karpenter expects this to match the node, then we need to get this value through capi.
	// TODO (jkyros) Add a test case to test this behavior
	instanceType.Name = labels[corev1.LabelInstanceTypeStable]

	// TODO (elmiko) add the proper overhead information, not sure where we will harvest this information.
	// perhaps it needs to be a configurable option somewhere.
	instanceType.Overhead = &cloudprovider.InstanceTypeOverhead{}

	// record the information from the MachineDeployment so we can find it again later.
	instanceType.MachineDeploymentName = machineDeployment.Name
	instanceType.MachineDeploymentNamespace = machineDeployment.Namespace

	return instanceType
}

func managedNodeLabelsFromLabels(labels map[string]string) map[string]string {
	managedLabels := map[string]string{}
	for key, value := range labels {
		dnsSubdomainOrName := strings.Split(key, "/")[0]
		if dnsSubdomainOrName == capiv1beta1.NodeRoleLabelPrefix {
			managedLabels[key] = value
		}
		if dnsSubdomainOrName == capiv1beta1.NodeRestrictionLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+capiv1beta1.NodeRestrictionLabelDomain) {
			managedLabels[key] = value
		}
		if dnsSubdomainOrName == capiv1beta1.ManagedNodeLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+capiv1beta1.ManagedNodeLabelDomain) {
			managedLabels[key] = value
		}
	}

	return managedLabels
}

func nodeLabelsFromMachineDeployment(machineDeployment *capiv1beta1.MachineDeployment) map[string]string {
	labels := map[string]string{}

	if machineDeployment.Spec.Template.Labels != nil {
		// get the labels that will be propagated to the node from the machinedeployment
		// see https://cluster-api.sigs.k8s.io/developer/architecture/controllers/metadata-propagation#metadata-propagation
		labels = managedNodeLabelsFromLabels(machineDeployment.Spec.Template.Labels)
	}

	// next we integrate labels from the scale-from-zero annotations, these can override
	// the propagated labels.
	// see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210310-opt-in-autoscaling-from-zero.md#machineset-and-machinedeployment-annotations
	if annotation, found := machineDeployment.GetAnnotations()[labelsKey]; found {
		for k, v := range labelsFromScaleFromZeroAnnotation(annotation) {
			labels[k] = v
		}
	}

	return labels
}

// zoneLabelFromLabels returns the value of the kubernetes well-known zone label or an empty string
func zoneLabelFromLabels(labels map[string]string) string {
	zone := ""

	if value, found := labels[corev1.LabelTopologyZone]; found {
		zone = value
	}

	return zone
}

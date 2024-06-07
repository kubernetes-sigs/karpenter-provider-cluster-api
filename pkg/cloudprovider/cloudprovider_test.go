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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	"sigs.k8s.io/karpenter/pkg/apis/v1beta1"
)

func eventuallyDeleteAllOf(cl client.Client, obj client.Object, ls client.ObjectList) {
	Expect(cl.DeleteAllOf(context.Background(), obj, client.InNamespace(testNamespace))).To(Succeed())
	Eventually(func() client.ObjectList {
		Expect(cl.List(context.Background(), ls, client.InNamespace(testNamespace))).To(Succeed())
		return ls
	}).Should(HaveField("Items", HaveLen(0)))
}

var _ = Describe("CloudProvider.GetInstanceTypes method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		machineProvider := machine.NewDefaultProvider(context.Background(), cl)
		machineDeploymentProvider := machinedeployment.NewDefaultProvider(context.Background(), cl)
		provider = NewCloudProvider(context.Background(), cl, machineProvider, machineDeploymentProvider)
	})

	AfterEach(func() {
		eventuallyDeleteAllOf(cl, &v1beta1.NodePool{}, &v1beta1.NodePoolList{})
		eventuallyDeleteAllOf(cl, &capiv1beta1.MachineDeployment{}, &capiv1beta1.MachineDeploymentList{})
		eventuallyDeleteAllOf(cl, &api.ClusterAPINodeClass{}, &api.ClusterAPINodeClassList{})
	})

	It("returns an error when NodePool is not supplied", func() {
		instanceTypes, err := provider.GetInstanceTypes(context.Background(), nil)
		Expect(err).To(MatchError(fmt.Errorf("node pool reference is nil, no way to proceed")))
		Expect(instanceTypes).To(HaveLen(0))
	})

	It("returns an error when the NodeClass reference is not found", func() {
		nodePool := v1beta1.NodePool{}
		nodePool.Spec.Template.Spec.NodeClassRef = nil

		instanceTypes, err := provider.GetInstanceTypes(context.Background(), &nodePool)
		Expect(err).To(MatchError(fmt.Errorf("node class reference is nil, no way to proceed")))
		Expect(instanceTypes).To(HaveLen(0))
	})

	It("returns the expected number of instance types when mixed MachineDeployments are available", func() {
		nodeClass := &api.ClusterAPINodeClass{}
		nodeClass.Name = "default"
		Expect(cl.Create(context.Background(), nodeClass)).To(Succeed())

		nodePool := v1beta1.NodePool{}
		nodePool.Spec.Template.Spec.NodeClassRef = &v1beta1.NodeClassReference{
			Name: nodeClass.Name,
		}

		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			cpuKey:    "4",
			memoryKey: "16777220Ki",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment = newMachineDeployment("md-2", "other-cluster", false)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		instanceTypes, err := provider.GetInstanceTypes(context.Background(), &nodePool)

		Expect(err).ToNot(HaveOccurred())
		Expect(instanceTypes).To(HaveLen(1))
	})
})

var _ = Describe("CloudProvider.machineDeploymentToInstanceType method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		provider = NewCloudProvider(context.Background(), cl, nil, nil)
	})

	It("adds capacity resources from scale from zero annotations", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Annotations = map[string]string{
			cpuKey:      "1",
			memoryKey:   "16Gi",
			gpuCountKey: "1",
			gpuTypeKey:  "nvidia.com/gpu",
		}

		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Capacity).Should(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("1")))
		Expect(instanceType.Capacity).Should(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("16Gi")))
		Expect(instanceType.Capacity).Should(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("1")))
	})

	It("adds nothing to requirements when no managed labels or scale from zero annotations are present", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Spec.Template.Labels = map[string]string{}
		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)

		Expect(instanceType.Requirements).To(HaveLen(0))
	})

	It("adds labels to the requirements from the Cluster API propagation rules", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Spec.Template.Labels = map[string]string{
			providers.NodePoolMemberLabel:                              "1",
			"node-restriction.kubernetes.io/some-thing":                "2",
			"prefixed.node-restriction.kubernetes.io/some-other-thing": "3",
			"node.cluster.x-k8s.io/another-thing":                      "4",
			"prefixed.node.cluster.x-k8s.io/another-thing":             "5",
			// the following should NOT propagate
			"my.special.label/should-not-propagate":         "bar",
			"prefixed.node-role.kubernetes.io/no-propagate": "special-role",
		}

		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Requirements).To(HaveLen(5))
		Expect(instanceType.Requirements).Should(HaveKey(providers.NodePoolMemberLabel))
		Expect(instanceType.Requirements).Should(HaveKey("node-restriction.kubernetes.io/some-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("prefixed.node-restriction.kubernetes.io/some-other-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("node.cluster.x-k8s.io/another-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("prefixed.node.cluster.x-k8s.io/another-thing"))
	})

	It("adds labels to the requirements from the scale from zero annotations", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Annotations = map[string]string{
			labelsKey:          fmt.Sprintf("%s=east,%s=big", corev1.LabelTopologyZone, InstanceSizeLabelKey),
			"some-other-label": "stuff!",
		}

		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Requirements).To(HaveLen(2))
		Expect(instanceType.Requirements).Should(HaveKey(corev1.LabelTopologyZone))
		Expect(instanceType.Requirements).Should(HaveKey(InstanceSizeLabelKey))
	})

	It("adds labels to the requirements from the propagation rules and the scale from zero annotations", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Annotations = map[string]string{
			labelsKey:          fmt.Sprintf("%s=east,%s=big", corev1.LabelTopologyZone, InstanceSizeLabelKey),
			"some-other-label": "stuff!",
		}
		machineDeployment.Spec.Template.Labels = map[string]string{
			providers.NodePoolMemberLabel:                              "1",
			"node-restriction.kubernetes.io/some-thing":                "2",
			"prefixed.node-restriction.kubernetes.io/some-other-thing": "3",
			"node.cluster.x-k8s.io/another-thing":                      "4",
			"prefixed.node.cluster.x-k8s.io/another-thing":             "5",
			// the following should NOT propagate
			"my.special.label/should-not-propagate":         "bar",
			"prefixed.node-role.kubernetes.io/no-propagate": "special-role",
		}

		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Requirements).To(HaveLen(7))
		Expect(instanceType.Requirements).Should(HaveKey(corev1.LabelTopologyZone))
		Expect(instanceType.Requirements).Should(HaveKey(InstanceSizeLabelKey))
		Expect(instanceType.Requirements).Should(HaveKey(providers.NodePoolMemberLabel))
		Expect(instanceType.Requirements).Should(HaveKey("node-restriction.kubernetes.io/some-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("prefixed.node-restriction.kubernetes.io/some-other-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("node.cluster.x-k8s.io/another-thing"))
		Expect(instanceType.Requirements).Should(HaveKey("prefixed.node.cluster.x-k8s.io/another-thing"))
	})

	It("adds a single available on-demand offering with price 0 and empty zone", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Offerings).To(HaveLen(1))
		Expect(instanceType.Offerings[0]).To(HaveField("CapacityType", v1beta1.CapacityTypeOnDemand))
		Expect(instanceType.Offerings[0]).To(HaveField("Zone", ""))
		Expect(instanceType.Offerings[0]).To(HaveField("Price", 0.0))
		Expect(instanceType.Offerings[0]).To(HaveField("Available", true))
	})

	It("adds the correct zone to offering when the well known zone label is present", func() {
		zone := "east"
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		machineDeployment.Annotations = map[string]string{
			// we need to add the zone label to the scale from zero annotations due to the capi metadata propagation rules
			labelsKey: fmt.Sprintf("%s=%s", corev1.LabelTopologyZone, zone),
		}
		instanceType := provider.machineDeploymentToInstanceType(machineDeployment)
		Expect(instanceType.Offerings).To(HaveLen(1))
		Expect(instanceType.Offerings[0]).To(HaveField("Zone", zone))
	})
})

var _ = Describe("CloudProvider.resolveNodeClassFromNodePool method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		machineProvider := machine.NewDefaultProvider(context.Background(), cl)
		machineDeploymentProvider := machinedeployment.NewDefaultProvider(context.Background(), cl)
		provider = NewCloudProvider(context.Background(), cl, machineProvider, machineDeploymentProvider)
	})

	AfterEach(func() {
		eventuallyDeleteAllOf(cl, &api.ClusterAPINodeClass{}, &api.ClusterAPINodeClassList{})
	})

	It("returns an error when no NodeClass reference is found", func() {
		nodePool := v1beta1.NodePool{}
		nodePool.Spec.Template.Spec.NodeClassRef = nil

		nodeClass, err := provider.resolveNodeClassFromNodePool(context.Background(), &nodePool)
		Expect(err).To(MatchError(fmt.Errorf("node class reference is nil, no way to proceed")))
		Expect(nodeClass).To(BeNil())
	})

	It("returns an error when NodeClass name reference is empty", func() {
		nodePool := v1beta1.NodePool{}
		nodePool.Spec.Template.Spec.NodeClassRef = &v1beta1.NodeClassReference{
			Name: "",
		}

		nodeClass, err := provider.resolveNodeClassFromNodePool(context.Background(), &nodePool)
		Expect(err).To(MatchError(fmt.Errorf("node class reference name is empty, no way to proceed")))
		Expect(nodeClass).To(BeNil())
	})

	It("returns a NodeClass when present", func() {
		nodeClass := &api.ClusterAPINodeClass{}
		nodeClass.Name = "default"
		Expect(cl.Create(context.Background(), nodeClass)).To(Succeed())

		nodePool := v1beta1.NodePool{}
		nodePool.Spec.Template.Spec.NodeClassRef = &v1beta1.NodeClassReference{
			Name: nodeClass.Name,
		}

		nodeClass, err := provider.resolveNodeClassFromNodePool(context.Background(), &nodePool)
		Expect(err).ToNot(HaveOccurred())
		Expect(nodeClass).ToNot(BeNil())
	})
})

var _ = Describe("CloudProvider.List method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		machineProvider := machine.NewDefaultProvider(context.Background(), cl)
		machineDeploymentProvider := machinedeployment.NewDefaultProvider(context.Background(), cl)
		provider = NewCloudProvider(context.Background(), cl, machineProvider, machineDeploymentProvider)
	})

	AfterEach(func() {
		eventuallyDeleteAllOf(cl, &capiv1beta1.Machine{}, &capiv1beta1.MachineList{})
		eventuallyDeleteAllOf(cl, &capiv1beta1.MachineDeployment{}, &capiv1beta1.MachineDeploymentList{})
	})

	It("returns an empty list when no Machines are present", func() {
		nodeClaims, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(nodeClaims).To(HaveLen(0))
	})

	It("returns the correct size list when only participating Machines are present", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			cpuKey:    "4",
			memoryKey: "16777220Ki",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = machineDeployment.Name
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		nodeClaims, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(nodeClaims).To(HaveLen(1))
	})

	It("returns the correct size list when participating and non-participating Machines are present", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			cpuKey:    "4",
			memoryKey: "16777220Ki",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = machineDeployment.Name
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machine = newMachine("m-2", "test-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		nodeClaims, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(nodeClaims).To(HaveLen(1))
	})
})

var _ = Describe("CloudProvider.machineToNodeClaim method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		machineProvider := machine.NewDefaultProvider(context.Background(), cl)
		machineDeploymentProvider := machinedeployment.NewDefaultProvider(context.Background(), cl)
		provider = NewCloudProvider(context.Background(), cl, machineProvider, machineDeploymentProvider)
	})

	AfterEach(func() {
		eventuallyDeleteAllOf(cl, &capiv1beta1.Machine{}, &capiv1beta1.MachineList{})
		eventuallyDeleteAllOf(cl, &capiv1beta1.MachineDeployment{}, &capiv1beta1.MachineDeploymentList{})
	})

	It("returns the proper capacity information in the NodeClaim", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			cpuKey:    "4",
			memoryKey: "16777220Ki",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = machineDeployment.Name
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		nodeClaim, err := provider.machineToNodeClaim(context.Background(), machine)
		Expect(err).ToNot(HaveOccurred())

		cpu := resource.MustParse("4")
		Expect(nodeClaim.Status.Capacity).Should(HaveKeyWithValue(corev1.ResourceCPU, cpu))
		memory := resource.MustParse("16777220Ki")
		Expect(nodeClaim.Status.Capacity).Should(HaveKeyWithValue(corev1.ResourceMemory, memory))
	})

	It("returns an error when the cpu annotation is not on the MachineDeployment", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			memoryKey: "16777220Ki",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = machineDeployment.Name
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		nodeClaim, err := provider.machineToNodeClaim(context.Background(), machine)
		Expect(nodeClaim).To(BeNil())
		Expect(err).To(MatchError(fmt.Errorf("unable to convert Machine %q to a NodeClaim, no cpu capacity found on MachineDeployment %q", machine.Name, machineDeployment.Name)))
	})

	It("returns an error when the memory annotation is not on the MachineDeployment", func() {
		machineDeployment := newMachineDeployment("md-1", "test-cluster", true)
		annotations := map[string]string{
			cpuKey: "4",
		}
		machineDeployment.SetAnnotations(annotations)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = machineDeployment.Name
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		nodeClaim, err := provider.machineToNodeClaim(context.Background(), machine)
		Expect(nodeClaim).To(BeNil())
		Expect(err).To(MatchError(fmt.Errorf("unable to convert Machine %q to a NodeClaim, no memory capacity found on MachineDeployment %q", machine.Name, machineDeployment.Name)))
	})

	It("returns an error when the MachineDeployment label is not present", func() {
		machine := newMachine("m-1", "test-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())
		nodeClaim, err := provider.machineToNodeClaim(context.Background(), machine)
		Expect(nodeClaim).To(BeNil())
		Expect(err).To(MatchError(fmt.Errorf("unable to convert Machine %q to a NodeClaim, Machine has no MachineDeployment label %q", "m-1", capiv1beta1.MachineDeploymentNameLabel)))
	})

	It("returns a not found error when the MachineDeployment is not found", func() {
		machine := newMachine("m-1", "test-cluster", true)
		machine.GetLabels()[capiv1beta1.MachineDeploymentNameLabel] = "md-1"
		Expect(cl.Create(context.Background(), machine)).To(Succeed())
		nodeClaim, err := provider.machineToNodeClaim(context.Background(), machine)
		Expect(nodeClaim).To(BeNil())
		Expect(err).To(MatchError(errors.IsNotFound, "IsNotFound"))
	})
})

func newMachine(machineName string, clusterName string, karpenterMember bool) *capiv1beta1.Machine {
	machine := &capiv1beta1.Machine{}
	machine.SetName(machineName)
	machine.SetNamespace(testNamespace)
	if karpenterMember {
		labels := map[string]string{}
		labels[providers.NodePoolMemberLabel] = ""
		machine.SetLabels(labels)
	}
	machine.Spec.ClusterName = clusterName
	return machine
}

func newMachineDeployment(name string, clusterName string, karpenterMember bool) *capiv1beta1.MachineDeployment {
	machineDeployment := &capiv1beta1.MachineDeployment{}
	machineDeployment.SetName(name)
	machineDeployment.SetNamespace(testNamespace)
	if karpenterMember {
		labels := map[string]string{}
		labels[providers.NodePoolMemberLabel] = ""
		machineDeployment.SetLabels(labels)
	}
	machineDeployment.Spec.ClusterName = clusterName
	machineDeployment.Spec.Template.Spec.ClusterName = clusterName
	return machineDeployment
}

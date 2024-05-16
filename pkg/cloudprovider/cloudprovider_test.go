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
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
)

var _ = Describe("CloudProvider machineToNodeClaim method", func() {
	var provider *CloudProvider

	BeforeEach(func() {
		machineProvider := machine.NewDefaultProvider(context.Background(), cl)
		machineDeploymentProvider := machinedeployment.NewDefaultProvider(context.Background(), cl)
		provider = NewCloudProvider(context.Background(), cl, machineProvider, machineDeploymentProvider)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.MachineDeployment{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineDeploymentList := &capiv1beta1.MachineDeploymentList{}
			Expect(cl.List(context.Background(), machineDeploymentList, client.InNamespace(testNamespace))).To(Succeed())
			return machineDeploymentList
		}).Should(HaveField("Items", HaveLen(0)))
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

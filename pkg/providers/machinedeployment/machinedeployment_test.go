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

package machinedeployment

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

var _ = Describe("MachineDeployment DefaultProvider.Get method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.MachineDeployment{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineDeploymentList := &capiv1beta1.MachineDeploymentList{}
			Expect(cl.List(context.Background(), machineDeploymentList, client.InNamespace(testNamespace))).To(Succeed())
			return machineDeploymentList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns the named MachineDeployment when it exists", func() {
		name := "test-machine-deployment"
		machineDeployment := newMachineDeployment(name, "workload-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment, err := provider.Get(context.Background(), name, testNamespace)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployment).ToNot(BeNil())
	})

	It("returns nil and an error when the MachineDeployment does not exist", func() {
		name := "test-machine-deployment"
		machineDeployment, err := provider.Get(context.Background(), name, testNamespace)
		Expect(err).To(HaveOccurred())
		Expect(machineDeployment).To(BeNil())
	})
})

var _ = Describe("MachineDeployment DefaultProvider.List method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.MachineDeployment{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineDeploymentList := &capiv1beta1.MachineDeploymentList{}
			Expect(cl.List(context.Background(), machineDeploymentList, client.InNamespace(testNamespace))).To(Succeed())
			return machineDeploymentList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an empty list when no MachineDeployments are present in API", func() {
		machineDeployments, err := provider.List(context.Background(), nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(0))
	})

	It("returns a list of correct length when there are only karpenter member MachineDeployments", func() {
		machineDeployment := newMachineDeployment("md-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployments, err := provider.List(context.Background(), nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(1))
	})

	It("returns a list of correct length when there are mixed member MachineDeployments", func() {
		machineDeployment := newMachineDeployment("md-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment = newMachineDeployment("md-2", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployments, err := provider.List(context.Background(), nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(1))
	})

	It("returns an empty list when no member MachineDeployments are present", func() {
		machineDeployment := newMachineDeployment("md-1", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployments, err := provider.List(context.Background(), nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(0))
	})

	It("returns a list of correct length when there are mixed member MachineDeployments with a selector", func() {
		machineDeployment := newMachineDeployment("md-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		selectorLabel := "label-for-selection"
		machineDeployment = newMachineDeployment("md-2", "karpenter-cluster", true)
		machineDeployment.GetLabels()[selectorLabel] = ""
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment = newMachineDeployment("md-3", "karpenter-cluster", false)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		selector := &metav1.LabelSelector{
			MatchLabels: map[string]string{
				selectorLabel: "",
			},
		}
		machineDeployments, err := provider.List(context.Background(), selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(1))
	})

	It("returns an empty list when there are no member MachineDeployments with a selector", func() {
		machineDeployment := newMachineDeployment("md-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment = newMachineDeployment("md-2", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment = newMachineDeployment("md-3", "karpenter-cluster", false)
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		selectorLabel := "label-for-selection"
		selector := &metav1.LabelSelector{
			MatchLabels: map[string]string{
				selectorLabel: "",
			},
		}
		machineDeployments, err := provider.List(context.Background(), selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(machineDeployments).To(HaveLen(0))
	})
})

var _ = Describe("MachineDeployment DefaultProvider.Update method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.MachineDeployment{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineDeploymentList := &capiv1beta1.MachineDeploymentList{}
			Expect(cl.List(context.Background(), machineDeploymentList, client.InNamespace(testNamespace))).To(Succeed())
			return machineDeploymentList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an error when the MachineDeployment does not exist", func() {
		machineDeployment := newMachineDeployment("non-existant", "fake-cluster", true)
		err := provider.Update(context.Background(), machineDeployment)
		Expect(err).Should(MatchError(ContainSubstring(fmt.Sprintf("unable to update MachineDeployment %q", machineDeployment.Name))))
	})

	It("updates the MachineDeployment as expected", func() {
		machineDeployment := newMachineDeployment("md-1", "karpenter-cluster", true)
		machineDeployment.Spec.Replicas = ptr.To(int32(0))
		Expect(cl.Create(context.Background(), machineDeployment)).To(Succeed())

		machineDeployment, err := provider.Get(context.Background(), machineDeployment.Name, machineDeployment.Namespace)
		expectedReplicas := *machineDeployment.Spec.Replicas + 1
		machineDeployment.Spec.Replicas = ptr.To(expectedReplicas)

		err = provider.Update(context.Background(), machineDeployment)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() *capiv1beta1.MachineDeployment {
			md, err := provider.Get(context.Background(), machineDeployment.Name, machineDeployment.Namespace)
			Expect(err).ToNot(HaveOccurred())
			return md
		}).Should(HaveField("Spec", HaveField("Replicas", ptr.To(expectedReplicas))))
	})
})

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

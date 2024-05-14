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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

var _ = Describe("DefaultProvider List method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.Background(), cl)
	})

	AfterEach(func() {
		Expect(cl.DeleteAllOf(context.Background(), &capiv1beta1.Machine{}, client.InNamespace(testNamespace))).To(Succeed())
		Eventually(func() client.ObjectList {
			machineList := &capiv1beta1.MachineList{}
			Expect(cl.List(context.Background(), machineList, client.InNamespace(testNamespace))).To(Succeed())
			return machineList
		}).Should(HaveField("Items", HaveLen(0)))
	})

	It("returns an empty list when no Machines are present in API", func() {
		machines, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(0))
	})

	It("returns a list of correct length when there are only karpenter member machines", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
	})

	It("returns a list of correct length when there are mixed member machines", func() {
		machine := newMachine("karpenter-1", "karpenter-cluster", true)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machine = newMachine("clusterapi-1", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
	})

	It("returns an empty list when no member machines are present", func() {
		machine := newMachine("clusterapi-1", "workload-cluster", false)
		Expect(cl.Create(context.Background(), machine)).To(Succeed())

		machines, err := provider.List(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(0))
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

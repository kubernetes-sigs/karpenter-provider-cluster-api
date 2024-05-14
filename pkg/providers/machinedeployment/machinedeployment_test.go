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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

var _ = Describe("MachineDeployment DefaultProvider Get method", func() {
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

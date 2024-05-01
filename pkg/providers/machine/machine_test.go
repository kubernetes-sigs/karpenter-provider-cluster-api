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
)

var _ = Describe("DefaultProvider List method", func() {
	var provider Provider

	BeforeEach(func() {
		provider = NewDefaultProvider(context.TODO(), cl)
	})

	It("returns an empty list when no Machines are present in API", func() {
		machines, err := provider.List(context.TODO())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(0))
	})

	It("returns a list of correct length when there are machines with the proper annotation", func() {
		machine := &capiv1beta1.Machine{}
		machine.SetName("karpenter-managed-1")
		machine.SetNamespace(testNamespace)
		labels := machine.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[nodePoolOwnedLabel] = ""
		machine.SetLabels(labels)
		machine.Spec.ClusterName = "karpenter-cluster"

		err := cl.Create(context.TODO(), machine)
		Expect(err).ToNot(HaveOccurred())

		machines, err := provider.List(context.TODO())
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
	})
})

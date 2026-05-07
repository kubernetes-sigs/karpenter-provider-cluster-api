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

package batcher_test

import (
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/batcher"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

var _ = Describe("Create Batching", func() {
	var (
		fakeMP  *fakeMachineProvider
		fakeMDP *fakeMDProvider
	)

	BeforeEach(func() {
		fakeMP = newFakeMachineProvider()
		fakeMDP = newFakeMDProvider()
	})

	// newCreateBatcher builds a CreateBatcher backed by a fake kube client
	// pre-seeded with the given NodeClaim names. The create batch executor
	// patches NodeClaim objects via client.Client, so they must exist before
	// the batch runs. The returned client can be used to verify NodeClaim
	// annotations after the batch completes.
	newCreateBatcher := func(nodeClaimNames ...string) (*batcher.CreateBatcher, client.Client) {
		builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
		for _, name := range nodeClaimNames {
			builder = builder.WithObjects(&karpv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: name},
			})
		}
		kubeClient := builder.Build()
		return batcher.NewCreateBatcher(ctx, kubeClient, fakeMP, fakeMDP, batcher.NewMDLockManager()), kubeClient
	}

	// expectMachineLabeled asserts that the Machine has the NodePoolMemberLabel.
	expectMachineLabeled := func(name, ns string) {
		GinkgoHelper()
		m := fakeMP.GetMachine(name, ns)
		Expect(m).NotTo(BeNil())
		Expect(m.Labels).To(HaveKeyWithValue(providers.NodePoolMemberLabel, ""))
	}

	// expectNodeClaimAnnotated asserts that the NodeClaim has the Machine annotation.
	expectNodeClaimAnnotated := func(kubeClient client.Client, ncName, expectedMachineRef string) {
		GinkgoHelper()
		nc := &karpv1.NodeClaim{}
		Expect(kubeClient.Get(ctx, client.ObjectKey{Name: ncName}, nc)).To(Succeed())
		Expect(nc.Annotations).To(HaveKeyWithValue(providers.MachineAnnotation, expectedMachineRef))
	}

	It("should reuse unclaimed machines without incrementing replicas", func() {
		// MD at 5 replicas with 5 unclaimed machines already present --
		// the batcher should claim them without touching replicas.
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 5))
		for i := range 5 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0"))
		}

		ncNames := []string{"nc-0", "nc-1", "nc-2", "nc-3", "nc-4"}
		cb, kubeClient := newCreateBatcher(ncNames...)

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.CreateOutput], 5)
		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = cb.Add(ctx, &batcher.CreateInput{
					NodeClaimName:         ncNames[idx],
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for i, r := range results {
			Expect(r.Err).NotTo(HaveOccurred())
			Expect(r.Output).NotTo(BeNil())
			expectMachineLabeled(r.Output.Machine.Name, "default")
			expectNodeClaimAnnotated(kubeClient, ncNames[i], "default/"+r.Output.Machine.Name)
		}

		// No replica increment needed -- existing unclaimed machines covered the demand.
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 1))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 0))

		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 5))
	})

	It("should batch different MachineDeployments into separate calls", func() {
		// Replicas match existing unclaimed machine counts.
		fakeMDP.AddMD(newMachineDeployment("md-east", "default", 4))
		fakeMDP.AddMD(newMachineDeployment("md-west", "default", 1))
		for i := range 4 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("m-east-%d", i), "default", "md-east"))
		}
		fakeMP.AddMachine(newMachineForMD("m-west-0", "default", "md-west"))

		ncNames := []string{"nc-0", "nc-1", "nc-2", "nc-3", "nc-4"}
		cb, kubeClient := newCreateBatcher(ncNames...)

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.CreateOutput], 5)
		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				input := &batcher.CreateInput{
					NodeClaimName:         ncNames[idx],
					MachineDeploymentName: "md-east",
					MachineDeploymentNS:   "default",
				}
				if idx == 3 {
					input.MachineDeploymentName = "md-west"
				}
				results[idx] = cb.Add(ctx, input)
			}(i)
		}
		wg.Wait()

		for i, r := range results {
			Expect(r.Err).NotTo(HaveOccurred())
			Expect(r.Output).NotTo(BeNil())
			expectMachineLabeled(r.Output.Machine.Name, "default")
			expectNodeClaimAnnotated(kubeClient, ncNames[i], "default/"+r.Output.Machine.Name)
		}

		// Two separate MDs = two separate batch executions, but no
		// replica increments needed (unclaimed machines cover demand).
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 2))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 0))

		mdEast := fakeMDP.GetMD("md-east", "default")
		Expect(mdEast).NotTo(BeNil())
		Expect(*mdEast.Spec.Replicas).To(BeNumerically("==", 4))

		mdWest := fakeMDP.GetMD("md-west", "default")
		Expect(mdWest).NotTo(BeNil())
		Expect(*mdWest.Spec.Replicas).To(BeNumerically("==", 1))
	})

	It("should return errors to all callers when MachineDeployment get fails", func() {
		fakeMDP.GetError = fmt.Errorf("simulated MD get failure")
		cb, _ := newCreateBatcher("nc-0", "nc-1", "nc-2")

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.CreateOutput], 3)
		for i := range 3 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = cb.Add(ctx, &batcher.CreateInput{
					NodeClaimName:         fmt.Sprintf("nc-%d", idx),
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.Err).To(HaveOccurred())
			Expect(r.Err.Error()).To(ContainSubstring("simulated MD get failure"))
			Expect(r.Output).To(BeNil())
		}
		// Get was called but Update should never have been reached.
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 1))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 0))
	})

	It("should return errors to all callers when MachineDeployment update fails", func() {
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 0))
		fakeMDP.UpdateError = fmt.Errorf("simulated MD update failure")
		cb, _ := newCreateBatcher("nc-0", "nc-1")

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.CreateOutput], 2)
		for i := range 2 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = cb.Add(ctx, &batcher.CreateInput{
					NodeClaimName:         fmt.Sprintf("nc-%d", idx),
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.Err).To(HaveOccurred())
			Expect(r.Err.Error()).To(ContainSubstring("simulated MD update failure"))
			Expect(r.Output).To(BeNil())
		}
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 0))

		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 1))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 1))
	})

	It("should increment only by deficit and not rollback when too few machines are available", func() {
		// 2 unclaimed machines already exist at replicas=2. We request 5,
		// so the batcher increments by 3 (deficit). Only 2 machines can
		// be claimed (the fake has no async machine creation). Replicas
		// stays at 5 (no rollback) so retries can pick up remaining machines.
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 2))
		fakeMP.AddMachine(newMachineForMD("machine-0", "default", "md-0"))
		fakeMP.AddMachine(newMachineForMD("machine-1", "default", "md-0"))

		ncNames := []string{"nc-0", "nc-1", "nc-2", "nc-3", "nc-4"}
		cb, kubeClient := newCreateBatcher(ncNames...)

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.CreateOutput], 5)
		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = cb.Add(ctx, &batcher.CreateInput{
					NodeClaimName:         ncNames[idx],
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		var successes, failures int
		for i, r := range results {
			if r.Err == nil && r.Output != nil {
				Expect(r.Output.Machine).NotTo(BeNil())
				expectMachineLabeled(r.Output.Machine.Name, "default")
				expectNodeClaimAnnotated(kubeClient, ncNames[i], "default/"+r.Output.Machine.Name)
				successes++
			} else {
				failures++
			}
		}
		Expect(successes).To(Equal(2))
		Expect(failures).To(Equal(3))

		// Replicas = 2 (original) + 3 (deficit increment) = 5.
		// No rollback -- retries will eventually claim the remaining machines.
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 5))
		// Only one Update: the deficit increment.
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 1))
	})
})

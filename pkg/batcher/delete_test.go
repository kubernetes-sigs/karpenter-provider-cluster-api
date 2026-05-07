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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/batcher"
)

var _ = Describe("Delete Batching", func() {
	var (
		fakeMP  *fakeMachineProvider
		fakeMDP *fakeMDProvider
		db      *batcher.DeleteBatcher
	)

	BeforeEach(func() {
		fakeMP = newFakeMachineProvider()
		fakeMDP = newFakeMDProvider()
		db = batcher.NewDeleteBatcher(ctx, fakeMP, fakeMDP, batcher.NewMDLockManager())
	})

	It("should batch the same MachineDeployment deletes into a single replica decrement", func() {
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 5))

		for i := range 5 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0"))
		}

		var wg sync.WaitGroup
		var successCount atomic.Int64
		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				result := db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
				if result.Err == nil && result.Output != nil {
					successCount.Add(1)
				}
			}(i)
		}
		wg.Wait()

		Expect(successCount.Load()).To(BeNumerically("==", 5))

		// One batched Get + one batched Update (not 5 individual calls).
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 1))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 1))
		// All 5 machines annotated for deletion in one batch.
		Expect(fakeMP.AddDeleteAnnotationCount.Load()).To(BeNumerically("==", 5))

		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 0))

		for i := range 5 {
			m := fakeMP.GetMachine(fmt.Sprintf("machine-%d", i), "default")
			Expect(m).NotTo(BeNil())
			Expect(m.Annotations).To(HaveKey(capiv1beta1.DeleteMachineAnnotation))
		}
	})

	It("should batch different MachineDeployments into separate calls", func() {
		fakeMDP.AddMD(newMachineDeployment("md-east", "default", 4))
		fakeMDP.AddMD(newMachineDeployment("md-west", "default", 1))

		eastMachines := []string{"m-east-0", "m-east-1", "m-east-2", "m-east-3"}
		for _, name := range eastMachines {
			fakeMP.AddMachine(newMachineForMD(name, "default", "md-east"))
		}
		fakeMP.AddMachine(newMachineForMD("m-west-0", "default", "md-west"))

		type testCase struct {
			machineName string
			mdName      string
		}
		cases := []testCase{
			{"m-east-0", "md-east"},
			{"m-east-1", "md-east"},
			{"m-east-2", "md-east"},
			{"m-west-0", "md-west"},
			{"m-east-3", "md-east"},
		}

		var wg sync.WaitGroup
		var eastSuccess, westSuccess atomic.Int64
		for _, tc := range cases {
			wg.Add(1)
			go func(tc testCase) {
				defer GinkgoRecover()
				defer wg.Done()

				result := db.Add(ctx, &batcher.DeleteInput{
					MachineName:           tc.machineName,
					MachineNamespace:      "default",
					MachineDeploymentName: tc.mdName,
					MachineDeploymentNS:   "default",
				})
				if result.Err == nil && result.Output != nil {
					if tc.mdName == "md-east" {
						eastSuccess.Add(1)
					} else {
						westSuccess.Add(1)
					}
				}
			}(tc)
		}
		wg.Wait()

		Expect(eastSuccess.Load()).To(BeNumerically("==", 4))
		Expect(westSuccess.Load()).To(BeNumerically("==", 1))

		// Two separate MDs = two separate batch executions.
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 2))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 2))

		mdEast := fakeMDP.GetMD("md-east", "default")
		Expect(mdEast).NotTo(BeNil())
		Expect(*mdEast.Spec.Replicas).To(BeNumerically("==", 0))

		mdWest := fakeMDP.GetMD("md-west", "default")
		Expect(mdWest).NotTo(BeNil())
		Expect(*mdWest.Spec.Replicas).To(BeNumerically("==", 0))
	})

	It("should return errors to all callers when MachineDeployment update fails", func() {
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 3))
		fakeMDP.UpdateError = fmt.Errorf("simulated MD update failure")

		for i := range 3 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0"))
		}

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.DeleteOutput], 3)
		for i := range 3 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.Err).To(HaveOccurred())
			Expect(r.Err.Error()).To(ContainSubstring("simulated MD update failure"))
		}
		// Annotations added then rolled back; replicas unchanged at 3.
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 3))

		for i := range 3 {
			m := fakeMP.GetMachine(fmt.Sprintf("machine-%d", i), "default")
			Expect(m).NotTo(BeNil())
			Expect(m.Annotations).NotTo(HaveKey(capiv1beta1.DeleteMachineAnnotation))
		}
	})

	It("should only fail the affected caller when annotation fails for one machine", func() {
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 3))

		fakeMP.AddMachine(newMachineForMD("machine-0", "default", "md-0"))
		fakeMP.AddMachine(newMachineForMD("machine-1", "default", "md-0"))
		// machine-2 is intentionally NOT added so AddDeleteAnnotation will fail with "not found".

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.DeleteOutput], 3)
		for i := range 3 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		var successes, failures int
		for _, r := range results {
			if r.Err == nil && r.Output != nil {
				successes++
			} else {
				Expect(r.Err).To(HaveOccurred())
				failures++
			}
		}
		Expect(successes).To(Equal(2))
		Expect(failures).To(Equal(1))

		// Replicas decremented by 2 (only the successfully annotated machines).
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 1))
		// Only one Update call for the batch (not per-machine).
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 1))
	})

	It("should rollback delete annotations when MachineDeployment get fails", func() {
		fakeMDP.GetError = fmt.Errorf("simulated MD get failure")

		for i := range 2 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0"))
		}

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.DeleteOutput], 2)
		for i := range 2 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.Err).To(HaveOccurred())
			Expect(r.Err.Error()).To(ContainSubstring("simulated MD get failure"))
		}
		// Get was called but Update should never have been reached.
		Expect(fakeMDP.GetCallCount.Load()).To(BeNumerically("==", 1))
		Expect(fakeMDP.UpdateCallCount.Load()).To(BeNumerically("==", 0))

		for i := range 2 {
			m := fakeMP.GetMachine(fmt.Sprintf("machine-%d", i), "default")
			Expect(m).NotTo(BeNil())
			Expect(m.Annotations).NotTo(HaveKey(capiv1beta1.DeleteMachineAnnotation))
		}
	})

	It("should clamp replicas to zero when decrementing below zero", func() {
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 1))

		for i := range 3 {
			fakeMP.AddMachine(newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0"))
		}

		var wg sync.WaitGroup
		results := make([]batcher.Result[batcher.DeleteOutput], 3)
		for i := range 3 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				results[idx] = db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}
		wg.Wait()

		for _, r := range results {
			Expect(r.Err).NotTo(HaveOccurred())
			Expect(r.Output).NotTo(BeNil())
		}
		// 3 deletes with initial replicas=1 should clamp to 0.
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 0))
		// All 3 machines annotated for deletion.
		Expect(fakeMP.AddDeleteAnnotationCount.Load()).To(BeNumerically("==", 3))
	})
})

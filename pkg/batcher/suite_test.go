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
	"context"
	"fmt"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/batcher"
)

var ctx context.Context
var cancel context.CancelFunc

func init() {
	_ = capiv1beta1.AddToScheme(scheme.Scheme)
}

func TestBatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Batcher Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
})

var _ = AfterSuite(func() {
	cancel()
})

var _ = Describe("Concurrent Create and Delete", func() {
	It("should not corrupt replicas when create and delete batches run concurrently on the same MD", func() {
		fakeMP := newFakeMachineProvider()
		fakeMDP := newFakeMDProvider()
		mdLock := batcher.NewMDLockManager()

		// MD starts at 8 replicas: 5 claimed machines + 3 unclaimed
		// (simulating leftovers from a previous batch).
		fakeMDP.AddMD(newMachineDeployment("md-0", "default", 8))
		for i := range 8 {
			m := newMachineForMD(fmt.Sprintf("machine-%d", i), "default", "md-0")
			if i < 5 {
				m.Labels["karpenter.sh/nodepool"] = ""
			}
			fakeMP.AddMachine(m)
		}

		// Create batcher: add 3 new nodes. Pre-seed NodeClaims for binding.
		ncNames := []string{"nc-0", "nc-1", "nc-2"}
		builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
		for _, name := range ncNames {
			builder = builder.WithObjects(&karpv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: name},
			})
		}
		cb := batcher.NewCreateBatcher(ctx, builder.Build(), fakeMP, fakeMDP, mdLock)

		// Delete batcher: remove 2 existing claimed machines.
		db := batcher.NewDeleteBatcher(ctx, fakeMP, fakeMDP, mdLock)

		var wg sync.WaitGroup

		// Fire 3 creates concurrently.
		createResults := make([]batcher.Result[batcher.CreateOutput], 3)
		for i := range 3 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				createResults[idx] = cb.Add(ctx, &batcher.CreateInput{
					NodeClaimName:         ncNames[idx],
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}

		// Fire 2 deletes concurrently.
		deleteResults := make([]batcher.Result[batcher.DeleteOutput], 2)
		for i := range 2 {
			wg.Add(1)
			go func(idx int) {
				defer GinkgoRecover()
				defer wg.Done()
				deleteResults[idx] = db.Add(ctx, &batcher.DeleteInput{
					MachineName:           fmt.Sprintf("machine-%d", idx),
					MachineNamespace:      "default",
					MachineDeploymentName: "md-0",
					MachineDeploymentNS:   "default",
				})
			}(i)
		}

		wg.Wait()

		// All operations should succeed.
		for _, r := range createResults {
			Expect(r.Err).NotTo(HaveOccurred())
			Expect(r.Output).NotTo(BeNil())
		}
		for _, r := range deleteResults {
			Expect(r.Err).NotTo(HaveOccurred())
			Expect(r.Output).NotTo(BeNil())
		}

		// Create found 3 unclaimed machines (machine-5,machine-6,machine-7) so no increment.
		// Delete decremented by 2. Final: 8 - 2 = 6.
		md := fakeMDP.GetMD("md-0", "default")
		Expect(md).NotTo(BeNil())
		Expect(*md.Spec.Replicas).To(BeNumerically("==", 6))
	})
})

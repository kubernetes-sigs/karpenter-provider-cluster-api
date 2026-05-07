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

package batcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// CreateInput is the input to a single create request within a batch.
type CreateInput struct {
	NodeClaimName         string
	MachineDeploymentName string
	MachineDeploymentNS   string
}

func (c CreateInput) BatchKey() string {
	return c.MachineDeploymentNS + "/" + c.MachineDeploymentName
}

// CreateOutput is the result of a successfully bound Machine.
type CreateOutput struct {
	MachineDeployment *capiv1beta1.MachineDeployment
	Machine           *capiv1beta1.Machine
}

// CreateBatcher coalesces concurrent CloudProvider.Create calls targeting the
// same MachineDeployment into a single replica increment + Machine poll cycle.
type CreateBatcher struct {
	batcher *Batcher[CreateInput, CreateOutput]
}

func NewCreateBatcher(
	ctx context.Context,
	kubeClient client.Client,
	machineProvider machine.Provider,
	mdProvider machinedeployment.Provider,
	mdLock *MDLockManager,
) *CreateBatcher {
	options := Options[CreateInput, CreateOutput]{
		Name:          "create_machine",
		IdleTimeout:   100 * time.Millisecond,
		MaxTimeout:    1 * time.Second,
		RequestHasher: BatchKeyHasher[CreateInput],
		BatchExecutor: execCreateBatch(kubeClient, machineProvider, mdProvider, mdLock),
	}
	return &CreateBatcher{batcher: NewBatcher(ctx, options)}
}

func (b *CreateBatcher) Add(ctx context.Context, input *CreateInput) Result[CreateOutput] {
	return b.batcher.Add(ctx, input)
}

// execCreateBatch returns a BatchExecutor that provisions Machines for a set
// of NodeClaims targeting the same MachineDeployment. The algorithm is:
//
//  1. Lock the MachineDeployment and count existing unclaimed Machines (those
//     without the NodePoolMemberLabel). We only increment spec.replicas by the
//     deficit (requested − unclaimed) so that leftover Machines from a previous
//     batch are reused instead of leaked. This eliminates the need for an
//     explicit rollback of replicas on partial failure.
//  2. Poll for N unclaimed Machines to appear. This runs unlocked and can take
//     up to 30 seconds while CAPI's MachineSet controller creates them.
//  3. Bind each Machine to its corresponding NodeClaim in parallel by labeling
//     the Machine as claimed and annotating the NodeClaim with the Machine
//     reference.
//
// Requests that cannot be fulfilled (e.g. poll timeout, binding failure) are
// returned with an error so the lifecycle controller can re-reconcile them.
// Because we count unclaimed Machines on every batch, retries are cheap — we
// won't increment replicas for Machines that already exist.
//
// A subsequent batch can claim Machines that a previous batch created but has
// not yet bound (they are still unclaimed during the poll window). This is
// safe because the earlier batch's unfulfilled requests will error out, the
// lifecycle controller will re-reconcile them into a future batch, and that
// batch will count the Machines the later batch already created — so replicas
// are never over-incremented.
//
// Concurrent delete batches do not interfere: the replica read-modify-write
// is serialized by MDLockManager, and the poll skips Machines that carry the
// delete-machine annotation or a non-zero deletion timestamp.
func execCreateBatch(
	kubeClient client.Client,
	machineProvider machine.Provider,
	mdProvider machinedeployment.Provider,
	mdLock *MDLockManager,
) BatchExecutor[CreateInput, CreateOutput] {
	return func(ctx context.Context, inputs []*CreateInput) []Result[CreateOutput] {
		n := len(inputs)
		results := make([]Result[CreateOutput], n)

		if n == 0 {
			return results
		}

		mdName := inputs[0].MachineDeploymentName
		mdNS := inputs[0].MachineDeploymentNS
		mdKey := mdNS + "/" + mdName

		// 1) Count unclaimed Machines and increment replicas by the deficit.
		mdLock.Lock(mdKey)
		md, err := mdProvider.Get(ctx, mdName, mdNS)
		if err != nil {
			mdLock.Unlock(mdKey)
			for i := range results {
				results[i] = Result[CreateOutput]{Err: fmt.Errorf("unable to get MachineDeployment %q: %w", mdName, err)}
			}
			return results
		}

		unclaimed := countUnclaimedMachines(ctx, machineProvider, mdName, mdNS)
		deficit := int32(n) - int32(unclaimed)
		if deficit < 0 {
			deficit = 0
		}

		log.FromContext(ctx).V(1).Info("create batch", "machineDeployment", mdKey, "requests", n, "unclaimed", unclaimed, "deficit", deficit)

		if deficit > 0 {
			currentReplicas := ptr.Deref(md.Spec.Replicas, 0)
			md.Spec.Replicas = ptr.To(currentReplicas + deficit)
			if err := mdProvider.Update(ctx, md); err != nil {
				mdLock.Unlock(mdKey)
				for i := range results {
					results[i] = Result[CreateOutput]{Err: fmt.Errorf("unable to increment MachineDeployment %q replicas: %w", mdName, err)}
				}
				return results
			}
		}
		mdLock.Unlock(mdKey)

		// 2) Poll for N unclaimed Machines (unlocked; can take up to 30s).
		machines := pollForNUnclaimedMachines(ctx, machineProvider, mdName, mdNS, n, 30*time.Second)

		// 3) Bind each Machine to a NodeClaim in parallel.
		var wg sync.WaitGroup
		for i := 0; i < len(machines) && i < n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx] = bindMachineToNodeClaim(ctx, kubeClient, machineProvider, md, machines[idx], inputs[idx].NodeClaimName)
			}(i)
		}
		wg.Wait()

		for i, r := range results {
			if r.Err == nil && r.Output == nil {
				results[i] = Result[CreateOutput]{Err: fmt.Errorf("no Machine available for NodeClaim")}
			}
		}

		return results
	}
}

// pollForNUnclaimedMachines lists Machines belonging to the MachineDeployment
// that have not yet been claimed (no NodePoolMemberLabel). It polls every
// second until count Machines are found or the timeout elapses, returning
// whatever has been collected so far.
func pollForNUnclaimedMachines(
	ctx context.Context,
	machineProvider machine.Provider,
	mdName, mdNS string,
	count int,
	timeout time.Duration,
) []*capiv1beta1.Machine {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      providers.NodePoolMemberLabel,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
			{
				Key:      capiv1beta1.MachineDeploymentNameLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{mdName},
			},
		},
	}

	claimed := map[string]bool{}
	var found []*capiv1beta1.Machine

	deadline := time.After(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		machineList, err := machineProvider.List(ctx, mdNS, selector)
		if err == nil {
			for _, m := range machineList {
				if claimed[m.Name] {
					continue
				}
				if m.DeletionTimestamp != nil {
					continue
				}
				if _, marked := m.GetAnnotations()[capiv1beta1.DeleteMachineAnnotation]; marked {
					continue
				}
				claimed[m.Name] = true
				found = append(found, m)
				if len(found) >= count {
					return found
				}
			}
		}

		if len(found) >= count {
			return found
		}

		select {
		case <-ctx.Done():
			return found
		case <-deadline:
			return found
		case <-ticker.C:
		}
	}
}

// countUnclaimedMachines returns the number of Machines in the
// MachineDeployment that are not yet claimed and not pending deletion.
func countUnclaimedMachines(
	ctx context.Context,
	machineProvider machine.Provider,
	mdName, mdNS string,
) int {
	selector := &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      providers.NodePoolMemberLabel,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
			{
				Key:      capiv1beta1.MachineDeploymentNameLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{mdName},
			},
		},
	}

	machines, err := machineProvider.List(ctx, mdNS, selector)
	if err != nil {
		return 0
	}

	count := 0
	for _, m := range machines {
		if m.DeletionTimestamp != nil {
			continue
		}
		if _, marked := m.GetAnnotations()[capiv1beta1.DeleteMachineAnnotation]; marked {
			continue
		}
		count++
	}
	return count
}

// bindMachineToNodeClaim claims a Machine for a NodeClaim by labeling the
// Machine with NodePoolMemberLabel and annotating the NodeClaim with the
// Machine reference.
func bindMachineToNodeClaim(
	ctx context.Context,
	kubeClient client.Client,
	machineProvider machine.Provider,
	md *capiv1beta1.MachineDeployment,
	m *capiv1beta1.Machine,
	nodeClaimName string,
) Result[CreateOutput] {
	var fresh *capiv1beta1.Machine
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		fresh, err = machineProvider.Get(ctx, m.Name, m.Namespace)
		if err != nil {
			return Result[CreateOutput]{Err: fmt.Errorf("unable to get Machine %q: %w", m.Name, err)}
		}

		labels := fresh.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[providers.NodePoolMemberLabel] = ""
		fresh.SetLabels(labels)
		err = machineProvider.Update(ctx, fresh)
		if err == nil {
			break
		}
	}
	if err != nil {
		return Result[CreateOutput]{Err: fmt.Errorf("unable to label Machine %q: %w", fresh.Name, err)}
	}

	// Annotate NodeClaim with Machine reference using a merge-patch to avoid
	// stale-resourceVersion conflicts (the NodeClaim may have been updated by
	// another controller while the batch was accumulating).
	// If the NodeClaim annotation fails, we roll back the
	// Machine label so it can be reclaimed by a future batch.
	machineRef := fmt.Sprintf("%s/%s", fresh.Namespace, fresh.Name)
	patchBytes := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, providers.MachineAnnotation, machineRef))
	nc := &karpv1.NodeClaim{}
	nc.Name = nodeClaimName
	if err := kubeClient.Patch(ctx, nc, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		rollbackFresh, getErr := machineProvider.Get(ctx, fresh.Name, fresh.Namespace)
		if getErr == nil {
			lbls := rollbackFresh.GetLabels()
			delete(lbls, providers.NodePoolMemberLabel)
			rollbackFresh.SetLabels(lbls)
			if updateErr := machineProvider.Update(ctx, rollbackFresh); updateErr != nil {
				log.FromContext(ctx).Error(updateErr, "create batch: unable to remove member label from Machine", "machine", rollbackFresh.Name)
			}
		}
		return Result[CreateOutput]{Err: fmt.Errorf("unable to annotate NodeClaim %q: %w", nodeClaimName, err)}
	}

	return Result[CreateOutput]{Output: &CreateOutput{MachineDeployment: md, Machine: fresh}}
}

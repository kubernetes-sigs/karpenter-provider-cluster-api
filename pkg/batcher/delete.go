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

	"github.com/samber/lo"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
)

// DeleteInput is the input to a single delete request within a batch.
type DeleteInput struct {
	MachineName           string
	MachineNamespace      string
	MachineDeploymentName string
	MachineDeploymentNS   string
}

func (d DeleteInput) BatchKey() string {
	return d.MachineDeploymentNS + "/" + d.MachineDeploymentName
}

// DeleteOutput is the result of a successful Machine deletion request.
type DeleteOutput struct{}

// DeleteBatcher coalesces concurrent CloudProvider.Delete calls targeting the
// same MachineDeployment into a single replica decrement.
type DeleteBatcher struct {
	batcher *Batcher[DeleteInput, DeleteOutput]
}

func NewDeleteBatcher(
	ctx context.Context,
	machineProvider machine.Provider,
	mdProvider machinedeployment.Provider,
	mdLock *MDLockManager,
) *DeleteBatcher {
	options := Options[DeleteInput, DeleteOutput]{
		Name:        "delete_machine",
		IdleTimeout: 100 * time.Millisecond,
		MaxTimeout:  1 * time.Second,
		RequestHasher: BatchKeyHasher[DeleteInput],
		BatchExecutor: execDeleteBatch(machineProvider, mdProvider, mdLock),
	}
	return &DeleteBatcher{batcher: NewBatcher(ctx, options)}
}

func (b *DeleteBatcher) Add(ctx context.Context, input *DeleteInput) Result[DeleteOutput] {
	return b.batcher.Add(ctx, input)
}

// execDeleteBatch returns a BatchExecutor that deletes Machines from a
// MachineDeployment. The algorithm is:
//
//  1. Annotate each Machine with the CAPI delete-machine annotation in
//     parallel. This tells the MachineSet controller to prefer deleting these
//     specific Machines when replicas are decremented.
//  2. Lock the MachineDeployment and decrement spec.replicas by the number of
//     successfully annotated Machines. If the decrement fails, we roll back
//     the annotations so the Machines are not orphaned.
//
// Concurrent create batches do not interfere: the replica read-modify-write
// is serialized by MDLockManager, and create's poll skips Machines that carry
// the delete-machine annotation, so a Machine being deleted will not be
// claimed by a concurrent create batch.
func execDeleteBatch(
	machineProvider machine.Provider,
	mdProvider machinedeployment.Provider,
	mdLock *MDLockManager,
) BatchExecutor[DeleteInput, DeleteOutput] {
	return func(ctx context.Context, inputs []*DeleteInput) []Result[DeleteOutput] {
		n := len(inputs)
		results := make([]Result[DeleteOutput], n)

		if n == 0 {
			return results
		}

		mdName := inputs[0].MachineDeploymentName
		mdNS := inputs[0].MachineDeploymentNS
		mdKey := mdNS + "/" + mdName

		// 1) Annotate each Machine for deletion (parallel, unlocked).
		var wg sync.WaitGroup
		annotated := make([]bool, n)
		for i, input := range inputs {
			wg.Add(1)
			go func(idx int, machineName, machineNS string) {
				defer wg.Done()
				fresh, err := machineProvider.Get(ctx, machineName, machineNS)
				if err != nil {
					results[idx] = Result[DeleteOutput]{Err: fmt.Errorf("unable to get Machine %q: %w", machineName, err)}
					return
				}
				if err := machineProvider.AddDeleteAnnotation(ctx, fresh); err != nil {
					results[idx] = Result[DeleteOutput]{Err: fmt.Errorf("unable to annotate Machine %q for deletion: %w", machineName, err)}
					return
				}
				annotated[idx] = true
			}(i, input.MachineName, input.MachineNamespace)
		}
		wg.Wait()

		// We can get partial fulfillment of the annotation step, so we only
		// decrement replicas by the number that were successfully annotated.
		successCount := int32(lo.CountBy(annotated, func(b bool) bool { return b }))

		log.FromContext(ctx).V(1).Info("delete batch", "machineDeployment", mdKey, "requests", n, "annotated", successCount)

		if successCount == 0 {
			return results
		}

		// 2) Lock and decrement replicas by the number of annotated Machines.
		mdLock.Lock(mdKey)
		md, err := mdProvider.Get(ctx, mdName, mdNS)
		if err != nil {
			mdLock.Unlock(mdKey)
			log.FromContext(ctx).Error(err, "delete batch: unable to get MachineDeployment", "machineDeployment", mdName)
			rollbackDeleteAnnotations(ctx, machineProvider, inputs, annotated)
			for i := range results {
				if annotated[i] {
					results[i] = Result[DeleteOutput]{Err: fmt.Errorf("unable to get MachineDeployment %q for replica decrement: %w", mdName, err)}
				}
			}
			return results
		}

		currentReplicas := ptr.Deref(md.Spec.Replicas, 0)
		newReplicas := currentReplicas - successCount
		if newReplicas < 0 {
			newReplicas = 0
		}
		md.Spec.Replicas = ptr.To(newReplicas)

		if err := mdProvider.Update(ctx, md); err != nil {
			mdLock.Unlock(mdKey)
			log.FromContext(ctx).Error(err, "delete batch: unable to update MachineDeployment replicas", "machineDeployment", mdName)
			rollbackDeleteAnnotations(ctx, machineProvider, inputs, annotated)
			for i := range results {
				if annotated[i] {
					results[i] = Result[DeleteOutput]{Err: fmt.Errorf("unable to update MachineDeployment %q replicas: %w", mdName, err)}
				}
			}
			return results
		}
		mdLock.Unlock(mdKey)

		// Deliver success results for annotated Machines.
		for i := range results {
			if annotated[i] {
				results[i] = Result[DeleteOutput]{Output: &DeleteOutput{}}
			}
		}

		return results
	}
}

// rollbackDeleteAnnotations removes the delete-machine annotation from
// Machines that were annotated but whose replica decrement failed. This
// prevents the MachineSet controller from deleting Machines that Karpenter
// did not successfully request deletion for.
func rollbackDeleteAnnotations(ctx context.Context, machineProvider machine.Provider, inputs []*DeleteInput, annotated []bool) {
	for i, ok := range annotated {
		if !ok {
			continue
		}
		fresh, err := machineProvider.Get(ctx, inputs[i].MachineName, inputs[i].MachineNamespace)
		if err != nil {
			log.FromContext(ctx).Error(err, "delete batch rollback: unable to re-fetch Machine", "machine", inputs[i].MachineName)
			continue
		}
		if err := machineProvider.RemoveDeleteAnnotation(ctx, fresh); err != nil {
			log.FromContext(ctx).Error(err, "delete batch rollback: unable to remove delete annotation from Machine", "machine", fresh.Name)
		}
	}
}

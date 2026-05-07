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

// Package batcher implements a generic time-windowed batching library that
// coalesces concurrent requests into a single bulk operation, reducing API
// call volume and write contention on shared Kubernetes resources.
//
// The core [Batcher] type collects Add calls during an idle/max timeout
// window, groups them by a caller-supplied hash, and dispatches each group to
// a [BatchExecutor]. Callers block on Add until their result is available.
//
// Architecture adapted from github.com/aws/karpenter-provider-aws/pkg/batcher.
package batcher

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Options[T any, U any] struct {
	Name          string
	IdleTimeout   time.Duration
	MaxTimeout    time.Duration
	RequestHasher RequestHasher[T]
	BatchExecutor BatchExecutor[T, U]
}

type Result[U any] struct {
	Output *U
	Err    error
}

type request[T any, U any] struct {
	ctx       context.Context
	hash      uint64
	input     *T
	requestor chan Result[U]
}

// Batcher coalesces Add calls into time-windowed batches and dispatches
// them to a BatchExecutor.  Each caller blocks on Add until its result
// is available.
type Batcher[T any, U any] struct {
	ctx     context.Context
	options Options[T, U]

	mu       sync.Mutex
	requests map[uint64][]*request[T, U]
	trigger  chan struct{}
}

// BatchExecutor executes a batch of inputs and returns one Result per input,
// in the same order.
type BatchExecutor[T any, U any] func(ctx context.Context, inputs []*T) []Result[U]

// RequestHasher returns a bucket key for a given input so that requests
// targeting different resources can be batched separately.
type RequestHasher[T any] func(ctx context.Context, input *T) uint64

func NewBatcher[T any, U any](ctx context.Context, options Options[T, U]) *Batcher[T, U] {
	b := &Batcher[T, U]{
		ctx:      ctx,
		options:  options,
		requests: map[uint64][]*request[T, U]{},
		trigger:  make(chan struct{}, 1),
	}
	go b.run()
	return b
}

// Add submits an input to the batcher and blocks until the batch executes.
func (b *Batcher[T, U]) Add(ctx context.Context, input *T) Result[U] {
	req := &request[T, U]{
		ctx:       ctx,
		hash:      b.options.RequestHasher(ctx, input),
		input:     input,
		requestor: make(chan Result[U], 1),
	}
	b.mu.Lock()
	b.requests[req.hash] = append(b.requests[req.hash], req)
	b.mu.Unlock()

	// Non-blocking send: if trigger is already pending, this is a no-op.
	select {
	case b.trigger <- struct{}{}:
	default:
	}

	select {
	case result := <-req.requestor:
		return result
	case <-ctx.Done():
		return Result[U]{Err: ctx.Err()}
	}
}

// BatchKeyer is implemented by input types that know their own batch key.
type BatchKeyer interface {
	BatchKey() string
}

// BatchKeyHasher hashes inputs that implement BatchKeyer.
func BatchKeyHasher[T BatchKeyer](_ context.Context, input *T) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte((*input).BatchKey()))
	return h.Sum64()
}

func (b *Batcher[T, U]) run() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.trigger:
		}

		b.waitForIdle()

		b.mu.Lock()
		buckets := b.requests
		b.requests = map[uint64][]*request[T, U]{}
		b.mu.Unlock()

		for _, reqs := range buckets {
			go b.runBatch(reqs)
		}
	}
}

func (b *Batcher[T, U]) waitForIdle() {
	maxTimer := time.NewTimer(b.options.MaxTimeout)
	defer maxTimer.Stop()
	idleTimer := time.NewTimer(b.options.IdleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.trigger:
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(b.options.IdleTimeout)
		case <-maxTimer.C:
			return
		case <-idleTimer.C:
			return
		}
	}
}

func (b *Batcher[T, U]) runBatch(reqs []*request[T, U]) {
	inputs := make([]*T, len(reqs))
	for i, r := range reqs {
		inputs[i] = r.input
	}

	// TODO(maxcao13): Karpenter AWS emits batching metrics for observability. We can consider doing this in the future, but for now, just log.
	log.FromContext(reqs[0].ctx).V(1).Info("executing batch", "batcher", b.options.Name, "requests", len(reqs))
	results := b.options.BatchExecutor(reqs[0].ctx, inputs)

	for i, r := range results {
		if i < len(reqs) {
			reqs[i].requestor <- r
		}
	}
	for i := len(results); i < len(reqs); i++ {
		reqs[i].requestor <- Result[U]{Err: fmt.Errorf("batch executor returned too few results")}
	}
}

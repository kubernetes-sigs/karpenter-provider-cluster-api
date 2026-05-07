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

import "sync"

// MDLockManager provides per-MachineDeployment mutexes so that create and
// delete batch executors serialise their replica read-modify-write cycles
// on the same MachineDeployment.
type MDLockManager struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func NewMDLockManager() *MDLockManager {
	return &MDLockManager{locks: map[string]*sync.Mutex{}}
}

func (m *MDLockManager) Lock(key string) {
	m.mu.Lock()
	l, ok := m.locks[key]
	if !ok {
		l = &sync.Mutex{}
		m.locks[key] = l
	}
	m.mu.Unlock()
	l.Lock()
}

func (m *MDLockManager) Unlock(key string) {
	m.mu.Lock()
	l := m.locks[key]
	m.mu.Unlock()
	l.Unlock()
}

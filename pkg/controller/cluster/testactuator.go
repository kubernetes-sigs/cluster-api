/*
Copyright 2018 The Kubernetes Authors.

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

package cluster

import (
	"sync"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type TestActuator struct {
	unblock            chan string
	BlockOnReconcile   bool
	BlockOnDelete      bool
	ReconcileCallCount int64
	DeleteCallCount    int64
	Lock               sync.Mutex
}

func (a *TestActuator) Reconcile(*v1alpha1.Cluster) error {
	defer func() {
		if a.BlockOnReconcile {
			<-a.unblock
		}
	}()

	a.Lock.Lock()
	defer a.Lock.Unlock()
	a.ReconcileCallCount++
	return nil
}

func (a *TestActuator) Delete(*v1alpha1.Cluster) error {
	defer func() {
		if a.BlockOnDelete {
			<-a.unblock
		}
	}()

	a.Lock.Lock()
	defer a.Lock.Unlock()
	a.DeleteCallCount++
	return nil
}

func newTestActuator() *TestActuator {
	ta := new(TestActuator)
	ta.unblock = make(chan string)
	return ta
}

func (a *TestActuator) Unblock() {
	close(a.unblock)
}

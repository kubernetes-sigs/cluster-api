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

package machine

import (
	"context"
	"sync"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var _ Actuator = &TestActuator{}

type TestActuator struct {
	unblock         chan string
	BlockOnCreate   bool
	BlockOnDelete   bool
	BlockOnUpdate   bool
	BlockOnExists   bool
	CreateCallCount int64
	DeleteCallCount int64
	UpdateCallCount int64
	ExistsCallCount int64
	ExistsValue     bool
	Lock            sync.Mutex
}

func (a *TestActuator) Create(context.Context, *v1alpha1.Cluster, *v1alpha1.Machine) error {
	defer func() {
		if a.BlockOnCreate {
			<-a.unblock
		}
	}()

	a.Lock.Lock()
	defer a.Lock.Unlock()
	a.CreateCallCount++
	return nil
}

func (a *TestActuator) Delete(context.Context, *v1alpha1.Cluster, *v1alpha1.Machine) error {
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

func (a *TestActuator) Update(ctx context.Context, c *v1alpha1.Cluster, machine *v1alpha1.Machine) error {
	defer func() {
		if a.BlockOnUpdate {
			<-a.unblock
		}
	}()
	a.Lock.Lock()
	defer a.Lock.Unlock()
	a.UpdateCallCount++
	return nil
}

func (a *TestActuator) Exists(context.Context, *v1alpha1.Cluster, *v1alpha1.Machine) (bool, error) {
	defer func() {
		if a.BlockOnExists {
			<-a.unblock
		}
	}()

	a.Lock.Lock()
	defer a.Lock.Unlock()
	a.ExistsCallCount++
	return a.ExistsValue, nil
}

func newTestActuator() *TestActuator {
	ta := new(TestActuator)
	ta.unblock = make(chan string)
	return ta
}

func (a *TestActuator) Unblock() {
	close(a.unblock)
}

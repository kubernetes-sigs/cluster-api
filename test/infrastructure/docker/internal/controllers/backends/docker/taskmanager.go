/*
Copyright 2026 The Kubernetes Authors.

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

package docker

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// NewTaskManager create a new TaskManager that can be used for running dockerMachines provisioning tasks async of the reconcile loop.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:        make(map[string]*TaskState),
		progressChan: make(chan event.GenericEvent, 100),
	}
}

// TaskManager is responsible for running dockerMachines provisioning tasks async of the reconcile loop.
type TaskManager struct {
	mu           sync.RWMutex
	tasks        map[string]*TaskState
	progressChan chan event.GenericEvent
}

// TaskState represent the state of a task.
type TaskState struct {
	DockerMachineKey            client.ObjectKey
	ID                          string
	Completed                   bool
	CurrentOperationDescription string
	CurrentOperation            int
	TotalOperations             int
	Cancel                      context.CancelFunc
	Err                         error
}

func (s *TaskState) toEvent() event.GenericEvent {
	return event.GenericEvent{
		Object: &infrav1.DevMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.DockerMachineKey.Name,
				Namespace: s.DockerMachineKey.Namespace,
			},
		},
	}
}

func (s *TaskState) String() string {
	if s.Completed {
		return ""
	}
	if s.Err != nil {
		return fmt.Sprintf("%s failed: %s", s.CurrentOperationDescription, s.Err.Error())
	}
	return fmt.Sprintf("%s (%d of %d)", s.CurrentOperationDescription, s.CurrentOperation, s.TotalOperations)
}

// Operation defines an operation in a task, e.g. one of the preKubeadmCommand.
type Operation struct {
	Description string
	F           func(ctx context.Context) error
}

// RegisterTask start a task for a dockerMachine.
func (m *TaskManager) RegisterTask(ctx context.Context, dockerMachine client.Object, id string, operations []Operation, timeout time.Duration) (*TaskState, error) {
	m.mu.Lock()
	if _, exists := m.tasks[taskUID(dockerMachine, id)]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("task for %s, ID %s already exist", klog.KObj(dockerMachine), id)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("reconcileMode", "asyncTask")

	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to container runtime: %v", err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), timeout)
	ctxWithTimeout = ctrl.LoggerInto(ctxWithTimeout, log)
	ctxWithTimeout = container.RuntimeInto(ctxWithTimeout, containerRuntime)

	state := &TaskState{
		DockerMachineKey:            client.ObjectKeyFromObject(dockerMachine),
		ID:                          id,
		Completed:                   false,
		CurrentOperation:            0,
		CurrentOperationDescription: "Starting",
		TotalOperations:             len(operations),
		Cancel:                      cancel,
	}
	m.tasks[taskUID(dockerMachine, id)] = state
	m.mu.Unlock()

	go m.runTask(ctxWithTimeout, state, operations)

	snapshot := *state
	return new(snapshot), nil
}

func (m *TaskManager) runTask(ctx context.Context, state *TaskState, operations []Operation) {
	select {
	case <-ctx.Done():
		m.mu.Lock()
		state.Err = ctx.Err()
		m.mu.Unlock()
		m.progressChan <- state.toEvent()
		return
	default:
		for i, op := range operations {
			// Report error if the context has been canceled
			if ctx.Err() != nil {
				m.mu.Lock()
				state.Err = ctx.Err()
				m.mu.Unlock()
				m.progressChan <- state.toEvent()
				return
			}

			// Start the next operation
			m.mu.Lock()
			state.CurrentOperationDescription = op.Description
			state.CurrentOperation = i + 1
			m.mu.Unlock()
			m.progressChan <- state.toEvent()

			if err := op.F(ctx); err != nil {
				// Report error if the operation fails
				m.mu.Lock()
				state.Err = err
				m.mu.Unlock()
				m.progressChan <- state.toEvent()
				return
			}
		}

		// Report all the operations have been completed
		m.mu.Lock()
		state.Completed = true
		m.mu.Unlock()
		m.progressChan <- state.toEvent()
	}
}

// GetStatus return state of a task.
func (m *TaskManager) GetStatus(dockerMachine client.Object, id string) *TaskState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.tasks[taskUID(dockerMachine, id)]
	if !exists {
		return nil
	}

	snapshot := *state
	return new(snapshot)
}

// ResetStatus the status of a task for a dockerMachine.
func (m *TaskManager) ResetStatus(dockerMachine client.Object, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tasks, taskUID(dockerMachine, id))
}

// Cancel cancels all the tasks for a dockerMachine.
func (m *TaskManager) Cancel(dockerMachine client.Object) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, state := range m.tasks {
		if state.DockerMachineKey != client.ObjectKeyFromObject(dockerMachine) {
			continue
		}

		state.Cancel()
		delete(m.tasks, id)
	}
}

// GetSource return a controller runtime source that can be used to get notifications when an operation for
// a dockerMachine is completed.
func (m *TaskManager) GetSource() source.Source {
	return source.Channel(m.progressChan, &handler.EnqueueRequestForObject{})
}

func taskUID(dockerMachine client.Object, id string) string {
	return fmt.Sprintf("%s/%s", client.ObjectKeyFromObject(dockerMachine), id)
}

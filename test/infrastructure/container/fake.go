/*
Copyright 2021 The Kubernetes Authors.

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

package container

import (
	"context"
	"io"
)

var runContainerCallLog []RunContainerArgs
var deleteContainerCallLog []string
var killContainerCallLog []KillContainerArgs
var execContainerCallLog []ExecContainerArgs

// RunContainerArgs contains the arguments passed to calls to RunContainer.
type RunContainerArgs struct {
	RunConfig *RunContainerInput
	Output    io.Writer
}

// KillContainerArgs contains the arguments passed to calls to Kill.
type KillContainerArgs struct {
	Container string
	Signal    string
}

// ExecContainerArgs contains the arguments passed to calls to ExecContainer.
type ExecContainerArgs struct {
	ContainerName string
	Config        *ExecContainerInput
	Command       string
	Args          []string
}

type FakeRuntime struct {
}

// NewFakeClient gets a client for testing.
func NewFakeClient() (Runtime, error) {
	return &FakeRuntime{}, nil
}

// SaveContainerImage saves a Docker image to the file specified by dest.
func (f *FakeRuntime) SaveContainerImage(ctx context.Context, image, dest string) error {
	return nil
}

// PullContainerImageIfNotExists triggers the Docker engine to pull an image, but only if it doesn't
// already exist. This is important when we're using locally built images in CI which
// do not exist remotely.
func (f *FakeRuntime) PullContainerImageIfNotExists(ctx context.Context, image string) error {
	return nil
}

// PullContainerImage triggers the Docker engine to pull an image.
func (f *FakeRuntime) PullContainerImage(ctx context.Context, image string) error {
	return nil
}

// ImageExistsLocally returns if the specified image exists in local container image cache.
func (f *FakeRuntime) ImageExistsLocally(ctx context.Context, image string) (bool, error) {
	return false, nil
}

// GetHostPort looks up the host port bound for the port and protocol (e.g. "6443/tcp").
func (f *FakeRuntime) GetHostPort(ctx context.Context, containerName, portAndProtocol string) (string, error) {
	return "", nil
}

// ExecContainer executes a command in a running container and writes any output to the provided writer.
func (f *FakeRuntime) ExecContainer(ctx context.Context, containerName string, config *ExecContainerInput, command string, args ...string) error {
	execContainerCallLog = append(execContainerCallLog, ExecContainerArgs{
		ContainerName: containerName,
		Config:        config,
		Command:       command,
		Args:          args,
	})
	return nil
}

// ExecContainerCalls returns the set of arguments that have been passed to the ExecContainer method. This can
// be used by test code to validate the expected input.
func (f *FakeRuntime) ExecContainerCalls() []ExecContainerArgs {
	return execContainerCallLog
}

// ResetExecContainerCallLogs clears all existing records of any calls to the ExecContainer method.
func (f *FakeRuntime) ResetExecContainerCallLogs() {
	execContainerCallLog = []ExecContainerArgs{}
}

// ListContainers returns a list of all containers.
func (f *FakeRuntime) ListContainers(ctx context.Context, filters FilterBuilder) ([]Container, error) {
	return []Container{}, nil
}

// DeleteContainer will remove a container, forcing removal if still running.
func (f *FakeRuntime) DeleteContainer(ctx context.Context, containerName string) error {
	deleteContainerCallLog = append(deleteContainerCallLog, containerName)
	return nil
}

// DeleteContainerCalls returns the list of containerName arguments passed to calls to DeleteContainer.
func (f *FakeRuntime) DeleteContainerCalls() []string {
	return deleteContainerCallLog
}

// ResetDeleteContainerCallLogs clears all existing records of any calls to the DeleteContainer method.
func (f *FakeRuntime) ResetDeleteContainerCallLogs() {
	deleteContainerCallLog = []string{}
}

// KillContainer will kill a running container with the specified signal.
func (f *FakeRuntime) KillContainer(ctx context.Context, containerName, signal string) error {
	killContainerCallLog = append(killContainerCallLog, KillContainerArgs{
		Container: containerName,
		Signal:    signal,
	})
	return nil
}

// KillContainerCalls returns the list of arguments passed to calls to the KillContainer method.
func (f *FakeRuntime) KillContainerCalls() []KillContainerArgs {
	return killContainerCallLog
}

// ResetKillContainerCallLogs clears all existing records of any calls to the KillContainer method.
func (f *FakeRuntime) ResetKillContainerCallLogs() {
	killContainerCallLog = []KillContainerArgs{}
}

// GetContainerIPs inspects a container to get its IPv4 and IPv6 IP addresses.
// Will not error if there is no IP address assigned. Calling code will need to
// determine whether that is an issue or not.
func (f *FakeRuntime) GetContainerIPs(ctx context.Context, containerName string) (string, string, error) {
	return containerName + "IPv4", containerName + "IPv6", nil
}

// ContainerDebugInfo gets the container metadata and logs from the runtime (docker inspect, docker logs).
func (f *FakeRuntime) ContainerDebugInfo(ctx context.Context, containerName string, w io.Writer) error {
	return nil
}

// RunContainer will run a docker container with the given settings and arguments, returning any errors.
func (f *FakeRuntime) RunContainer(ctx context.Context, runConfig *RunContainerInput, output io.Writer) error {
	runContainerCallLog = append(runContainerCallLog, RunContainerArgs{runConfig, output})
	return nil
}

// RunContainerCalls returns a list of arguments passed in to all calls to RunContainer.
func (f *FakeRuntime) RunContainerCalls() []RunContainerArgs {
	return runContainerCallLog
}

// ResetRunContainerCallLogs clears all existing records of calls to the RunContainer method.
func (f *FakeRuntime) ResetRunContainerCallLogs() {
	runContainerCallLog = []RunContainerArgs{}
}

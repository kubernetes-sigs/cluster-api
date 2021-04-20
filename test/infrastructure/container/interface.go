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
	"os"
)

// RuntimeInterface defines the interface for interacting with a container runtime.
type RuntimeInterface interface {
	SaveContainerImage(ctx context.Context, image, dest string) error
	PullContainerImage(ctx context.Context, image string) error
	GetHostPort(ctx context.Context, name, portAndProtocol string) (string, error)
	ExecToFile(ctx context.Context, containerName string, fileOnHost *os.File, command string, args ...string) error
	RunContainer(ctx context.Context, runConfig *RunContainerInput, output io.Writer) error
}

// RunContainerInput holds the configuration settings for running a container.
type RunContainerInput struct {
	// Image is the name of the image to run.
	Image string
	// Network is the name of the network to connect to.
	Network string
	// User is the user name to run as.
	User string
	// Group is the user group to run as.
	Group string
	// Volumes is a collection of any volumes (docker's "-v" arg) to mount in the container.
	Volumes map[string]string
	// EnvironmentVars is a collection of name/values to pass as environment variables in the container.
	EnvironmentVars map[string]string
	// CommandArgs is the command and any additional arguments to execute in the container.
	CommandArgs []string
	// Entrypoint defines the entry point to use.
	Entrypoint []string
}

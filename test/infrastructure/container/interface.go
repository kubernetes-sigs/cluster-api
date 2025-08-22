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
	"fmt"
	"io"
	"net"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockersystem "github.com/docker/docker/api/types/system"
	"github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
)

// providerKey is the key type for accessing the runtime provider in passed contexts.
type providerKey struct{}

// Runtime defines the interface for interacting with a container runtime.
type Runtime interface {
	SaveContainerImage(ctx context.Context, image, dest string) error
	PullContainerImageIfNotExists(ctx context.Context, image string) error
	PullContainerImage(ctx context.Context, image string) error
	ImageExistsLocally(ctx context.Context, image string) (bool, error)
	GetHostPort(ctx context.Context, containerName, portAndProtocol string) (string, error)
	GetContainerIPs(ctx context.Context, containerName string) (string, string, error)
	ExecContainer(ctx context.Context, containerName string, config *ExecContainerInput, command string, args ...string) error
	RunContainer(ctx context.Context, runConfig *RunContainerInput, output io.Writer) error
	ListContainers(ctx context.Context, filters FilterBuilder) ([]Container, error)
	ContainerDebugInfo(ctx context.Context, containerName string, w io.Writer) error
	DeleteContainer(ctx context.Context, containerName string) error
	KillContainer(ctx context.Context, containerName, signal string) error
	GetSystemInfo(ctx context.Context) (dockersystem.Info, error)
}

// Mount contains mount details.
type Mount struct {
	// Source is the source host path to mount.
	Source string
	// Target is the path to mount in the container.
	Target string
	// ReadOnly specifies if the mount should be mounted read only.
	ReadOnly bool
}

// PortMapping contains port mapping information for the container.
type PortMapping struct {
	// ContainerPort is the port in the container to map to.
	ContainerPort int32
	// HostPort is the port to expose on the host.
	HostPort int32
	// ListenAddress is the address to bind to.
	ListenAddress string
	// Protocol is the protocol (tcp, udp, etc.) to use.
	Protocol string
}

// RunContainerInput holds the configuration settings for running a container.
type RunContainerInput struct {
	// Image is the name of the image to run.
	Image string
	// Name is the name to set for the container.
	Name string
	// Network is the name of the network to connect to.
	Network string
	// User is the user name to run as.
	User string
	// Group is the user group to run as.
	Group string
	// Volumes is a collection of any volumes (docker's "-v" arg) to mount in the container.
	Volumes map[string]string
	// Tmpfs is the temporary filesystem mounts to add.
	Tmpfs map[string]string
	// Mount contains mount information for the container.
	Mounts []Mount
	// EnvironmentVars is a collection of name/values to pass as environment variables in the container.
	EnvironmentVars map[string]string
	// CommandArgs is the command and any additional arguments to execute in the container.
	CommandArgs []string
	// Entrypoint defines the entry point to use.
	Entrypoint []string
	// Labels to apply to the container.
	Labels map[string]string
	// PortMappings contains host<>container ports to map.
	PortMappings []PortMapping
	// IPFamily is the IP version to use.
	IPFamily ClusterIPFamily
	// RestartPolicy to use for the container.
	// If not set, defaults to RestartPolicyUnlessStopped.
	RestartPolicy dockercontainer.RestartPolicyMode
	// Defines how the kindest/node image must be started.
	KindMode kind.Mode
}

// ExecContainerInput contains values for running exec on a container.
type ExecContainerInput struct {
	// OutputBuffer receives the stdout of the execution.
	OutputBuffer io.Writer
	// ErrorBuffer receives the stderr of the execution.
	ErrorBuffer io.Writer
	// InputBuffer contains stdin or nil if no input.
	InputBuffer io.Reader
	// EnvironmentVars is a collection of name=values to pass as environment variables in the container.
	EnvironmentVars []string
}

// FilterBuilder is a helper for building up filter strings of "key=value" or "key=name=value".
type FilterBuilder map[string]map[string][]string

// AddKeyValue adds a filter with a single name (--filter "label=io.x-k8s.kind.cluster").
func (f FilterBuilder) AddKeyValue(key, value string) {
	f.AddKeyNameValue(key, value, "")
}

// AddKeyNameValue adds a filter with a name=value (--filter "label=io.x-k8s.kind.cluster=quick-start-n95t5z").
func (f FilterBuilder) AddKeyNameValue(key, name, value string) {
	if _, ok := f[key]; !ok {
		f[key] = make(map[string][]string)
	}
	f[key][name] = append(f[key][name], value)
}

// Container represents a runtime container.
type Container struct {
	// Name is the name of the container
	Name string
	// Image is the name of the container's image
	Image string
	// Status is the status of the container
	Status string
}

// RuntimeFrom is used to extract the container runtime client from a
// context. If there is no runtime present, it will return nil.
func RuntimeFrom(ctx context.Context) (Runtime, error) {
	if provider, ok := ctx.Value(providerKey{}).(Runtime); ok {
		return provider, nil
	}
	return nil, fmt.Errorf("no container runtime client set for context")
}

// RuntimeInto is used to store the container runtime client into a
// context.
func RuntimeInto(ctx context.Context, runtime Runtime) context.Context {
	return context.WithValue(ctx, providerKey{}, runtime)
}

// GetClusterIPFamily returns a ClusterIPFamily based on the Cluster provided.
func GetClusterIPFamily(c *clusterv1.Cluster) (ClusterIPFamily, error) {
	podCIDRs := c.Spec.ClusterNetwork.Pods.CIDRBlocks
	serviceCIDRs := c.Spec.ClusterNetwork.Services.CIDRBlocks
	if len(podCIDRs) == 0 && len(serviceCIDRs) == 0 {
		return IPv4IPFamily, nil
	}

	podsIPFamily, err := ipFamilyForCIDRStrings(podCIDRs)
	if err != nil {
		return InvalidIPFamily, fmt.Errorf("pods: %s", err)
	}
	if len(serviceCIDRs) == 0 {
		return podsIPFamily, nil
	}

	servicesIPFamily, err := ipFamilyForCIDRStrings(serviceCIDRs)
	if err != nil {
		return InvalidIPFamily, fmt.Errorf("services: %s", err)
	}
	if len(podCIDRs) == 0 {
		return servicesIPFamily, nil
	}

	if podsIPFamily == DualStackIPFamily {
		return DualStackIPFamily, nil
	} else if podsIPFamily != servicesIPFamily {
		return InvalidIPFamily, errors.New("pods and services IP family mismatch")
	}

	return podsIPFamily, nil
}

func ipFamilyForCIDRStrings(cidrs []string) (ClusterIPFamily, error) {
	if len(cidrs) > 2 {
		return InvalidIPFamily, errors.New("too many CIDRs specified")
	}
	var foundIPv4 bool
	var foundIPv6 bool
	for _, cidr := range cidrs {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return InvalidIPFamily, fmt.Errorf("could not parse CIDR: %s", err)
		}
		if ip.To4() != nil {
			foundIPv4 = true
		} else {
			foundIPv6 = true
		}
	}
	switch {
	case foundIPv4 && foundIPv6:
		return DualStackIPFamily, nil
	case foundIPv4:
		return IPv4IPFamily, nil
	case foundIPv6:
		return IPv6IPFamily, nil
	default:
		return InvalidIPFamily, nil
	}
}

// ClusterIPFamily defines the types of supported IP families.
type ClusterIPFamily int

// Define the ClusterIPFamily constants.
const (
	InvalidIPFamily ClusterIPFamily = iota
	IPv4IPFamily
	IPv6IPFamily
	DualStackIPFamily
)

func (f ClusterIPFamily) String() string {
	return [...]string{"InvalidIPFamily", "IPv4IPFamily", "IPv6IPFamily", "DualStackIPFamily"}[f]
}

/*
Copyright 2020 The Kubernetes Authors.

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

package containers

//// CRIClient defines all behaviors CAPD needs to integrate with a container runtime.
//type Client interface {
//	ContainerList(ctx context.Context, options ListOptions) ([]*Container, error)
//	Exec(ctx context.Context, id string, config ExecConfig) (ExecResult, error)
//	Info(ctx context.Context) (*Info, error)
//	Inspect(ctx context.Context, id string) (*Inspection, error)
//	Kill(ctx context.Context, id, signal string) error
//	NetworkInspect(ctx context.Context, networkID string) (*NetworkResource, error)
//	Remove(ctx context.Context, id string) error
//	Run(ctx context.Context, runConfig RunConfig, hostConfig HostConfig, name string) (string, error)
//}

type Container struct {
	ID         string `json:"Id"`
	Names      []string
	Image      string
	ImageID    string
	Command    string
	Created    int64
	Ports      []Port
	SizeRw     int64 `json:",omitempty"`
	SizeRootFs int64 `json:",omitempty"`
	Labels     map[string]string
	State      string
	Status     string
	HostConfig struct {
		NetworkMode string `json:",omitempty"`
	}
	NetworkSettings *SummaryNetworkSettings
}

type SummaryNetworkSettings struct {
	Networks map[string]*EndpointSettings
}

// EndpointSettings stores the network endpoint details
type EndpointSettings struct {
	// Configurations
	IPAMConfig *EndpointIPAMConfig
}

type EndpointIPAMConfig struct {
	IPv4Address  string   `json:",omitempty"`
	IPv6Address  string   `json:",omitempty"`
	LinkLocalIPs []string `json:",omitempty"`
}

// ExecConfig is used to configure an exec call within a container.
// This is used as a common struct for the CRI interface within this repo.
// ExecConfig does not support stdin, use bindmounts to write files
type ExecConfig struct {
	User       string   // User that will run the command
	Privileged bool     // Is the container in privileged mode
	Tty        bool     // Attach standard streams to a tty.
	Detach     bool     // Execute in detach mode
	Env        []string // Environment variables
	WorkingDir string   // Working directory
	Cmd        []string // Execution commands and args
}

type ExecResult struct {
	ExitCode int
	StdOut   string
	StdErr   string
}

type ListOptions struct {
	All     bool
	Filters map[string][]string
}

type Info struct {
	SecurityOptions []string
}

type NetworkResource struct {
	IPAM IPAM // IPAM is the network's IP Address Management
}

// IPAM represents IP Address Management
type IPAM struct {
	Config []IPAMConfig
}

// IPAMConfig represents IPAM configurations
type IPAMConfig struct {
	Subnet string `json:",omitempty"`
}

type RunConfig struct {
	ExposedPorts []string `json:",omitempty"` // List of exposed ports
	Env          []string // List of environment variable to set in the container
	Cmd          []string // Command to run when starting the container
	// TODO consider adding healthcheck
	Image   string              // Name of the image as it was passed by the operator (e.g. could be symbolic)
	Volumes map[string]struct{} // List of volumes (mounts) used for the container
	Labels  map[string]string   // List of labels set to this container
}

type HostConfig struct {
	Privileged   bool                     // Is the container in privileged mode
	Binds        []string                 // List of volume bindings for this container
	NetworkMode  string                   // Network mode to use for the container
	PortBindings map[string][]PortBinding // Port mapping between the exposed port (container) and the host

	// Applicable to UNIX platforms
	SecurityOpt []string          // List of string values to customize labels for MLS systems, such as SELinux.
	Tmpfs       map[string]string `json:",omitempty"` // List of tmpfs (mounts) used for the container
	UsernsMode  string            // The user namespace to use for the container
}

// PortBinding represents a binding between a Host IP address and a Host Port
type PortBinding struct {
	// HostIP is the host IP Address
	HostIP string `json:"HostIp"`
	// HostPort is the host port number
	HostPort string
}

type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

type Inspection struct {
	IPv4, IPv6 string
}

type Mount struct {
	Source      string
	Destination string
}

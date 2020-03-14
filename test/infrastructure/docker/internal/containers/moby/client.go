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

package client

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/containers"
)

type Moby struct {
	Client *client.Client
}

func NewMoby() (*Moby, error) {
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &Moby{
		Client: c,
	}, nil
}

func (m *Moby) Info(ctx context.Context) (*containers.Info, error) {
	info, err := m.Client.Info(ctx)
	if err != nil {
		return nil, err
	}
	return &containers.Info{SecurityOptions: info.SecurityOptions}, nil
}

func (m *Moby) Inspect(ctx context.Context, id string) (*containers.Inspection, error) {
	resp, err := m.Client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to inspect container: %s", id)
	}
	inspection := &containers.Inspection{}
	inspection.IPv4 = resp.NetworkSettings.IPAddress
	inspection.IPv6 = resp.NetworkSettings.GlobalIPv6Address
	return inspection, nil
}

func (m *Moby) Remove(ctx context.Context, id string) error {
	options := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   false,
		Force:         true,
	}
	return m.Client.ContainerRemove(ctx, id, options)
}

func (m *Moby) Kill(ctx context.Context, id, signal string) error {
	return m.Client.ContainerKill(ctx, id, signal)
}

func (m *Moby) WriteFile(ctx context.Context, id, file string, contents io.Reader) error {
	tarFile, err := tarFile(file, contents)
	if err != nil {
		return err
	}
	return m.Client.CopyToContainer(ctx, id, "/tmp", bytes.NewReader(tarFile), types.CopyToContainerOptions{})
}

func (m *Moby) Run(ctx context.Context, containerConfig containers.RunConfig, hostConfig containers.HostConfig, name string) (string, error) {
	c := containerConfigToMobyContainerConfig(containerConfig)
	hc := hostConfigToMobyHostConfig(hostConfig)
	hc.Binds = append(hc.Binds, "/tmp/")
	networkingConfig := &network.NetworkingConfig{}
	body, err := m.Client.ContainerCreate(ctx, c, hc, networkingConfig, name)
	if err != nil {
		return "", errors.Wrap(err, "failed to create container")
	}
	if err := m.Client.ContainerStart(ctx, body.ID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}
	return body.ID, nil
}

func (m *Moby) Exec(ctx context.Context, id string, cfg containers.ExecConfig) (containers.ExecResult, error) {
	mobyExecConfig := types.ExecConfig{
		User:         cfg.User,
		Privileged:   cfg.Privileged,
		Tty:          cfg.Tty,
		Detach:       cfg.Detach,
		Env:          cfg.Env,
		WorkingDir:   cfg.WorkingDir,
		Cmd:          cfg.Cmd,
		AttachStderr: true,
		AttachStdout: true,
	}
	result := containers.ExecResult{}
	execID, err := m.Client.ContainerExecCreate(ctx, id, mobyExecConfig)
	if err != nil {
		return result, errors.Wrapf(err, "failed to create exec config to run a command in container: %v", id)
	}

	resp, err := m.Client.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return result, err
	}
	defer resp.Close()

	// read the output
	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error)

	go func() {
		// StdCopy demultiplexes the stream into two buffers
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return result, err
		}
		break

	case <-ctx.Done():
		return result, ctx.Err()
	}

	stdout, err := ioutil.ReadAll(&outBuf)
	if err != nil {
		return result, err
	}
	stderr, err := ioutil.ReadAll(&errBuf)
	if err != nil {
		return result, err
	}

	res, err := m.Client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return result, err
	}

	result.ExitCode = res.ExitCode
	result.StdOut = string(stdout)
	result.StdErr = string(stderr)
	return result, nil
}

func (m *Moby) ContainerList(ctx context.Context, options containers.ListOptions) ([]*containers.Container, error) {
	mobyOptions := types.ContainerListOptions{
		All:     options.All,
		Filters: argsToMobyArgs(options.Filters),
	}
	c, err := m.Client.ContainerList(ctx, mobyOptions)
	return mobyContainersToContainers(c), err
}

func (m *Moby) NetworkInspect(ctx context.Context, networkID string) (*containers.NetworkResource, error) {
	opts := types.NetworkInspectOptions{}
	resource, err := m.Client.NetworkInspect(ctx, networkID, opts)
	if err != nil {
		return nil, err
	}
	return mobyNetworkResourceToNetworkResource(resource), nil
}

func mobyNetworkResourceToNetworkResource(resource types.NetworkResource) *containers.NetworkResource {
	ipams := make([]containers.IPAMConfig, len(resource.IPAM.Config))
	for i := range resource.IPAM.Config {
		ipams[i] = containers.IPAMConfig{Subnet: resource.IPAM.Config[i].Subnet}
	}
	return &containers.NetworkResource{
		IPAM: containers.IPAM{
			Config: ipams,
		},
	}
}

func argsToMobyArgs(args map[string][]string) filters.Args {
	mobyArgs := filters.NewArgs()
	for k, v := range args {
		for _, v2 := range v {
			mobyArgs.Add(k, v2)
		}
	}
	return mobyArgs
}

func mobyContainersToContainers(in []types.Container) []*containers.Container {
	out := make([]*containers.Container, len(in))
	for i, c := range in {
		out[i] = &containers.Container{
			ID:              c.ID,
			Names:           c.Names,
			Image:           c.Image,
			ImageID:         c.ImageID,
			Command:         c.Command,
			Created:         c.Created,
			Ports:           mobyPortsToPorts(c.Ports),
			SizeRw:          c.SizeRw,
			SizeRootFs:      c.SizeRootFs,
			Labels:          c.Labels,
			State:           c.State,
			Status:          c.Status,
			HostConfig:      c.HostConfig,
			NetworkSettings: mobyNetworkSettingsToNetworkSettings(c.NetworkSettings),
			// Ignoring mount point...
		}
	}
	return out
}

func mobyPortsToPorts(in []types.Port) []containers.Port {
	out := make([]containers.Port, len(in))
	for i, p := range in {
		out[i] = containers.Port(p)
	}
	return out
}

func mobyNetworkSettingsToNetworkSettings(in *types.SummaryNetworkSettings) *containers.SummaryNetworkSettings {
	networks := map[string]*containers.EndpointSettings{}
	for k, v := range in.Networks {
		if v == nil {
			continue
		}
		networks[k] = mobyEndpointSettingsToEndpointSettings(v)
	}
	return &containers.SummaryNetworkSettings{Networks: networks}
}

func mobyEndpointSettingsToEndpointSettings(in *network.EndpointSettings) *containers.EndpointSettings {
	if in.IPAMConfig == nil {
		return nil
	}
	return &containers.EndpointSettings{
		IPAMConfig: mobyIPAMConfigToIPAMConfig(in.IPAMConfig),
	}
}

func mobyIPAMConfigToIPAMConfig(in *network.EndpointIPAMConfig) *containers.EndpointIPAMConfig {
	return &containers.EndpointIPAMConfig{
		IPv4Address:  in.IPv4Address,
		IPv6Address:  in.IPv6Address,
		LinkLocalIPs: in.LinkLocalIPs,
	}
}

func containerConfigToMobyContainerConfig(in containers.RunConfig) *container.Config {
	return &container.Config{
		Env:     in.Env,
		Cmd:     in.Cmd,
		Image:   in.Image,
		Volumes: in.Volumes,
		Labels:  in.Labels,
	}
}

func hostConfigToMobyHostConfig(in containers.HostConfig) *container.HostConfig {
	return &container.HostConfig{
		Binds:        in.Binds,
		NetworkMode:  container.NetworkMode(in.NetworkMode),
		PortBindings: portBindingsToNatPortSet(in.PortBindings),
		SecurityOpt:  in.SecurityOpt,
		Tmpfs:        in.Tmpfs,
		UsernsMode:   container.UsernsMode(in.UsernsMode),
	}
}

//
func portBindingsToNatPortSet(in map[string][]containers.PortBinding) nat.PortMap {
	out := nat.PortMap{}
	for k, v := range in {
		out[nat.Port(k)] = portBindingsToMobyPortBindings(v)
	}
	return out
}

func portBindingsToMobyPortBindings(in []containers.PortBinding) []nat.PortBinding {
	out := []nat.PortBinding{}
	for _, v := range in {
		out = append(out, nat.PortBinding{
			HostIP:   v.HostIP,
			HostPort: v.HostPort,
		})
	}
	return out
}

func tarFile(path string, contents io.Reader) ([]byte, error) {
	data, err := ioutil.ReadAll(contents)
	if err != nil {
		return nil, err
	}
	// Create and add some files to the archive.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var files = []struct {
		Name, Body string
	}{
		{path, string(data)},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Mode: 0600,
			Size: int64(len(file.Body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

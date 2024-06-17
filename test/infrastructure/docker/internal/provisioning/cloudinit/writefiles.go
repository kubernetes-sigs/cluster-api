/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	kubeadmInitPath          = "/run/kubeadm/kubeadm.yaml"
	kubeadmJoinPath          = "/run/kubeadm/kubeadm-join-config.yaml"
	kubeproxyComponentConfig = `
---
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
conntrack:
# Skip setting sysctl value "net.netfilter.nf_conntrack_max"
# It is a global variable that affects other namespaces
  maxPerCore: 0
`
)

var (
	cgroupDriverCgroupfs            = "cgroupfs"
	cgroupDriverPatchVersionCeiling = semver.Version{Major: 1, Minor: 24}
)

// writeFilesAction defines a list of files that should be written to a node.
type writeFilesAction struct {
	Files []files `json:"write_files,"`
}

type files struct {
	Path        string `json:"path,"`
	Encoding    string `json:"encoding,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Permissions string `json:"permissions,omitempty"`
	Content     string `json:"content,"`
	Append      bool   `json:"append,"`
}

func newWriteFilesAction() action {
	return &writeFilesAction{}
}

func (a *writeFilesAction) Unmarshal(userData []byte, kindMapping kind.Mapping) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing write_files action: %s", userData)
	}
	for i, f := range a.Files {
		if f.Path == kubeadmInitPath {
			// NOTE: in case of init the kubeadmConfigFile contains both the ClusterConfiguration and the InitConfiguration
			contentSplit := strings.Split(f.Content, "---\n")

			if len(contentSplit) != 3 {
				return errors.Errorf("invalid kubeadm config file, unable to parse it")
			}
			clusterConfiguration := &bootstrapv1.ClusterConfiguration{}
			initConfiguration, err := kubeadmtypes.UnmarshalInitConfiguration(contentSplit[2], clusterConfiguration)
			if err != nil {
				return errors.Wrapf(err, "failed to parse init configuration")
			}

			fixNodeRegistration(&initConfiguration.NodeRegistration, kindMapping)

			contentSplit[2], err = kubeadmtypes.MarshalInitConfigurationForVersion(clusterConfiguration, initConfiguration, kindMapping.KubernetesVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal init configuration")
			}
			a.Files[i].Content = strings.Join(contentSplit, "---\n")
		}
		if f.Path == kubeadmJoinPath {
			// NOTE: in case of join the kubeadmConfigFile contains only the join Configuration
			clusterConfiguration := &bootstrapv1.ClusterConfiguration{}
			joinConfiguration, err := kubeadmtypes.UnmarshalJoinConfiguration(f.Content, clusterConfiguration)
			if err != nil {
				return errors.Wrapf(err, "failed to parse join configuration")
			}

			fixNodeRegistration(&joinConfiguration.NodeRegistration, kindMapping)

			a.Files[i].Content, err = kubeadmtypes.MarshalJoinConfigurationForVersion(clusterConfiguration, joinConfiguration, kindMapping.KubernetesVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to marshal join configuration")
			}
		}
	}
	return nil
}

// fixNodeRegistration sets node registration for running Kubernetes/kubelet in docker.
// NOTE: we add those values if they do not exists; user can set those flags to different values to disable automatic fixing.
// NOTE: if there will be use case for it, we might investigate better ways to disable automatic fixing.
func fixNodeRegistration(nodeRegistration *bootstrapv1.NodeRegistrationOptions, kindMapping kind.Mapping) {
	if nodeRegistration.CRISocket == "" {
		// NOTE: self-hosted cluster have to mount the Docker socket.
		// On those nodes we have the Docker and the containerd socket and then kubeadm
		// wouldn't know which one to use unless we are explicit about it.
		nodeRegistration.CRISocket = "unix:///var/run/containerd/containerd.sock"
	}

	if nodeRegistration.KubeletExtraArgs == nil {
		nodeRegistration.KubeletExtraArgs = map[string]string{}
	}

	if _, ok := nodeRegistration.KubeletExtraArgs["eviction-hard"]; !ok {
		nodeRegistration.KubeletExtraArgs["eviction-hard"] = "nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%"
	}
	if _, ok := nodeRegistration.KubeletExtraArgs["fail-swap-on"]; !ok {
		nodeRegistration.KubeletExtraArgs["fail-swap-on"] = "false"
	}

	if kindMapping.Mode != kind.Mode0_19 {
		// kindest/node images generated by modo 0.20 and greater require to use CgroupnsMode = "private" when running images;
		// following settings are complementary to this change.

		if _, ok := nodeRegistration.KubeletExtraArgs["cgroup-root"]; !ok {
			nodeRegistration.KubeletExtraArgs["cgroup-root"] = "/kubelet"
		}
		if _, ok := nodeRegistration.KubeletExtraArgs["runtime-cgroups"]; !ok {
			nodeRegistration.KubeletExtraArgs["runtime-cgroups"] = "/system.slice/containerd.service"
		}
	}

	if version.Compare(kindMapping.KubernetesVersion, cgroupDriverPatchVersionCeiling) == -1 {
		// kubeadm for Kubernetes version <= 1.23 defaults to cgroup-driver=cgroupfs; following settings makes kubelet
		// to run consistently with what kubeadm expects.
		nodeRegistration.KubeletExtraArgs["cgroup-driver"] = cgroupDriverCgroupfs
	}
}

// Commands return a list of commands to run on the node.
// Each command defines the parameters of a shell command necessary to generate a file replicating the cloud-init write_files module.
func (a *writeFilesAction) Commands() ([]provisioning.Cmd, error) {
	commands := make([]provisioning.Cmd, 0)
	for _, f := range a.Files {
		// Fix attributes and apply defaults
		path := fixPath(f.Path) // NB. the real cloud init module for writes files converts path into absolute paths; this is not possible here...
		encodings := fixEncoding(f.Encoding)
		owner := fixOwner(f.Owner)
		permissions := fixPermissions(f.Permissions)
		content, err := fixContent(f.Content, encodings)
		if path == kubeadmInitPath {
			content += kubeproxyComponentConfig
		}
		if err != nil {
			return commands, errors.Wrapf(err, "error decoding content for %s", path)
		}

		// Make the directory so cat + redirection will work
		directory := filepath.Dir(path)
		commands = append(commands, provisioning.Cmd{Cmd: "mkdir", Args: []string{"-p", directory}})

		redirects := ">"
		if f.Append {
			redirects = ">>"
		}

		// generate a command that will create a file with the expected contents.
		commands = append(commands, provisioning.Cmd{Cmd: "/bin/sh", Args: []string{"-c", fmt.Sprintf("cat %s %s /dev/stdin", redirects, path)}, Stdin: content})

		// if permissions are different than default ownership, add a command to modify the permissions.
		if permissions != "0644" {
			commands = append(commands, provisioning.Cmd{Cmd: "chmod", Args: []string{permissions, path}})
		}

		// if ownership is different than default ownership, add a command to modify file ownerhsip.
		if owner != "root:root" {
			commands = append(commands, provisioning.Cmd{Cmd: "chown", Args: []string{owner, path}})
		}
	}
	return commands, nil
}

func fixPath(p string) string {
	return strings.TrimSpace(p)
}

func fixOwner(o string) string {
	o = strings.TrimSpace(o)
	if o != "" {
		return o
	}
	return "root:root"
}

func fixPermissions(p string) string {
	p = strings.TrimSpace(p)
	if p != "" {
		return p
	}
	return "0644"
}

func fixEncoding(e string) []string {
	e = strings.ToLower(e)
	e = strings.TrimSpace(e)

	switch e {
	case "gz", "gzip":
		return []string{"application/x-gzip"}
	case "gz+base64", "gzip+base64", "gz+b64", "gzip+b64":
		return []string{"application/base64", "application/x-gzip"}
	case "base64", "b64":
		return []string{"application/base64"}
	}

	return []string{"text/plain"}
}

func fixContent(content string, encodings []string) (string, error) {
	for _, e := range encodings {
		switch e {
		case "application/base64":
			rByte, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				return content, errors.WithStack(err)
			}
			return string(rByte), nil
		case "application/x-gzip":
			rByte, err := gUnzipData([]byte(content))
			if err != nil {
				return content, err
			}
			return string(rByte), nil
		case "text/plain":
			return content, nil
		default:
			return content, errors.Errorf("Unknown bootstrap data encoding: %q", content)
		}
	}
	return content, nil
}

func gUnzipData(data []byte) ([]byte, error) {
	var r io.Reader
	var err error
	b := bytes.NewBuffer(data)
	r, err = gzip.NewReader(b)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resB.Bytes(), nil
}

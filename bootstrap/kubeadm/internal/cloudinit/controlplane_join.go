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
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/secret"
)

const (
	controlPlaneJoinCloudInit = `{{.Header}}
{{template "files" .WriteFiles}}
-   path: /run/kubeadm/kubeadm-join-config.yaml
    owner: root:root
    permissions: '0640'
    content: |
{{.JoinConfiguration | Indent 6}}
runcmd:
{{- template "commands" .PreKubeadmCommands }}
  - {{ .KubeadmCommand }}
{{- template "commands" .PostKubeadmCommands }}
{{- template "ntp" .NTP }}
{{- template "users" .Users }}
{{- template "disk_setup" .DiskSetup}}
{{- template "fs_setup" .DiskSetup}}
{{- template "mounts" .Mounts}}
`
)

// ControlPlaneJoinInput defines context to generate controlplane instance user data for control plane node join.
type ControlPlaneJoinInput struct {
	BaseUserData
	secret.Certificates
	BootstrapToken    string
	JoinConfiguration string
}

// NewJoinControlPlane returns the user data string to be used on a new control plane instance.
func NewJoinControlPlane(input *ControlPlaneJoinInput) ([]byte, error) {
	// TODO: Consider validating that the correct certificates exist. It is different for external/stacked etcd
	input.WriteFiles = input.Certificates.AsFiles()
	input.ControlPlane = true
	if err := input.prepare(); err != nil {
		return nil, err
	}
	userData, err := generate("JoinControlplane", controlPlaneJoinCloudInit, input)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate user data for machine joining control plane")
	}

	return userData, err
}

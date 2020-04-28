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

const (
	nodeCloudInit = `{{.Header}}
{{template "files" .WriteFiles}}
-   path: /tmp/kubeadm-join-config.yaml
    owner: root:root
    permissions: '0640'
    content: |
      ---
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

// NodeInput defines the context to generate a node user data.
type NodeInput struct {
	BaseUserData
	JoinConfiguration string
}

// NewNode returns the user data string to be used on a node instance.
func NewNode(input *NodeInput) ([]byte, error) {
	if err := input.prepare(); err != nil {
		return nil, err
	}
	input.Header = cloudConfigHeader
	input.WriteFiles = append(input.WriteFiles, input.AdditionalFiles...)
	return generate("Node", nodeCloudInit, input)
}

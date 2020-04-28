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
	"fmt"
	"text/template"

	"github.com/pkg/errors"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
)

const (
	standardJoinCommand            = "kubeadm join --config /tmp/kubeadm-join-config.yaml %s"
	retriableJoinScriptName        = "/usr/local/bin/kubeadm-bootstrap-script"
	retriableJoinScriptOwner       = "root"
	retriableJoinScriptPermissions = "0755"
	cloudConfigHeader              = `## template: jinja
#cloud-config
`
)

// BaseUserData is shared across all the various types of files written to disk.
type BaseUserData struct {
	Header               string
	PreKubeadmCommands   []string
	PostKubeadmCommands  []string
	AdditionalFiles      []bootstrapv1.File
	WriteFiles           []bootstrapv1.File
	Users                []bootstrapv1.User
	NTP                  *bootstrapv1.NTP
	DiskSetup            *bootstrapv1.DiskSetup
	Mounts               []bootstrapv1.MountPoints
	ControlPlane         bool
	UseExperimentalRetry bool
	KubeadmCommand       string
	KubeadmVerbosity     string
}

func (input *BaseUserData) prepare() error {
	input.Header = cloudConfigHeader
	input.WriteFiles = append(input.WriteFiles, input.AdditionalFiles...)
	input.KubeadmCommand = fmt.Sprintf(standardJoinCommand, input.KubeadmVerbosity)
	if input.UseExperimentalRetry {
		input.KubeadmCommand = retriableJoinScriptName
		joinScriptFile, err := generateBootstrapScript(input)
		if err != nil {
			return errors.Wrap(err, "failed to generate user data for machine joining control plane")
		}
		input.WriteFiles = append(input.WriteFiles, *joinScriptFile)
	}
	return nil
}

func generate(kind string, tpl string, data interface{}) ([]byte, error) {
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	if _, err := tm.Parse(filesTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse files template")
	}

	if _, err := tm.Parse(commandsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse commands template")
	}

	if _, err := tm.Parse(ntpTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse ntp template")
	}

	if _, err := tm.Parse(usersTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse users template")
	}

	if _, err := tm.Parse(diskSetupTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse users template")
	}

	if _, err := tm.Parse(fsSetupTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse users template")
	}

	if _, err := tm.Parse(mountsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse users template")
	}

	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	return out.Bytes(), nil
}

func generateBootstrapScript(input interface{}) (*bootstrapv1.File, error) {
	scriptBytes, err := bootstrapKubeadmInternalCloudinitKubeadmBootstrapScriptShBytes()
	if err != nil {
		return nil, errors.Wrap(err, "couldn't read bootstrap script")
	}
	joinScript, err := generate("JoinScript", string(scriptBytes), input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to bootstrap script for machine joins")
	}
	return &bootstrapv1.File{
		Path:        retriableJoinScriptName,
		Owner:       retriableJoinScriptOwner,
		Permissions: retriableJoinScriptPermissions,
		Content:     string(joinScript),
	}, nil
}

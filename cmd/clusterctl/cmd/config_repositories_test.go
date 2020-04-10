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

package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_runGetRepositories(t *testing.T) {
	t.Run("prints output", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir, err := ioutil.TempDir("", "cc")
		g.Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		path := filepath.Join(tmpDir, "clusterctl.yaml")
		g.Expect(ioutil.WriteFile(path, []byte(template), 0644)).To(Succeed())

		buf := bytes.NewBufferString("")
		g.Expect(runGetRepositories(path, buf)).To(Succeed())

		out, err := ioutil.ReadAll(buf)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(out)).To(Equal(expectedOutput))
	})

	t.Run("returns error for bad cfgFile path", func(t *testing.T) {
		g := NewWithT(t)
		buf := bytes.NewBufferString("")
		g.Expect(runGetRepositories("do-not-exist", buf)).ToNot(Succeed())
	})

	t.Run("returns error for nil writer", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(runGetRepositories("do-exist", nil)).ToNot(Succeed())
	})

	t.Run("returns error for bad template", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir, err := ioutil.TempDir("", "cc")
		g.Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		path := filepath.Join(tmpDir, "clusterctl.yaml")
		g.Expect(ioutil.WriteFile(path, []byte("providers: foobar"), 0644)).To(Succeed())

		buf := bytes.NewBufferString("")
		g.Expect(runGetRepositories(path, buf)).ToNot(Succeed())
	})
}

var template = `---
providers:
  # add a custom provider
  - name: "my-infra-provider"
    url: "/home/.cluster-api/overrides/infrastructure-docker/latest/infrastructure-components.yaml"
    type: "InfrastructureProvider"
  # add a custom provider
  - name: "another-provider"
    url: "./bootstrap-components.yaml"
    type: "BootstrapProvider"
  # bad url
  - name: "aws"
    url: "my-aws-infrastructure-components.yaml"
    type: "InfrastructureProvider"
  # override a pre-defined provider
  - name: "cluster-api"
    url: "https://github.com/myorg/myforkofclusterapi/releases/latest/core_components.yaml"
    type: "CoreProvider"
`

var expectedOutput = `NAME                TYPE                     URL                                                                                  FILE
cluster-api         CoreProvider             https://github.com/myorg/myforkofclusterapi/releases/latest/                         core_components.yaml
another-provider    BootstrapProvider        ./                                                                                   bootstrap-components.yaml
kubeadm             BootstrapProvider        https://github.com/kubernetes-sigs/cluster-api/releases/latest/                      bootstrap-components.yaml
kubeadm             ControlPlaneProvider     https://github.com/kubernetes-sigs/cluster-api/releases/latest/                      control-plane-components.yaml
aws                 InfrastructureProvider                                                                                        my-aws-infrastructure-components.yaml
azure               InfrastructureProvider   https://github.com/kubernetes-sigs/cluster-api-provider-azure/releases/latest/       infrastructure-components.yaml
metal3              InfrastructureProvider   https://github.com/metal3-io/cluster-api-provider-metal3/releases/latest/            infrastructure-components.yaml
my-infra-provider   InfrastructureProvider   /home/.cluster-api/overrides/infrastructure-docker/latest/                           infrastructure-components.yaml
openstack           InfrastructureProvider   https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases/latest/   infrastructure-components.yaml
vsphere             InfrastructureProvider   https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/     infrastructure-components.yaml
`

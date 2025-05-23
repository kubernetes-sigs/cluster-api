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
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestNewInitControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:              "test",
			BootCommands:        nil,
			PreKubeadmCommands:  nil,
			PostKubeadmCommands: nil,
			AdditionalFiles: []bootstrapv1.File{
				{
					Path:     "/tmp/my-path",
					Encoding: bootstrapv1.Base64,
					Content:  "aGk=",
				},
				{
					Path:    "/tmp/my-other-path",
					Content: "hi",
				},
				{
					Path:    "/tmp/existing-path",
					Append:  true,
					Content: "hi",
				},
			},
			WriteFiles: nil,
			Users:      nil,
			NTP:        nil,
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedFiles := []string{
		`-   path: /tmp/my-path
    encoding: "base64"
    content: |
      aGk=`,
		`-   path: /tmp/my-other-path
    content: |
      hi`,
		`-   path: /tmp/existing-path
    append: true
    content: |
      hi`,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewInitControlPlaneCommands(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:              "test",
			BootCommands:        []string{"echo $(date)", "echo 'hello BootCommands!'"},
			PreKubeadmCommands:  []string{`"echo $(date) ': hello PreKubeadmCommands!'"`},
			PostKubeadmCommands: []string{"echo $(date) ': hello PostKubeadmCommands!'"},
			AdditionalFiles:     nil,
			WriteFiles:          nil,
			Users:               nil,
			NTP:                 nil,
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedBootCmd := `bootcmd:
  - "echo $(date)"
  - "echo 'hello BootCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedBootCmd))

	expectedRunCmd := `runcmd:
  - "\"echo $(date) ': hello PreKubeadmCommands!'\""
  - 'kubeadm init --config /run/kubeadm/kubeadm.yaml  && echo success > /run/cluster-api/bootstrap-success.complete'
  - "echo $(date) ': hello PostKubeadmCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedRunCmd))
}

func TestNewInitControlPlaneDiskMounts(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:              "test",
			BootCommands:        nil,
			PreKubeadmCommands:  nil,
			PostKubeadmCommands: nil,
			WriteFiles:          nil,
			Users:               nil,
			NTP:                 nil,
			DiskSetup: &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "test-device",
						Layout:    true,
						Overwrite: ptr.To(false),
						TableType: ptr.To("gpt"),
					},
				},
				Filesystems: []bootstrapv1.Filesystem{
					{
						Device:     "test-device",
						Filesystem: "ext4",
						Label:      "test_disk",
						ExtraOpts:  []string{"-F", "-E", "lazy_itable_init=1,lazy_journal_init=1"},
					},
				},
			},
			Mounts: []bootstrapv1.MountPoints{
				{"test_disk", "/var/lib/testdir"},
			},
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedDiskSetup := `disk_setup:
  test-device:
    table_type: gpt
    layout: true
    overwrite: false`
	expectedFSSetup := `fs_setup:
  - label: test_disk
    filesystem: ext4
    device: test-device
    extra_opts:
      - -F
      - -E
      - lazy_itable_init=1,lazy_journal_init=1`
	expectedMounts := `mounts:
  - - test_disk
    - /var/lib/testdir`

	g.Expect(string(out)).To(ContainSubstring(expectedDiskSetup))
	g.Expect(string(out)).To(ContainSubstring(expectedFSSetup))
	g.Expect(string(out)).To(ContainSubstring(expectedMounts))
}

func TestNewJoinControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
			BootCommands:        nil,
			Header:              "test",
			PreKubeadmCommands:  nil,
			PostKubeadmCommands: nil,
			AdditionalFiles: []bootstrapv1.File{
				{
					Path:     "/tmp/my-path",
					Encoding: bootstrapv1.Base64,
					Content:  "aGk=",
				},
				{
					Path:    "/tmp/my-other-path",
					Content: "hi",
				},
			},
			WriteFiles: nil,
			Users:      nil,
			NTP:        nil,
		},
		Certificates:      secret.Certificates{},
		BootstrapToken:    "my-bootstrap-token",
		JoinConfiguration: "my-join-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewJoinControlPlane(cpinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedFiles := []string{
		`-   path: /tmp/my-path
    encoding: "base64"
    content: |
      aGk=`,
		`-   path: /tmp/my-other-path
    content: |
      hi`,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewJoinControlPlaneCommands(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
			Header:              "test",
			BootCommands:        []string{"echo $(date)", "echo 'hello BootCommands!'"},
			PreKubeadmCommands:  []string{`"echo $(date) ': hello PreKubeadmCommands!'"`},
			PostKubeadmCommands: []string{"echo $(date) ': hello PostKubeadmCommands!'"},
			AdditionalFiles:     nil,
			WriteFiles:          nil,
			Users:               nil,
			NTP:                 nil,
		},
		Certificates:      secret.Certificates{},
		JoinConfiguration: "my-join-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewJoinControlPlane(cpinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedBootCmd := `bootcmd:
  - "echo $(date)"
  - "echo 'hello BootCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedBootCmd))

	expectedRunCmd := `runcmd:
  - "\"echo $(date) ': hello PreKubeadmCommands!'\""
  - kubeadm join --config /run/kubeadm/kubeadm-join-config.yaml  && echo success > /run/cluster-api/bootstrap-success.complete
  - "echo $(date) ': hello PostKubeadmCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedRunCmd))
}

func TestNewJoinNodeCommands(t *testing.T) {
	g := NewWithT(t)

	nodeinput := &NodeInput{
		BaseUserData: BaseUserData{
			Header:              "test",
			BootCommands:        []string{"echo $(date)", "echo 'hello BootCommands!'"},
			PreKubeadmCommands:  []string{`"echo $(date) ': hello PreKubeadmCommands!'"`},
			PostKubeadmCommands: []string{"echo $(date) ': hello PostKubeadmCommands!'"},
			AdditionalFiles:     nil,
			WriteFiles:          nil,
			Users:               nil,
			NTP:                 nil,
		},
		JoinConfiguration: "my-join-config",
	}

	out, err := NewNode(nodeinput)
	g.Expect(err).ToNot(HaveOccurred())

	expectedBootCmd := `bootcmd:
  - "echo $(date)"
  - "echo 'hello BootCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedBootCmd))

	expectedRunCmd := `runcmd:
  - "\"echo $(date) ': hello PreKubeadmCommands!'\""
  - kubeadm join --config /run/kubeadm/kubeadm-join-config.yaml  && echo success > /run/cluster-api/bootstrap-success.complete
  - "echo $(date) ': hello PostKubeadmCommands!'"`

	g.Expect(out).To(ContainSubstring(expectedRunCmd))
}

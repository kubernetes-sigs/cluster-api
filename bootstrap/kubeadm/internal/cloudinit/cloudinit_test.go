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
					Append:  ptr.To(true),
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

func TestNewInitControlPlaneDiskSetup(t *testing.T) {
	tests := []struct {
		name              string
		diskSetup         *bootstrapv1.DiskSetup
		mounts            []bootstrapv1.MountPoints
		expectedDiskSetup string
		expectedFSSetup   string
		expectedMounts    string
	}{
		{
			name: "Disk setup with partitions, filesystems and mounts",
			diskSetup: &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "test-device",
						Layout:    ptr.To(true),
						Overwrite: ptr.To(false),
						TableType: "gpt",
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
			mounts: []bootstrapv1.MountPoints{
				{"test_disk", "/var/lib/testdir"},
			},
			expectedDiskSetup: `disk_setup:
  test-device:
    table_type: gpt
    layout: true
    overwrite: false`,
			expectedFSSetup: `fs_setup:
  - label: test_disk
    filesystem: ext4
    device: test-device
    extra_opts:
      - -F
      - -E
      - lazy_itable_init=1,lazy_journal_init=1`,
			expectedMounts: `mounts:
  - - test_disk
    - /var/lib/testdir`,
		},
		{
			name: "Disk setup with DiskLayout",
			diskSetup: &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "test-device",
						TableType: "gpt",
						DiskLayout: []bootstrapv1.PartitionSpec{
							{
								Percentage:    30,
								PartitionType: bootstrapv1.PartitionTypeLinux,
							},
							{
								Percentage: 20,
							},
							{
								Percentage:    50,
								PartitionType: bootstrapv1.PartitionTypeLinuxRAID,
							},
						},
					},
				},
			},
			expectedDiskSetup: `disk_setup:
  test-device:
    table_type: gpt
    layout:
      - [30, 83]
      - 20
      - [50, fd]`,
		},
		{
			name: "Disk setup with DiskLayout user-friendly partition type literal",
			diskSetup: &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "test-device",
						TableType: "gpt",
						DiskLayout: []bootstrapv1.PartitionSpec{
							{
								Percentage:    100,
								PartitionType: "LinuxSwap",
							},
						},
					},
				},
			},
			expectedDiskSetup: `disk_setup:
  test-device:
    table_type: gpt
    layout:
      - [100, 82]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cpinput := &ControlPlaneInput{
				BaseUserData: BaseUserData{
					Header:    "test",
					DiskSetup: tt.diskSetup,
					Mounts:    tt.mounts,
				},
				Certificates:         secret.Certificates{},
				ClusterConfiguration: "my-cluster-config",
				InitConfiguration:    "my-init-config",
			}

			out, err := NewInitControlPlane(cpinput)
			g.Expect(err).ToNot(HaveOccurred())

			if tt.expectedDiskSetup != "" {
				g.Expect(string(out)).To(ContainSubstring(tt.expectedDiskSetup))
			}
			if tt.expectedFSSetup != "" {
				g.Expect(string(out)).To(ContainSubstring(tt.expectedFSSetup))
			}
			if tt.expectedMounts != "" {
				g.Expect(string(out)).To(ContainSubstring(tt.expectedMounts))
			}
		})
	}
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

func TestOmittableFields(t *testing.T) {
	tests := []struct {
		name string
		A    BaseUserData
		B    BaseUserData
	}{
		{
			name: "No diff between empty or nil additionalFiles", // NOTE: it maps to .Files in the KubeadmConfigSpec
			A: BaseUserData{
				AdditionalFiles: []bootstrapv1.File{},
			},
			B: BaseUserData{
				AdditionalFiles: nil,
			},
		},
		{
			name: "No diff between empty or nil diskSetup.partitions",
			A: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Partitions: []bootstrapv1.Partition{},
				},
			},
			B: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Partitions: nil,
				},
			},
		},
		{
			name: "No diff between empty or nil diskSetup.filesystems.extraOpts",
			A: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Filesystems: []bootstrapv1.Filesystem{
						{
							ExtraOpts: []string{},
						},
					},
				},
			},
			B: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Filesystems: []bootstrapv1.Filesystem{
						{
							ExtraOpts: nil,
						},
					},
				},
			},
		},
		{
			name: "No diff between empty or nil diskSetup.filesystems",
			A: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Filesystems: []bootstrapv1.Filesystem{},
				},
			},
			B: BaseUserData{
				DiskSetup: &bootstrapv1.DiskSetup{
					Filesystems: nil,
				},
			},
		},
		{
			name: "No diff between empty or nil mounts",
			A: BaseUserData{
				Mounts: []bootstrapv1.MountPoints{},
			},
			B: BaseUserData{
				Mounts: nil,
			},
		},
		{
			name: "No diff between empty or nil bootCommands",
			A: BaseUserData{
				BootCommands: []string{},
			},
			B: BaseUserData{
				BootCommands: nil,
			},
		},
		{
			name: "No diff between empty or nil preKubeadmCommands",
			A: BaseUserData{
				PreKubeadmCommands: []string{},
			},
			B: BaseUserData{
				PreKubeadmCommands: nil,
			},
		},
		{
			name: "No diff between empty or nil postKubeadmCommands",
			A: BaseUserData{
				PostKubeadmCommands: []string{},
			},
			B: BaseUserData{
				PostKubeadmCommands: nil,
			},
		},
		{
			name: "No diff between empty or nil users",
			A: BaseUserData{
				Users: []bootstrapv1.User{},
			},
			B: BaseUserData{
				Users: nil,
			},
		},
		{
			name: "No diff between empty or nil users.sshAuthorizedKeys",
			A: BaseUserData{
				Users: []bootstrapv1.User{
					{
						SSHAuthorizedKeys: nil,
					},
				},
			},
			B: BaseUserData{
				Users: []bootstrapv1.User{
					{
						SSHAuthorizedKeys: []string{},
					},
				},
			},
		},
		{
			name: "No diff between empty or nil ntp.servers",
			A: BaseUserData{
				NTP: &bootstrapv1.NTP{
					Servers: []string{},
				},
			},
			B: BaseUserData{
				NTP: &bootstrapv1.NTP{
					Servers: nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			outA, err := NewInitControlPlane(&ControlPlaneInput{BaseUserData: tt.A})
			g.Expect(err).ToNot(HaveOccurred())

			outB, err := NewInitControlPlane(&ControlPlaneInput{BaseUserData: tt.B})
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(outA)).To(Equal(string(outB)))

			outA, err = NewJoinControlPlane(&ControlPlaneJoinInput{BaseUserData: tt.A})
			g.Expect(err).ToNot(HaveOccurred())

			outB, err = NewJoinControlPlane(&ControlPlaneJoinInput{BaseUserData: tt.B})
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(outA)).To(Equal(string(outB)))
		})
	}
}

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

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestNewInitControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
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
			PreKubeadmCommands:  []string{`"echo $(date) ': hello world!'"`},
			PostKubeadmCommands: []string{"echo $(date) ': hello world!'"},
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

	expectedCommands := []string{
		`"\"echo $(date) ': hello world!'\""`,
		`"echo $(date) ': hello world!'"`,
	}
	for _, f := range expectedCommands {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewInitControlPlaneDiskMounts(t *testing.T) {
	tests := []struct {
		name     string
		input    *ControlPlaneInput
		expected []string
	}{
		{
			name: "simple layout",
			input: &ControlPlaneInput{
				BaseUserData: BaseUserData{
					Header: "test",
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
			},
			expected: []string{
				`disk_setup:
  test-device:
    table_type: gpt
    layout: true
    overwrite: false`,
			},
		},
		{
			name: "detailed disk layout",
			input: &ControlPlaneInput{
				BaseUserData: BaseUserData{
					Header: "test",
					DiskSetup: &bootstrapv1.DiskSetup{
						Partitions: []bootstrapv1.Partition{
							{
								Device: "test-device",
								DiskLayout: bootstrapv1.DiskLayout{
									{Percentage: 33},
									{Percentage: 33, PartitionType: ptr.To[int32](82)},
									{Percentage: 33},
								},
								Overwrite: ptr.To(true),
								TableType: ptr.To("gpt"),
							},
						},
					},
				},
				Certificates:         secret.Certificates{},
				ClusterConfiguration: "my-cluster-config",
				InitConfiguration:    "my-init-config",
			},
			expected: []string{
				`disk_setup:
  test-device:
    table_type: gpt
    layout:
      - 33
      - [33, 82]
      - 33
    overwrite: true`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			out, err := NewInitControlPlane(tt.input)
			g.Expect(err).ToNot(HaveOccurred())

			for _, expectedString := range tt.expected {
				g.Expect(string(out)).To(ContainSubstring(expectedString))
			}
		})
	}
}

func TestNewJoinControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
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

func TestNewJoinControlPlaneExperimentalRetry(t *testing.T) {
	g := NewWithT(t)

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
			Header:               "test",
			PreKubeadmCommands:   nil,
			PostKubeadmCommands:  nil,
			UseExperimentalRetry: true,
			WriteFiles:           nil,
			Users:                nil,
			NTP:                  nil,
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
		`-   path: ` + retriableJoinScriptName + `
    owner: ` + retriableJoinScriptOwner + `
    permissions: '` + retriableJoinScriptPermissions + `'
    `,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func Test_useKubeadmBootstrapScriptPre1_31(t *testing.T) {
	tests := []struct {
		name          string
		parsedversion semver.Version
		want          bool
	}{
		{
			name:          "true for version for v1.30",
			parsedversion: semver.MustParse("1.30.99"),
			want:          true,
		},
		{
			name:          "true for version for v1.28",
			parsedversion: semver.MustParse("1.28.0"),
			want:          true,
		},
		{
			name:          "false for v1.31.0",
			parsedversion: semver.MustParse("1.31.0"),
			want:          false,
		},
		{
			name:          "false for v1.31.0-beta.0",
			parsedversion: semver.MustParse("1.31.0-beta.0"),
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := useKubeadmBootstrapScriptPre1_31(tt.parsedversion); got != tt.want {
				t.Errorf("useKubeadmBootstrapScriptPre1_31() = %v, want %v", got, tt.want)
			}
		})
	}
}

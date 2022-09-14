/*
Copyright 2021 The Kubernetes Authors.

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

// Package clc_test tests clc package.
package clc_test

import (
	"testing"

	ignition "github.com/flatcar/ignition/config/v2_3"
	"github.com/flatcar/ignition/config/v2_3/types"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/pointer"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/cloudinit"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/ignition/clc"
)

const (
	configWithWarning = `---
storage:
  files:
  - path: /foo
    contents:
      inline: foo
`

	// Should generate an Ignition warning about the colon in the partition label.
	configWithIgnitionWarning = `---
storage:
  disks:
  - device: /dev/sda
    partitions:
    - label: foo:bar
`
)

func TestRender(t *testing.T) {
	t.Parallel()

	preKubeadmCommands := []string{
		"pre-command",
		"another-pre-command",
		// Test multi-line commands as well.
		"cat <<EOF > /etc/modules-load.d/containerd.conf\noverlay\nbr_netfilter\nEOF\n",
	}
	postKubeadmCommands := []string{
		"post-kubeadm-command",
		"another-post-kubeamd-command",
		// Test multi-line commands as well.
		"cat <<EOF > /etc/modules-load.d/containerd.conf\noverlay\nbr_netfilter\nEOF\n",
	}

	tc := []struct {
		desc         string
		input        *cloudinit.BaseUserData
		wantIgnition types.Config
	}{
		{
			desc: "renders valid Ignition JSON",
			input: &cloudinit.BaseUserData{
				PreKubeadmCommands:  preKubeadmCommands,
				PostKubeadmCommands: postKubeadmCommands,
				KubeadmCommand:      "kubeadm join",
				NTP: &bootstrapv1.NTP{
					Enabled: pointer.BoolPtr(true),
					Servers: []string{
						"foo.bar",
						"baz",
					},
				},
				Users: []bootstrapv1.User{
					{
						Name:         "foo",
						Gecos:        pointer.StringPtr("Foo B. Bar"),
						Groups:       pointer.StringPtr("foo, bar"),
						HomeDir:      pointer.StringPtr("/home/foo"),
						Shell:        pointer.StringPtr("/bin/false"),
						Passwd:       pointer.StringPtr("$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/"),
						PrimaryGroup: pointer.StringPtr("foo"),
						Sudo:         pointer.StringPtr("ALL=(ALL) NOPASSWD:ALL"),
						SSHAuthorizedKeys: []string{
							"foo",
							"bar",
						},
					},
				},
				DiskSetup: &bootstrapv1.DiskSetup{
					Partitions: []bootstrapv1.Partition{
						{
							Device:    "/dev/disk/azure/scsi1/lun0",
							Layout:    true,
							Overwrite: pointer.BoolPtr(true),
							TableType: pointer.StringPtr("gpt"),
						},
					},
					Filesystems: []bootstrapv1.Filesystem{
						{
							Device:     "/dev/disk/azure/scsi1/lun0",
							Filesystem: "ext4",
							Label:      "test_disk",
							ExtraOpts:  []string{"-F", "-E", "lazy_itable_init=1,lazy_journal_init=1"},
							Overwrite:  pointer.BoolPtr(true),
						},
					},
				},
				Mounts: []bootstrapv1.MountPoints{
					{
						"test_disk", "/var/lib/testdir", "foo",
					},
				},
				WriteFiles: []bootstrapv1.File{
					{
						Path:        "/etc/testfile.yaml",
						Encoding:    bootstrapv1.Base64,
						Content:     "Zm9vCg==",
						Permissions: "0600",
						Owner:       "nobody:nobody",
					},
				},
			},
			wantIgnition: types.Config{
				Ignition: types.Ignition{
					Version: "2.3.0",
				},
				Passwd: types.Passwd{
					Users: []types.PasswdUser{
						{
							Gecos: "Foo B. Bar",
							Groups: []types.Group{
								"foo",
								"bar",
							},
							HomeDir:      "/home/foo",
							Name:         "foo",
							PasswordHash: pointer.StringPtr("$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/"),
							PrimaryGroup: "foo",
							SSHAuthorizedKeys: []types.SSHAuthorizedKey{
								"foo",
								"bar",
							},
							Shell: "/bin/false",
						},
					},
				},
				Storage: types.Storage{
					Disks: []types.Disk{
						{
							Device:     "/dev/disk/azure/scsi1/lun0",
							Partitions: []types.Partition{{}},
							WipeTable:  true,
						},
					},
					Files: []types.File{
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/sudoers.d/foo",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,foo%20ALL%3D(ALL)%20NOPASSWD%3AALL%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/testfile.yaml",
								User:       &types.NodeUser{Name: "nobody"},
								Group:      &types.NodeGroup{Name: "nobody"},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,foo%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23!%2Fbin%2Fbash%0Aset%20-e%0A%0Apre-command%0Aanother-pre-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A%0A%0Akubeadm%20join%0Amkdir%20-p%20%2Frun%2Fcluster-api%20%26%26%20echo%20success%20%3E%20%2Frun%2Fcluster-api%2Fbootstrap-success.complete%0Amv%20%2Fetc%2Fkubeadm.yml%20%2Ftmp%2F%0A%0Apost-kubeadm-command%0Aanother-post-kubeamd-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A",
								},
								Mode: pointer.IntPtr(448),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,---%0Afoo%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/ntp.conf",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23%20Common%20pool%0Aserver%20foo.bar%0Aserver%20baz%0A%0A%23%20Warning%3A%20Using%20default%20NTP%20settings%20will%20leave%20your%20NTP%0A%23%20server%20accessible%20to%20all%20hosts%20on%20the%20Internet.%0A%0A%23%20If%20you%20want%20to%20deny%20all%20machines%20(including%20your%20own)%0A%23%20from%20accessing%20the%20NTP%20server%2C%20uncomment%3A%0A%23restrict%20default%20ignore%0A%0A%23%20Default%20configuration%3A%0A%23%20-%20Allow%20only%20time%20queries%2C%20at%20a%20limited%20rate%2C%20sending%20KoD%20when%20in%20excess.%0A%23%20-%20Allow%20all%20local%20queries%20(IPv4%2C%20IPv6)%0Arestrict%20default%20nomodify%20nopeer%20noquery%20notrap%20limited%20kod%0Arestrict%20127.0.0.1%0Arestrict%20%5B%3A%3A1%5D%0A",
								},
								Mode: pointer.IntPtr(420),
							},
						},
					},
					Filesystems: []types.Filesystem{
						{
							Mount: &types.Mount{
								Device: "/dev/disk/azure/scsi1/lun0",
								Format: "ext4",
								Label:  pointer.StringPtr("test_disk"),
								Options: []types.MountOption{
									"-F",
									"-E",
									"lazy_itable_init=1,lazy_journal_init=1",
								},
								WipeFilesystem: true,
							},
							Name: "test_disk",
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
							Enabled:  pointer.BoolPtr(true),
							Name:     "kubeadm.service",
						},
						{
							Enabled: pointer.BoolPtr(true),
							Name:    "ntpd.service",
						},
						{
							Contents: "[Unit]\nDescription = Mount test_disk\n\n[Mount]\nWhat=/dev/disk/azure/scsi1/lun0\nWhere=/var/lib/testdir\nOptions=foo\n\n[Install]\nWantedBy=multi-user.target\n",
							Enabled:  pointer.BoolPtr(true),
							Name:     "var-lib-testdir.mount",
						},
					},
				},
			},
		},
		{
			desc: "multiple users with password auth",
			input: &cloudinit.BaseUserData{
				PreKubeadmCommands:  preKubeadmCommands,
				PostKubeadmCommands: postKubeadmCommands,
				KubeadmCommand:      "kubeadm join",
				Users: []bootstrapv1.User{
					{
						Name:         "foo",
						LockPassword: pointer.BoolPtr(false),
					},
					{
						Name:         "bar",
						LockPassword: pointer.BoolPtr(false),
					},
				},
			},
			wantIgnition: types.Config{
				Ignition: types.Ignition{
					Version: "2.3.0",
				},
				Passwd: types.Passwd{
					Users: []types.PasswdUser{
						{
							Name: "foo",
						},
						{
							Name: "bar",
						},
					},
				},
				Storage: types.Storage{
					Files: []types.File{
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/ssh/sshd_config",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23%20Use%20most%20defaults%20for%20sshd%20configuration.%0ASubsystem%20sftp%20internal-sftp%0AClientAliveInterval%20180%0AUseDNS%20no%0AUsePAM%20yes%0APrintLastLog%20no%20%23%20handled%20by%20PAM%0APrintMotd%20no%20%23%20handled%20by%20PAM%0A%0AMatch%20User%20foo%2Cbar%0A%20%20PasswordAuthentication%20yes%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23!%2Fbin%2Fbash%0Aset%20-e%0A%0Apre-command%0Aanother-pre-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A%0A%0Akubeadm%20join%0Amkdir%20-p%20%2Frun%2Fcluster-api%20%26%26%20echo%20success%20%3E%20%2Frun%2Fcluster-api%2Fbootstrap-success.complete%0Amv%20%2Fetc%2Fkubeadm.yml%20%2Ftmp%2F%0A%0Apost-kubeadm-command%0Aanother-post-kubeamd-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A",
								},
								Mode: pointer.IntPtr(448),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,---%0Afoo%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
							Enabled:  pointer.BoolPtr(true),
							Name:     "kubeadm.service",
						},
					},
				},
			},
		},
		{
			desc: "base64 encoded content",
			input: &cloudinit.BaseUserData{
				PreKubeadmCommands:  preKubeadmCommands,
				PostKubeadmCommands: postKubeadmCommands,
				KubeadmCommand:      "kubeadm join",
				WriteFiles: []bootstrapv1.File{
					{
						Path:        "/etc/base64encodedcontent.yaml",
						Encoding:    bootstrapv1.Base64,
						Content:     "Zm9vCg==",
						Permissions: "0600",
					},
					{
						Path:        "/etc/plaincontent.yaml",
						Content:     "foo",
						Permissions: "0600",
					},
				},
			},
			wantIgnition: types.Config{
				Ignition: types.Ignition{
					Version: "2.3.0",
				},
				Storage: types.Storage{
					Files: []types.File{
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/base64encodedcontent.yaml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,foo%0A"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/plaincontent.yaml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,foo%0A"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23!%2Fbin%2Fbash%0Aset%20-e%0A%0Apre-command%0Aanother-pre-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A%0A%0Akubeadm%20join%0Amkdir%20-p%20%2Frun%2Fcluster-api%20%26%26%20echo%20success%20%3E%20%2Frun%2Fcluster-api%2Fbootstrap-success.complete%0Amv%20%2Fetc%2Fkubeadm.yml%20%2Ftmp%2F%0A%0Apost-kubeadm-command%0Aanother-post-kubeamd-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A",
								},
								Mode: pointer.IntPtr(448),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,---%0Afoo%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
							Enabled:  pointer.BoolPtr(true),
							Name:     "kubeadm.service",
						},
					},
				},
			},
		},
		{
			desc: "all file ownership combinations",
			input: &cloudinit.BaseUserData{
				PreKubeadmCommands:  preKubeadmCommands,
				PostKubeadmCommands: postKubeadmCommands,
				KubeadmCommand:      "kubeadm join",
				WriteFiles: []bootstrapv1.File{
					{
						Path:        "/etc/username-group-name-owner.yaml",
						Owner:       "nobody:nobody",
						Permissions: "0600",
					},
					{
						Path:        "/etc/user-only-owner.yaml",
						Owner:       "nobody",
						Permissions: "0600",
					},
					{
						Path:        "/etc/user-only-with-colon-owner.yaml",
						Owner:       "nobody:",
						Permissions: "0600",
					},
					{
						Path:        "/etc/group-only-owner.yaml",
						Owner:       ":nobody",
						Permissions: "0600",
					},
				},
			},
			wantIgnition: types.Config{
				Ignition: types.Ignition{
					Version: "2.3.0",
				},
				Storage: types.Storage{
					Files: []types.File{
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/username-group-name-owner.yaml",
								User: &types.NodeUser{
									Name: "nobody",
								},
								Group: &types.NodeGroup{
									Name: "nobody",
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/user-only-owner.yaml",
								User: &types.NodeUser{
									Name: "nobody",
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/user-only-with-colon-owner.yaml",
								User: &types.NodeUser{
									Name: "nobody",
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/group-only-owner.yaml",
								Group: &types.NodeGroup{
									Name: "nobody",
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{Source: "data:,"},
								Mode:     pointer.IntPtr(384),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,%23!%2Fbin%2Fbash%0Aset%20-e%0A%0Apre-command%0Aanother-pre-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A%0A%0Akubeadm%20join%0Amkdir%20-p%20%2Frun%2Fcluster-api%20%26%26%20echo%20success%20%3E%20%2Frun%2Fcluster-api%2Fbootstrap-success.complete%0Amv%20%2Fetc%2Fkubeadm.yml%20%2Ftmp%2F%0A%0Apost-kubeadm-command%0Aanother-post-kubeamd-command%0Acat%20%3C%3CEOF%20%3E%20%2Fetc%2Fmodules-load.d%2Fcontainerd.conf%0Aoverlay%0Abr_netfilter%0AEOF%0A",
								},
								Mode: pointer.IntPtr(448),
							},
						},
						{
							Node: types.Node{
								Filesystem: "root",
								Path:       "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.FileContents{
									Source: "data:,---%0Afoo%0A",
								},
								Mode: pointer.IntPtr(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: "[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n",
							Enabled:  pointer.BoolPtr(true),
							Name:     "kubeadm.service",
						},
					},
				},
			},
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			ignitionBytes, _, err := clc.Render(tt.input, &bootstrapv1.ContainerLinuxConfig{}, "foo")
			if err != nil {
				t.Fatalf("rendering: %v", err)
			}

			ign, reports, err := ignition.Parse(ignitionBytes)
			if err != nil {
				t.Fatalf("Parsing generated Ignition: %v", err)
			}

			if reports.IsFatal() {
				t.Fatalf("Generated Ignition has fatal reports: %s", reports)
			}

			if diff := cmp.Diff(tt.wantIgnition, ign); diff != "" {
				t.Fatalf("Ignition mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("validates input parameter", func(t *testing.T) {
		t.Parallel()

		if _, _, err := clc.Render(nil, &bootstrapv1.ContainerLinuxConfig{}, "foo"); err == nil {
			t.Fatal("expected error when passing empty input data")
		}
	})

	t.Run("accepts empty clc parameter", func(t *testing.T) {
		t.Parallel()

		if _, _, err := clc.Render(&cloudinit.BaseUserData{}, nil, "bar"); err != nil {
			t.Fatalf("unexpected error while rendering: %v", err)
		}
	})

	t.Run("treats warnings as errors in strict mode", func(t *testing.T) {
		config := &bootstrapv1.ContainerLinuxConfig{
			Strict:           true,
			AdditionalConfig: configWithWarning,
		}

		if _, _, err := clc.Render(&cloudinit.BaseUserData{}, config, "foo"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("returns warnings", func(t *testing.T) {
		config := &bootstrapv1.ContainerLinuxConfig{
			AdditionalConfig: configWithWarning,
		}

		data, warnings, err := clc.Render(&cloudinit.BaseUserData{}, config, "foo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if warnings == "" {
			t.Errorf("expected warnings to be not empty")
		}

		if len(data) == 0 {
			t.Errorf("expected data to be returned on config with warnings")
		}
	})

	t.Run("returns Ignition warnings", func(t *testing.T) {
		config := &bootstrapv1.ContainerLinuxConfig{
			AdditionalConfig: configWithIgnitionWarning,
		}

		data, warnings, err := clc.Render(&cloudinit.BaseUserData{}, config, "foo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if warnings == "" {
			t.Errorf("expected warnings to be not empty")
		}

		if len(data) == 0 {
			t.Errorf("expected data to be returned on config with warnings")
		}
	})
}

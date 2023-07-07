/*
Copyright 2023 The Kubernetes Authors.

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

// Package butane_test tests butane package.
package butane_test

import (
	"testing"

	"github.com/coreos/ignition/v2/config/util"
	ignition "github.com/coreos/ignition/v2/config/v3_4"
	"github.com/coreos/ignition/v2/config/v3_4/types"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/pointer"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/cloudinit"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/internal/ignition/butane"
)

const (
	configWithWarning = `---
variant: flatcar
version: 1.1.0
storage:
  files:
  - path: /foo
    contents:
      inline: foo
foo:
`

	// Should generate an Ignition warning about the colon in the partition label.
	configWithIgnitionWarning = `---
variant: flatcar
version: 1.1.0
storage:
  disks:
  - device: /dev/sda
foo:
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
					Enabled: pointer.Bool(true),
					Servers: []string{
						"foo.bar",
						"baz",
					},
				},
				Users: []bootstrapv1.User{
					{
						Name:         "foo",
						Gecos:        pointer.String("Foo B. Bar"),
						Groups:       pointer.String("foo, bar"),
						HomeDir:      pointer.String("/home/foo"),
						Shell:        pointer.String("/bin/false"),
						Passwd:       pointer.String("$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/"),
						PrimaryGroup: pointer.String("foo"),
						Sudo:         pointer.String("ALL=(ALL) NOPASSWD:ALL"),
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
							Overwrite: pointer.Bool(true),
							TableType: pointer.String("gpt"),
						},
					},
					Filesystems: []bootstrapv1.Filesystem{
						{
							Device:     "/dev/disk/azure/scsi1/lun0p1",
							Filesystem: "ext4",
							Label:      "test_disk",
							ExtraOpts:  []string{"-F", "-E", "lazy_itable_init=1,lazy_journal_init=1"},
							Overwrite:  pointer.Bool(true),
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
					Version: "3.4.0",
				},
				Passwd: types.Passwd{
					Users: []types.PasswdUser{
						{
							Gecos: util.StrToPtr("Foo B. Bar"),
							Groups: []types.Group{
								"foo",
								"bar",
							},
							HomeDir:      util.StrToPtr("/home/foo"),
							Name:         "foo",
							PasswordHash: pointer.String("$6$j212wezy$7H/1LT4f9/N3wpgNunhsIqtMj62OKiS3nyNwuizouQc3u7MbYCarYeAHWYPYb2FT.lbioDm2RrkJPb9BZMN1O/"),
							PrimaryGroup: util.StrToPtr("foo"),
							SSHAuthorizedKeys: []types.SSHAuthorizedKey{
								"foo",
								"bar",
							},
							Shell: util.StrToPtr("/bin/false"),
						},
					},
				},
				Storage: types.Storage{
					Disks: []types.Disk{
						{
							Device: "/dev/disk/azure/scsi1/lun0",
							Partitions: []types.Partition{{
								Number:             1,
								SizeMiB:            util.IntToPtr(0),
								StartMiB:           util.IntToPtr(0),
								WipePartitionEntry: util.BoolToPtr(true),
							}},
							WipeTable: util.BoolToPtr(true),
						},
					},
					Files: []types.File{
						{
							Node: types.Node{
								Path: "/etc/sudoers.d/foo",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,foo%20ALL%3D(ALL)%20NOPASSWD%3AALL%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path:  "/etc/testfile.yaml",
								User:  types.NodeUser{Name: util.StrToPtr("nobody")},
								Group: types.NodeGroup{Name: util.StrToPtr("nobody")},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,foo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/6yPQWoDMQxF9zqFSiE7jy8Quku2OULxyArjxrKMLAdy+1I6lNJus/168PReX+JaWlzT2GCwY2CAbhxIRVLLkJr6xhZ+b5Qcj8fT5YxvGNkpiuZZeYSqKS85kjZPpbHlhbRdQe9sNT1gtffGfi3V2eB0OQPAba6csuCHlgZyy8UwdIw2W6Q6h7OF1AseDsi0KY5JxGN8ef8gcVX14ZZ62KGFVHplZ5D795u7bHlIxejSI0DX4WHf/zf/HCU/P/4zAAD//xTZa6R6AQAA"),
								},
								Mode: pointer.Int(448),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,---%0Afoo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/ntp.conf",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/2SPQYvUQBCF7/0rHuSyC2PYEVHITdzLIMgcFA/ioadTSQq7q8buSmL89dJxZ1fwWNX9vvdVgw+akgquqtEVygtlDKrtxefbePG/nWvw1WdhGTt8KSwjehr8HA2fPp9RyIxlLFg5RkTyC2HTOddH1+CJ40OgUvgSCabwMWLSYgUqsIlwEqMsZG0tOw0VgNWL1c89ybYnkg8TCxXcsYQ499Vkb9JV7l2DIWu6Fcm4c/8KVoMDZgmaEol1rslULHOw50t4FM1U2x+fNkFl4HHO3lilcw1e4X2MukIlbjBOhJ8zZaZygDd4RE5s1CN7owMKyS74UR+xTiRgAf2qbu0/rHpW1ODjjYW703l5c8DpvLy9d/9piibtedggeiXKEK25Olr212eFH9q/ZI+v37UP7UN7fFl967rjd/cnAAD//30wxGoAAgAA"),
								},
								Mode: pointer.Int(420),
							},
						},
					},
					Filesystems: []types.Filesystem{
						{

							Device: "/dev/disk/azure/scsi1/lun0p1",
							Format: util.StrToPtr("ext4"),
							Label:  pointer.String("test_disk"),
							MountOptions: []types.MountOption{
								"-F",
								"-E",
								"lazy_itable_init=1,lazy_journal_init=1",
							},
							WipeFilesystem: util.BoolToPtr(true),
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: util.StrToPtr("[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\nAfter=network.target\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n"),
							Enabled:  pointer.Bool(true),
							Name:     "kubeadm.service",
						},
						{
							Enabled: pointer.Bool(true),
							Name:    "ntpd.service",
						},
						{
							Contents: util.StrToPtr("[Unit]\nDescription = Mount test_disk\n\n[Mount]\nWhat=/dev/disk/azure/scsi1/lun0p1\nWhere=/var/lib/testdir\nOptions=foo\n\n[Install]\nWantedBy=multi-user.target\n"),
							Enabled:  pointer.Bool(true),
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
						LockPassword: pointer.Bool(false),
					},
					{
						Name:         "bar",
						LockPassword: pointer.Bool(false),
					},
				},
			},
			wantIgnition: types.Config{
				Ignition: types.Ignition{
					Version: "3.4.0",
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
								Path: "/etc/ssh/sshd_config",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/2yOsWoDMRBEe33FgNskJF3aI2kCvnBg/AF7p5UlkHeDduVwfx/ktO5meAPzDjgb46rmiJyoVzckbTDLEZtKKpfeyIvKSzj11XZzvsKS/6CIcxOqz6OFj1pYfKrlxl8D3Kji7f01nI0/v08QHWmZZuxsYWlF/EjmR71AFAdkklg5Yt2xTPP/YFaPD2mYybc8zBuS6tNKLQALmf1qi1P3zOJlu2vf//4CAAD//2/4Fa3mAAAA"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/6yPQWoDMQxF9zqFSiE7jy8Quku2OULxyArjxrKMLAdy+1I6lNJus/168PReX+JaWlzT2GCwY2CAbhxIRVLLkJr6xhZ+b5Qcj8fT5YxvGNkpiuZZeYSqKS85kjZPpbHlhbRdQe9sNT1gtffGfi3V2eB0OQPAba6csuCHlgZyy8UwdIw2W6Q6h7OF1AseDsi0KY5JxGN8ef8gcVX14ZZ62KGFVHplZ5D795u7bHlIxejSI0DX4WHf/zf/HCU/P/4zAAD//xTZa6R6AQAA"),
								},
								Mode: pointer.Int(448),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,---%0Afoo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: util.StrToPtr("[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\nAfter=network.target\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n"),
							Enabled:  pointer.Bool(true),
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
					Version: "3.4.0",
				},
				Storage: types.Storage{
					Files: []types.File{
						{
							Node: types.Node{
								Path: "/etc/base64encodedcontent.yaml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,foo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/plaincontent.yaml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,foo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/6yPQWoDMQxF9zqFSiE7jy8Quku2OULxyArjxrKMLAdy+1I6lNJus/168PReX+JaWlzT2GCwY2CAbhxIRVLLkJr6xhZ+b5Qcj8fT5YxvGNkpiuZZeYSqKS85kjZPpbHlhbRdQe9sNT1gtffGfi3V2eB0OQPAba6csuCHlgZyy8UwdIw2W6Q6h7OF1AseDsi0KY5JxGN8ef8gcVX14ZZ62KGFVHplZ5D795u7bHlIxejSI0DX4WHf/zf/HCU/P/4zAAD//xTZa6R6AQAA"),
								},
								Mode: pointer.Int(448),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,---%0Afoo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: util.StrToPtr("[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\nAfter=network.target\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n"),
							Enabled:  pointer.Bool(true),
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
					Version: "3.4.0",
				},
				Storage: types.Storage{
					Files: []types.File{
						{
							Node: types.Node{
								Path: "/etc/username-group-name-owner.yaml",
								User: types.NodeUser{
									Name: util.StrToPtr("nobody"),
								},
								Group: types.NodeGroup{
									Name: util.StrToPtr("nobody"),
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/user-only-owner.yaml",
								User: types.NodeUser{
									Name: util.StrToPtr("nobody"),
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/user-only-with-colon-owner.yaml",
								User: types.NodeUser{
									Name: util.StrToPtr("nobody"),
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/group-only-owner.yaml",
								Group: types.NodeGroup{
									Name: util.StrToPtr("nobody"),
								},
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,"),
								},
								Mode: pointer.Int(384),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.sh",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr("gzip"),
									Source:      util.StrToPtr("data:;base64,H4sIAAAAAAAC/6yPQWoDMQxF9zqFSiE7jy8Quku2OULxyArjxrKMLAdy+1I6lNJus/168PReX+JaWlzT2GCwY2CAbhxIRVLLkJr6xhZ+b5Qcj8fT5YxvGNkpiuZZeYSqKS85kjZPpbHlhbRdQe9sNT1gtffGfi3V2eB0OQPAba6csuCHlgZyy8UwdIw2W6Q6h7OF1AseDsi0KY5JxGN8ef8gcVX14ZZ62KGFVHplZ5D795u7bHlIxejSI0DX4WHf/zf/HCU/P/4zAAD//xTZa6R6AQAA"),
								},
								Mode: pointer.Int(448),
							},
						},
						{
							Node: types.Node{
								Path: "/etc/kubeadm.yml",
							},
							FileEmbedded1: types.FileEmbedded1{
								Contents: types.Resource{
									Compression: util.StrToPtr(""),
									Source:      util.StrToPtr("data:,---%0Afoo%0A"),
								},
								Mode: pointer.Int(384),
							},
						},
					},
				},
				Systemd: types.Systemd{
					Units: []types.Unit{
						{
							Contents: util.StrToPtr("[Unit]\nDescription=kubeadm\n# Run only once. After successful run, this file is moved to /tmp/.\nConditionPathExists=/etc/kubeadm.yml\nAfter=network.target\n[Service]\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/kubeadm.sh\n[Install]\nWantedBy=multi-user.target\n"),
							Enabled:  pointer.Bool(true),
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

			ignitionBytes, _, err := butane.Render(tt.input, &bootstrapv1.ButaneConfig{}, "foo")
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

		if _, _, err := butane.Render(nil, &bootstrapv1.ButaneConfig{}, "foo"); err == nil {
			t.Fatal("expected error when passing empty input data")
		}
	})

	t.Run("accepts empty butane config parameter", func(t *testing.T) {
		t.Parallel()

		if _, _, err := butane.Render(&cloudinit.BaseUserData{}, nil, "bar"); err != nil {
			t.Fatalf("unexpected error while rendering: %v", err)
		}
	})

	t.Run("treats warnings as errors in strict mode", func(t *testing.T) {
		config := &bootstrapv1.ButaneConfig{
			Strict:           true,
			AdditionalConfig: configWithWarning,
		}

		if _, _, err := butane.Render(&cloudinit.BaseUserData{}, config, "foo"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("returns warnings", func(t *testing.T) {
		config := &bootstrapv1.ButaneConfig{
			AdditionalConfig: configWithWarning,
		}

		data, warnings, err := butane.Render(&cloudinit.BaseUserData{}, config, "foo")
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
		config := &bootstrapv1.ButaneConfig{
			AdditionalConfig: configWithIgnitionWarning,
		}

		data, warnings, err := butane.Render(&cloudinit.BaseUserData{}, config, "foo")
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

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

package v1alpha3

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKubeadmConfigConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	kubeadmConfigName := fmt.Sprintf("test-kubeadmconfig-%s", util.RandomString(5))
	kubeadmConfig := &KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigName,
			Namespace: ns.Name,
		},
		Spec: fakeKubeadmConfigSpec,
	}

	g.Expect(env.Create(ctx, kubeadmConfig)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, kubeadmConfig)
}

func TestKubeadmConfigTemplateConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	kubeadmConfigTemplateName := fmt.Sprintf("test-kubeadmconfigtemplate-%s", util.RandomString(5))
	kubeadmConfigTemplate := &KubeadmConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigTemplateName,
			Namespace: ns.Name,
		},
		Spec: KubeadmConfigTemplateSpec{
			Template: KubeadmConfigTemplateResource{
				Spec: fakeKubeadmConfigSpec,
			},
		},
	}

	g.Expect(env.Create(ctx, kubeadmConfigTemplate)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, kubeadmConfigTemplate)
}

var fakeKubeadmConfigSpec = KubeadmConfigSpec{
	ClusterConfiguration: &upstreamv1beta1.ClusterConfiguration{
		KubernetesVersion: "v1.20.2",
		APIServer: upstreamv1beta1.APIServer{
			ControlPlaneComponent: upstreamv1beta1.ControlPlaneComponent{
				ExtraArgs: map[string]string{
					"foo": "bar",
				},
				ExtraVolumes: []upstreamv1beta1.HostPathMount{
					{
						Name:      "mount-path",
						HostPath:  "/foo",
						MountPath: "/foo",
						ReadOnly:  false,
					},
				},
			},
		},
	},
	InitConfiguration: &upstreamv1beta1.InitConfiguration{
		NodeRegistration: upstreamv1beta1.NodeRegistrationOptions{
			Name:      "foo",
			CRISocket: "/var/run/containerd/containerd.sock",
		},
	},
	JoinConfiguration: &upstreamv1beta1.JoinConfiguration{
		NodeRegistration: upstreamv1beta1.NodeRegistrationOptions{
			Name:      "foo",
			CRISocket: "/var/run/containerd/containerd.sock",
		},
	},
	Files: []File{
		{
			Path:        "/foo",
			Owner:       "root:root",
			Permissions: "0644",
			Content:     "foo",
		},
		{
			Path:        "/foobar",
			Owner:       "root:root",
			Permissions: "0644",
			ContentFrom: &FileSource{
				Secret: SecretFileSource{
					Name: "foo",
					Key:  "bar",
				},
			},
		},
	},
	DiskSetup: &DiskSetup{
		Partitions: []Partition{
			{
				Device:    "/dev/disk/scsi1/lun0",
				Layout:    true,
				Overwrite: pointer.Bool(false),
				TableType: pointer.String("gpt"),
			},
		},
		Filesystems: []Filesystem{
			{
				Device:     "/dev/disk/scsi2/lun0",
				Filesystem: "ext4",
				Label:      "disk",
				Partition:  pointer.String("auto"),
				Overwrite:  pointer.Bool(true),
				ReplaceFS:  pointer.String("ntfs"),
				ExtraOpts:  []string{"-E"},
			},
		},
	},
	Mounts: []MountPoints{
		{
			"LABEL=disk",
			"/var/lib/disk",
		},
	},
	PreKubeadmCommands:  []string{`echo "foo"`},
	PostKubeadmCommands: []string{`echo "bar"`},
	Users: []User{
		{
			Name:              "foo",
			Groups:            pointer.String("foo"),
			HomeDir:           pointer.String("/home/foo"),
			Inactive:          pointer.Bool(false),
			Shell:             pointer.String("/bin/bash"),
			Passwd:            pointer.String("password"),
			SSHAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQD24GRNlhO+rgrseyWYrGwP0PACO/9JAsKV06W63yQ=="},
		},
	},
	NTP: &NTP{
		Servers: []string{"ntp.server.local"},
		Enabled: pointer.Bool(true),
	},
	Format:                   Format("cloud-config"),
	Verbosity:                pointer.Int32(3),
	UseExperimentalRetryJoin: true,
}

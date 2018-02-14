/*
Copyright 2018 The Kubernetes Authors.

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

package google

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
)

var aConfig = `{
    "apiVersion": "gceproviderconfig/v1alpha1",
    "kind": "GCEProviderConfig",
    "project": "ex-straw-dinary",
    "zone": "us-central1-f",
    "machineType": "n1-standard-1",
    "image": "projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts"
}`
var bConfig = `{
    "apiVersion": "gceproviderconfig/v1alpha1",
    "kind": "GCEProviderConfig",
    "project": "ex-straw-dinary",
    "machineType": "n1-standard-1",
    "image": "projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts"
}`

// This is aConfig but with changed zone.
var cConfig = `{
    "apiVersion": "gceproviderconfig/v1alpha1",
    "kind": "GCEProviderConfig",
    "project": "ex-straw-dinary",
    "zone": "us-central1-a",
    "machineType": "n1-standard-1",
    "image": "projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts"
}`

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestProviderConfigDecode(t *testing.T) {
	testConfigs := []struct {
		config      string
		project     string
		zone        string
		machineType string
		iamge       string
	}{
		{
			aConfig,
			"ex-straw-dinary",
			"us-central1-f",
			"n1-standard-1",
			"projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts",
		},
		{
			bConfig,
			"ex-straw-dinary",
			"",
			"n1-standard-1",
			"projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts",
		},
	}

	gce, _ := NewMachineActuator("", nil)
	for _, tt := range testConfigs {
		config, _ := gce.providerconfig(tt.config)
		assertEqual(t, config.Project, tt.project)
		assertEqual(t, config.Zone, tt.zone)
		assertEqual(t, config.MachineType, tt.machineType)
		assertEqual(t, config.Image, tt.iamge)
	}
}

func TestRequiresUpdate(t *testing.T) {
	tests := []struct {
		a            *clusterv1.Machine
		b            *clusterv1.Machine
		shouldUpdate bool
	}{
		// Provider configs different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ProviderConfig: cConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
		// Provider configs same, so no updates.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			false,
		},
		// Spec.ObjectMeta different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
					},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo-bar",
					},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
		// Spec.Roles different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					Roles: []string{"Master"},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					Roles: []string{"Node"},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
		// Spec.Versions different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					Versions: clusterv1.MachineVersionInfo{
						Kubelet: "foo",
					},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{},
				clusterv1.MachineSpec{
					Versions: clusterv1.MachineVersionInfo{
						Kubelet: "bar",
					},
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
		// ObjectMeta.Name different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{
					Name: "foo",
				},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{
					Name: "bar",
				},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
		// ObjectMeta.UID different.
		{
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{
					UID: "foo",
				},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			&clusterv1.Machine{
				metav1.TypeMeta{},
				metav1.ObjectMeta{
					UID: "bar",
				},
				clusterv1.MachineSpec{
					ProviderConfig: aConfig,
				},
				clusterv1.MachineStatus{},
			},
			true,
		},
	}

	gce, _ := NewMachineActuator("", nil)
	for _, tt := range tests {
		shouldUpdate := gce.requiresUpdate(tt.a, tt.b)
		assertEqual(t, shouldUpdate, tt.shouldUpdate)
	}
}

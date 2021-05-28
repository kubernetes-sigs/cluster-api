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

package client

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

type rolloutTest struct {
	name    string
	fields  fields
	args    args
	wantErr bool
}
type fields struct {
	client *fakeClient
}
type args struct {
	options RolloutOptions
}

// genericTestCases are test cases that can be passed to any of the rollout subcommands.
func genericTestCases() []rolloutTest {
	return []rolloutTest{
		{
			name: "return an error if machinedeployment is not found",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Resources:  []string{"machinedeployment/foo"},
					Namespace:  "default",
				},
			},
			wantErr: true,
		},
		{
			name: "return error if one of the machinedeployments is not found",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Resources:  []string{"machinedeployment/md-1", "machinedeployment/md-does-not-exist"},
					Namespace:  "default",
				},
			},
			wantErr: true,
		},
		{
			name: "return error if unknown resource specified",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Resources:  []string{"foo/bar"},
					Namespace:  "default",
				},
			},
			wantErr: true,
		},
		{
			name: "return error if no resource specified",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Namespace:  "default",
				},
			},
			wantErr: true,
		},
	}
}

func fakeClientForRollout() *fakeClient {
	core := config.NewProvider("cluster-api", "https://somewhere.com", clusterctlv1.CoreProviderType)
	infra := config.NewProvider("infra", "https://somewhere.com", clusterctlv1.InfrastructureProviderType)
	md1 := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: "cluster.x-k8s.io/v1alpha4",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "md-1",
		},
	}
	md2 := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: "cluster.x-k8s.io/v1alpha4",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "md-2",
		},
	}
	config1 := newFakeConfig().
		WithProvider(core).
		WithProvider(infra)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1).
		WithProviderInventory(core.Name(), core.Type(), "v1.0.0", "cluster-api-system").
		WithProviderInventory(infra.Name(), infra.Type(), "v2.0.0", "infra-system").
		WithObjs(md1).
		WithObjs(md2)

	client := newFakeClient(config1).
		WithCluster(cluster1)

	return client
}

func Test_clusterctlClient_RolloutRestart(t *testing.T) {
	tests := genericTestCases()
	additionalTests := []rolloutTest{
		{
			name: "do not return error if machinedeployment found",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Resources:  []string{"machinedeployment/md-1"},
					Namespace:  "default",
				},
			},
			wantErr: false,
		},
		{
			name: "do not return error if all machinedeployments found",
			fields: fields{
				client: fakeClientForRollout(),
			},
			args: args{
				options: RolloutOptions{
					Kubeconfig: Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					Resources:  []string{"machinedeployment/md-1", "machinedeployment/md-2"},
					Namespace:  "default",
				},
			},
			wantErr: false,
		},
	}

	tests = append(tests, additionalTests...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.fields.client.RolloutRestart(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func Test_clusterctlClient_RolloutPause(t *testing.T) {
	tests := genericTestCases()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.fields.client.RolloutPause(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func Test_clusterctlClient_RolloutResume(t *testing.T) {
	tests := genericTestCases()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.fields.client.RolloutResume(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

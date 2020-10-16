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

package cluster

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_providerInstaller_Install(t *testing.T) {
	unavailableDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "ns",
			UID:       "1",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	availableDeployment := unavailableDeployment.DeepCopy()
	availableDeployment.Status = appsv1.DeploymentStatus{
		Conditions: []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			},
		},
	}

	tests := []struct {
		name        string
		deployments []*appsv1.Deployment
		expectErr   bool
	}{
		{
			name:        "should return error if deployment is not available yet",
			deployments: []*appsv1.Deployment{unavailableDeployment.DeepCopy()},
			expectErr:   true,
		},
		{
			name:        "should succeed if deployment has the available condition",
			deployments: []*appsv1.Deployment{availableDeployment.DeepCopy()},
			expectErr:   false,
		},
		{
			name:        "should return error if one of the deployments is not available yet",
			deployments: []*appsv1.Deployment{availableDeployment.DeepCopy(), unavailableDeployment.DeepCopy()},
			expectErr:   true,
		},
	}

	fastBackoff := wait.Backoff{
		Duration: time.Millisecond,
		Factor:   1,
		Steps:    1,
		Jitter:   0,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			iq := make([]repository.Components, 0, len(tt.deployments))
			for _, d := range tt.deployments {
				dus, err := runtime.DefaultUnstructuredConverter.ToUnstructured(d)
				g.Expect(err).ToNot(HaveOccurred())
				obj := []unstructured.Unstructured{{Object: dus}}

				comp := newFakeComponents(fakeComponentsInput{
					name:         "some-component",
					version:      "v1.0.0",
					providerType: clusterctlv1.InfrastructureProviderType,
				}, withInstanceObjs(obj))

				iq = append(iq, comp)
			}

			proxy := test.NewFakeProxy()
			i := &providerInstaller{
				proxy:             proxy,
				providerInventory: newInventoryClient(proxy, nil),
				providerComponents: newComponentsClient(
					proxy,
					withCreateComponentObjectBackoff(fastBackoff),
					withReadComponentObjectBackoff(fastBackoff),
				),
				installQueue:         iq,
				installerReadBackoff: fastBackoff,
			}
			_, err := i.Install()
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}

}

func Test_providerInstaller_Validate(t *testing.T) {
	fakeReader := test.NewFakeReader().
		WithProvider("cluster-api", clusterctlv1.CoreProviderType, "https://somewhere.com").
		WithProvider("infra1", clusterctlv1.InfrastructureProviderType, "https://somewhere.com").
		WithProvider("infra2", clusterctlv1.InfrastructureProviderType, "https://somewhere.com")

	repositoryMap := map[string]repository.Repository{
		"cluster-api": test.NewFakeRepository().
			WithVersions("v1.0.0", "v1.0.1").
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
					{Major: 2, Minor: 0, Contract: "v1alpha4"},
				},
			}),
		"infrastructure-infra1": test.NewFakeRepository().
			WithVersions("v1.0.0", "v1.0.1").
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
					{Major: 2, Minor: 0, Contract: "v1alpha4"},
				},
			}),
		"infrastructure-infra2": test.NewFakeRepository().
			WithVersions("v1.0.0", "v1.0.1").
			WithMetadata("v1.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
				},
			}).
			WithMetadata("v2.0.0", &clusterctlv1.Metadata{
				ReleaseSeries: []clusterctlv1.ReleaseSeries{
					{Major: 1, Minor: 0, Contract: "v1alpha3"},
					{Major: 2, Minor: 0, Contract: "v1alpha4"},
				},
			}),
	}

	type fields struct {
		proxy        Proxy
		installQueue []repository.Components
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "install core + infra1 on an empty cluster",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{ // install core + infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "cluster-api",
						version:         "v1.0.0",
						targetNamespace: "cluster-api-system",
						providerType:    clusterctlv1.CoreProviderType,
					}),
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v1.0.0",
						targetNamespace: "infra1-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: false,
		},
		{
			name: "install infra2 on a cluster already initialized with core + infra1",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with core + infra1, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
								WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				installQueue: []repository.Components{ // install infra2, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "infra2",
						version:         "v1.0.0",
						targetNamespace: "infra2-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: false,
		},
		{
			name: "install another instance of infra1 on a cluster already initialized with core + infra1, no overlaps",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with core + infra1, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
								WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{ // install infra2, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:              "infra2",
						version:           "v1.0.0",
						targetNamespace:   "ns2",
						watchingNamespace: "ns2",
						providerType:      clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: false,
		},
		{
			name: "install another instance of infra1 on a cluster already initialized with core + infra1, same namespace of the existing infra1",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with core + infra1, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
								WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "n1", ""),
				installQueue: []repository.Components{ // install infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v1.0.0",
						targetNamespace: "n1",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
		{
			name: "install another instance of infra1 on a cluster already initialized with core + infra1, watching overlap with the existing infra1",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with core + infra1, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "cluster-api-system", "").
								WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "infra1-system", ""),
				installQueue: []repository.Components{ // install infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v1.0.0",
						targetNamespace: "infra2-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
		{
			name: "install another instance of infra1 on a cluster already initialized with core + infra1, not part of the existing management group",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with core + infra1, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1").
								WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{ // install infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:              "infra1",
						version:           "v1.0.0",
						targetNamespace:   "ns2",
						watchingNamespace: "ns2",
						providerType:      clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
		{
			name: "install an instance of infra1 on a cluster already initialized with two core, but it is part of two management group",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with two core (two management groups)
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1").
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns2", "ns2"),
				installQueue: []repository.Components{ // install infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v1.0.0",
						targetNamespace: "infra1-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
		{
			name: "install core@v1alpha3 + infra1@v1alpha4 on an empty cluster",
			fields: fields{
				proxy: test.NewFakeProxy(), //empty cluster
				installQueue: []repository.Components{ // install core + infra1, v1alpha3 contract
					newFakeComponents(fakeComponentsInput{
						name:            "cluster-api",
						version:         "v1.0.0",
						targetNamespace: "cluster-api-system",
						providerType:    clusterctlv1.CoreProviderType,
					}),
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v2.0.0",
						targetNamespace: "infra1-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
		{
			name: "install infra1@v1alpha4 on a cluster already initialized with core@v1alpha3 +",
			fields: fields{
				proxy: test.NewFakeProxy(). // cluster with one core, v1alpha3 contract
								WithProviderInventory("cluster-api", clusterctlv1.CoreProviderType, "v1.0.0", "ns1", "ns1"),
				installQueue: []repository.Components{ // install infra1, v1alpha4 contract
					newFakeComponents(fakeComponentsInput{
						name:            "infra1",
						version:         "v2.0.0",
						targetNamespace: "infra1-system",
						providerType:    clusterctlv1.InfrastructureProviderType,
					}),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configClient, _ := config.New("", config.InjectReader(fakeReader))

			i := &providerInstaller{
				configClient:      configClient,
				proxy:             tt.fields.proxy,
				providerInventory: newInventoryClient(tt.fields.proxy, nil),
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(repositoryMap[provider.ManifestLabel()]))
				},
				installQueue: tt.fields.installQueue,
			}

			err := i.Validate()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

type fakeComponents struct {
	config.Provider
	inventoryObject clusterctlv1.Provider
	instanceObjs    []unstructured.Unstructured
}

func (c *fakeComponents) Version() string {
	return ""
}

func (c *fakeComponents) Variables() []string {
	panic("not implemented")
}

func (c *fakeComponents) Images() []string {
	panic("not implemented")
}

func (c *fakeComponents) TargetNamespace() string {
	return ""
}

func (c *fakeComponents) WatchingNamespace() string {
	panic("not implemented")
}

func (c *fakeComponents) InventoryObject() clusterctlv1.Provider {
	return c.inventoryObject
}

func (c *fakeComponents) InstanceObjs() []unstructured.Unstructured {
	return c.instanceObjs
}

func (c *fakeComponents) SharedObjs() []unstructured.Unstructured {
	return []unstructured.Unstructured{}
}

func (c *fakeComponents) Yaml() ([]byte, error) {
	panic("not implemented")
}

type fakeComponentsInput struct {
	name, version, targetNamespace, watchingNamespace string
	providerType                                      clusterctlv1.ProviderType
}

type fakeComponentsOpts func(*fakeComponents)

func withInstanceObjs(o []unstructured.Unstructured) fakeComponentsOpts {
	return func(f *fakeComponents) {
		f.instanceObjs = o
	}
}

func newFakeComponents(in fakeComponentsInput, opts ...fakeComponentsOpts) repository.Components {
	inventoryObject := fakeProvider(in.name, in.providerType, in.version, in.targetNamespace, in.watchingNamespace)
	f := &fakeComponents{
		Provider:        config.NewProvider(inventoryObject.ProviderName, "", clusterctlv1.ProviderType(inventoryObject.Type)),
		inventoryObject: inventoryObject,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

func Test_shouldInstallSharedComponents(t *testing.T) {
	type args struct {
		providerList *clusterctlv1.ProviderList
		provider     clusterctlv1.Provider
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "First instance of the provider, must install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{}}, // no core provider installed
				provider:     fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, same version, must NOT install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, older version, must NOT install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v1.0.0", "", ""),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Second instance of the provider, newer version, must install shared components",
			args: args{
				providerList: &clusterctlv1.ProviderList{Items: []clusterctlv1.Provider{
					fakeProvider("core", clusterctlv1.CoreProviderType, "v2.0.0", "", ""),
				}},
				provider: fakeProvider("core", clusterctlv1.CoreProviderType, "v3.0.0", "", ""),
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := shouldInstallSharedComponents(tt.args.providerList, tt.args.provider)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

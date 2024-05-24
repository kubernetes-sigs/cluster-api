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

package test

import (
	"context"
	"errors"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	fakebootstrap "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/bootstrap"
	fakecontrolplane "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/controlplane"
	fakeexternal "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/external"
	fakeinfrastructure "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/infrastructure"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

type FakeProxy struct {
	cs        client.Client
	namespace string
	objs      []client.Object
	available *bool
}

var (
	FakeScheme = runtime.NewScheme() //nolint:revive
)

func init() {
	_ = clientgoscheme.AddToScheme(FakeScheme)
	_ = clusterctlv1.AddToScheme(FakeScheme)
	_ = clusterv1.AddToScheme(FakeScheme)
	_ = expv1.AddToScheme(FakeScheme)
	_ = addonsv1.AddToScheme(FakeScheme)
	_ = apiextensionsv1.AddToScheme(FakeScheme)
	_ = controlplanev1.AddToScheme(FakeScheme)

	_ = fakebootstrap.AddToScheme(FakeScheme)
	_ = fakecontrolplane.AddToScheme(FakeScheme)
	_ = fakeexternal.AddToScheme(FakeScheme)
	_ = fakeinfrastructure.AddToScheme(FakeScheme)
}

func (f *FakeProxy) CurrentNamespace() (string, error) {
	return f.namespace, nil
}

func (f *FakeProxy) ValidateKubernetesVersion() error {
	return nil
}

func (f *FakeProxy) GetConfig() (*rest.Config, error) {
	return nil, nil
}

func (f *FakeProxy) NewClient(_ context.Context) (client.Client, error) {
	if f.cs != nil {
		return f.cs, nil
	}
	f.cs = fake.NewClientBuilder().WithScheme(FakeScheme).WithObjects(f.objs...).Build()
	return f.cs, nil
}

func (f *FakeProxy) CheckClusterAvailable(_ context.Context) error {
	// default to considering the cluster as available unless explicitly set to be
	// unavailable.
	if f.available == nil || *f.available {
		return nil
	}
	return errors.New("cluster is not available")
}

// ListResources returns all the resources known by the FakeProxy.
func (f *FakeProxy) ListResources(_ context.Context, labels map[string]string, namespaces ...string) ([]unstructured.Unstructured, error) {
	var ret []unstructured.Unstructured //nolint:prealloc
	for _, o := range f.objs {
		u := unstructured.Unstructured{}
		err := FakeScheme.Convert(o, &u, nil)
		if err != nil {
			return nil, err
		}

		// filter by namespace, if any
		if len(namespaces) > 0 && u.GetNamespace() != "" {
			inNamespaces := false
			for _, namespace := range namespaces {
				if u.GetNamespace() == namespace {
					inNamespaces = true
					break
				}
			}
			if !inNamespaces {
				continue
			}
		}

		// filter by label, if any
		haslabel := false
		for l, v := range labels {
			for ul, uv := range u.GetLabels() {
				if l == ul && v == uv {
					haslabel = true
				}
			}
		}
		if !haslabel {
			continue
		}

		ret = append(ret, u)
	}

	return ret, nil
}

func (f *FakeProxy) GetContexts(_ string) ([]string, error) {
	return nil, nil
}

func (f *FakeProxy) GetResourceNames(_ context.Context, _, _ string, _ []client.ListOption, _ string) ([]string, error) {
	return nil, nil
}

func NewFakeProxy() *FakeProxy {
	return &FakeProxy{
		namespace: "default",
	}
}

func (f *FakeProxy) WithObjs(objs ...client.Object) *FakeProxy {
	f.objs = append(f.objs, objs...)
	return f
}

func (f *FakeProxy) WithNamespace(n string) *FakeProxy {
	f.namespace = n
	return f
}

// WithProviderInventory can be used as a fast track for setting up test scenarios requiring an already initialized management cluster.
// NB. this method adds an items to the Provider inventory, but it doesn't install the corresponding provider; if the
// test case requires the actual provider to be installed, use the fake client to install both the provider
// components and the corresponding inventory item.
func (f *FakeProxy) WithProviderInventory(name string, providerType clusterctlv1.ProviderType, version, targetNamespace string) *FakeProxy {
	f.objs = append(f.objs, &clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterctlv1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "999",
			Namespace:       targetNamespace,
			Name:            clusterctlv1.ManifestLabel(name, providerType),
			Labels: map[string]string{
				clusterctlv1.ClusterctlLabel:     "",
				clusterv1.ProviderNameLabel:      clusterctlv1.ManifestLabel(name, providerType),
				clusterctlv1.ClusterctlCoreLabel: clusterctlv1.ClusterctlCoreLabelInventoryValue,
			},
		},
		ProviderName: name,
		Type:         string(providerType),
		Version:      version,
	})

	return f
}

// WithFakeCAPISetup adds required objects in order to make kubeadm pass checks
// ensuring that management cluster has a proper release of Cluster API installed.
// NOTE: When using the fake client it is not required to install CRDs, given that type information are
// derived from the schema. However, CheckCAPIContract looks for CRDs to be installed, so this
// helper provide a way to get around to this difference between fake client and a real API server.
func (f *FakeProxy) WithFakeCAPISetup() *FakeProxy {
	f.objs = append(f.objs, FakeCAPISetupObjects()...)

	return f
}

func (f *FakeProxy) WithClusterAvailable(available bool) *FakeProxy {
	f.available = ptr.To(available)
	return f
}

// FakeCAPISetupObjects return required objects in order to make kubeadm pass checks
// ensuring that management cluster has a proper release of Cluster API installed.
func FakeCAPISetupObjects() []client.Object {
	return []client.Object{
		&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "clusters.cluster.x-k8s.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{
						Name:    clusterv1.GroupVersion.Version, // Current Cluster API contract
						Storage: true,
					},
				},
			},
		},
	}
}

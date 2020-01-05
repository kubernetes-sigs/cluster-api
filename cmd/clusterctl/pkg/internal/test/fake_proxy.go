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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type FakeProxy struct {
	cs   client.Client
	objs []runtime.Object
}

func (f *FakeProxy) CurrentNamespace() (string, error) {
	return "default", nil
}

func (f *FakeProxy) NewClient() (client.Client, error) {
	if f.cs != nil {
		return f.cs, nil
	}
	f.cs = fake.NewFakeClientWithScheme(scheme.Scheme, f.objs...)

	return f.cs, nil
}

func NewFakeProxy() *FakeProxy {
	return &FakeProxy{}
}

func (f *FakeProxy) WithObjs(objs ...runtime.Object) *FakeProxy {
	f.objs = append(f.objs, objs...)
	return f
}

// WithProviderInventory can be used as a fast track for setting up test scenarios requiring an already initialized management cluster.
// NB. this method adds an items to the Provider inventory, but it doesn't install the corresponding provider; if the
// test case requires the actual provider to be installed, use the the fake client to install both the provider
// components and the corresponding inventory item.
func (f *FakeProxy) WithProviderInventory(name string, providerType clusterctlv1.ProviderType, version, targetNamespace, watchingNamespace string) *FakeProxy {
	f.objs = append(f.objs, &clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterctlv1.GroupVersion.String(),
			Kind:       "Provider",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: targetNamespace,
			Name:      name,
		},
		Type:             string(providerType),
		Version:          version,
		WatchedNamespace: watchingNamespace,
	})

	return f
}

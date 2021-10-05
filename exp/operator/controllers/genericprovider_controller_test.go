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

package controllers

import (
	"context"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	operatorv1 "sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/operator/controllers/genericprovider"
)

const (
	testMetadata = `
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
releaseSeries:
  - major: 0
    minor: 4
    contract: v1alpha4
`
	testComponents = `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    cluster.x-k8s.io/provider: infrastructure-docker
    control-plane: controller-manager
  name: capd-controller-manager
  namespace: capd-system
spec:
  replicas: 1
  selector:
    matchLabels:
      cluster.x-k8s.io/provider: infrastructure-docker
      control-plane: controller-manager
  template:
    metadata:
      labels:
        cluster.x-k8s.io/provider: infrastructure-docker
        control-plane: controller-manager
    spec:
      containers:
      - image: gcr.io/google-samples/hello-app:1.0
        name: manager
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 200m
`
)

func insertDummyConfig(provider genericprovider.GenericProvider) {
	spec := provider.GetSpec()
	spec.FetchConfig = &operatorv1.FetchConfiguration{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"test": "dummy-config",
			},
		},
	}
	provider.SetSpec(spec)
}

func dummyConfigMap(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "v0.4.2",
			Namespace: ns,
			Labels: map[string]string{
				"test": "dummy-config",
			},
		},
		Data: map[string]string{
			"metadata":   testMetadata,
			"components": testComponents,
		},
	}
}

func TestReconcilerPreflightConditions(t *testing.T) {
	testCases := []struct {
		name      string
		namespace string
		providers []genericprovider.GenericProvider
	}{
		{
			name:      "preflight conditions for CoreProvider",
			namespace: "test-core-provider",
			providers: []genericprovider.GenericProvider{
				&genericprovider.CoreProviderWrapper{
					CoreProvider: &operatorv1.CoreProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster-api",
						},
					},
				},
			},
		},
		{
			name:      "preflight conditions for ControlPlaneProvider",
			namespace: "test-cp-provider",
			providers: []genericprovider.GenericProvider{
				&genericprovider.CoreProviderWrapper{
					CoreProvider: &operatorv1.CoreProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster-api",
						},
					},
				},
				&genericprovider.ControlPlaneProviderWrapper{
					ControlPlaneProvider: &operatorv1.ControlPlaneProvider{
						ObjectMeta: metav1.ObjectMeta{
							Name: "kubeadm",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			t.Log("creating namespace", tc.namespace)
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tc.namespace}}
			g.Expect(env.CreateAndWait(ctx, namespace)).To(Succeed())
			g.Expect(env.CreateAndWait(ctx, dummyConfigMap(tc.namespace))).To(Succeed())

			for _, p := range tc.providers {
				insertDummyConfig(p)
				p.SetNamespace(tc.namespace)
				t.Log("creating test provider", p.GetName())
				g.Expect(env.CreateAndWait(ctx, p.GetObject())).To(Succeed())
			}

			g.Eventually(func() bool {
				for _, p := range tc.providers {
					if err := env.Get(ctx, client.ObjectKeyFromObject(p.GetObject()), p.GetObject()); err != nil {
						return false
					}

					for _, cond := range p.GetStatus().Conditions {
						if cond.Type == operatorv1.PreflightCheckCondition {
							t.Log(t.Name(), p.GetName(), cond)
							if cond.Status == corev1.ConditionTrue {
								return true
							}
						}
					}
				}

				return false
			}, timeout).Should(BeEquivalentTo(true))

			objs := []client.Object{}
			for _, p := range tc.providers {
				objs = append(objs, p.GetObject())
			}
			g.Expect(env.CleanupAndWait(ctx, objs...)).To(Succeed())
		})
	}
}

func TestNewGenericProvider(t *testing.T) {
	testCases := []struct {
		name         string
		provider     client.Object
		expectError  bool
		expectedType genericprovider.GenericProvider
	}{
		{
			name:         "create new generic provider from core provider",
			provider:     &operatorv1.CoreProvider{},
			expectedType: &genericprovider.CoreProviderWrapper{},
		},
		{
			name:         "create new generic provider from bootstrap provider",
			provider:     &operatorv1.BootstrapProvider{},
			expectedType: &genericprovider.BootstrapProviderWrapper{},
		},
		{
			name:         "create new generic provider from control plane provider",
			provider:     &operatorv1.ControlPlaneProvider{},
			expectedType: &genericprovider.ControlPlaneProviderWrapper{},
		},
		{
			name:         "create new generic provider from infrastructure plane provider",
			provider:     &operatorv1.InfrastructureProvider{},
			expectedType: &genericprovider.InfrastructureProviderWrapper{},
		},
		{
			name:        "fail to create new generic provider",
			provider:    &corev1.Pod{},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			r := GenericProviderReconciler{
				Provider: tc.provider,
			}

			genericProvider, err := r.newGenericProvider()
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(reflect.TypeOf(genericProvider)).To(Equal(reflect.TypeOf(tc.expectedType)))
				g.Expect(genericProvider.GetObject()).ToNot(BeIdenticalTo(tc.provider))
			}
		})
	}
}

func TestNewGenericProviderList(t *testing.T) {
	testCases := []struct {
		name         string
		providerList client.ObjectList
		expectError  bool
		expectedType genericprovider.GenericProviderList
	}{
		{
			name:         "create new generic provider from core provider",
			providerList: &operatorv1.CoreProviderList{},
			expectedType: &genericprovider.CoreProviderListWrapper{},
		},
		{
			name:         "create new generic provider from bootstrap provider",
			providerList: &operatorv1.BootstrapProviderList{},
			expectedType: &genericprovider.BootstrapProviderListWrapper{},
		},
		{
			name:         "create new generic provider from control plane provider",
			providerList: &operatorv1.ControlPlaneProviderList{},
			expectedType: &genericprovider.ControlPlaneProviderListWrapper{},
		},
		{
			name:         "create new generic provider from infrastructure plane provider",
			providerList: &operatorv1.InfrastructureProviderList{},
			expectedType: &genericprovider.InfrastructureProviderListWrapper{},
		},
		{
			name:         "fail to create new generic provider",
			providerList: &corev1.PodList{},
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			r := GenericProviderReconciler{
				ProviderList: tc.providerList,
			}

			genericProviderList, err := r.NewGenericProviderList()
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(reflect.TypeOf(genericProviderList)).To(Equal(reflect.TypeOf(tc.expectedType)))
				g.Expect(genericProviderList.GetObject()).ToNot(BeIdenticalTo(tc.providerList))
			}
		})
	}
}

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(operatorv1.AddToScheme(scheme))
	utilruntime.Must(clusterctlv1.AddToScheme(scheme))
	return scheme
}

func TestConfigmapRepository(t *testing.T) {
	provider := &genericprovider.InfrastructureProviderWrapper{
		InfrastructureProvider: &operatorv1.InfrastructureProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aws",
				Namespace: "ns1",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "InfrastructureProvider",
				APIVersion: "operator.cluster.x-k8s.io/v1alpha1",
			},
			Spec: operatorv1.InfrastructureProviderSpec{
				ProviderSpec: operatorv1.ProviderSpec{
					FetchConfig: &operatorv1.FetchConfiguration{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"provider-components": "aws"},
						},
					},
				},
			},
		},
	}
	metadata := `
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
releaseSeries:
  - major: 0
	minor: 4
	contract: v1alpha4
  - major: 0
	minor: 3
	contract: v1alpha3`

	components := `
	apiVersion: v1
kind: Namespace
metadata:
  labels:
    cluster.x-k8s.io/provider: cluster-api
    control-plane: controller-manager
  name: capi-system
---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    cluster.x-k8s.io/provider: cluster-api
    control-plane: controller-manager
  name: capi-webhook-system
---`
	tests := []struct {
		name               string
		configMaps         []corev1.ConfigMap
		want               repository.Repository
		wantErr            string
		wantDefaultVersion string
	}{
		{
			name:    "missing configmaps",
			wantErr: "no ConfigMaps found with selector &LabelSelector{MatchLabels:map[string]string{provider-components: aws,},MatchExpressions:[]LabelSelectorRequirement{},}",
		},
		{
			name: "configmap with missing metadata",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.3",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{"components": components},
				},
			},
			wantErr: "ConfigMap ns1/v1.2.3 has no metadata",
		},
		{
			name: "configmap with missing components",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.3",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata": metadata,
					},
				},
			},
			wantErr: "ConfigMap ns1/v1.2.3 has no components",
		},
		{
			name: "configmap with invalid version in the name",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-a-version",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata": metadata,
					},
				},
			},
			wantErr: "ConfigMap ns1/not-a-version has invalid version:not-a-version (from the Name)",
		},
		{
			name: "configmap with invalid version in the Label",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-a-version",
						Namespace: "ns1",
						Labels: map[string]string{
							"provider-components":               "aws",
							"provider.cluster.x-k8s.io/version": "also-not-a-label",
						},
					},
					Data: map[string]string{
						"metadata": metadata,
					},
				},
			},
			wantErr: "ConfigMap ns1/not-a-version has invalid version:also-not-a-label (from the Label provider.cluster.x-k8s.io/version)",
		},
		{
			name: "one correct configmap",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.3",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata":   metadata,
						"components": components,
					},
				},
			},
			wantDefaultVersion: "v1.2.3",
		},
		{
			name: "one correct configmap with label version",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-provider",
						Namespace: "ns1",
						Labels: map[string]string{
							"provider-components":               "aws",
							"provider.cluster.x-k8s.io/version": "v1.2.3",
						},
					},
					Data: map[string]string{
						"metadata":   metadata,
						"components": components,
					},
				},
			},
			wantDefaultVersion: "v1.2.3",
		},
		{
			name: "three correct configmaps",
			configMaps: []corev1.ConfigMap{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.3",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata":   metadata,
						"components": components,
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.7",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata":   metadata,
						"components": components,
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "v1.2.4",
						Namespace: "ns1",
						Labels:    map[string]string{"provider-components": "aws"},
					},
					Data: map[string]string{
						"metadata":   metadata,
						"components": components,
					},
				},
			},
			wantDefaultVersion: "v1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeclient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(provider.GetObject()).Build()
			inst := &reconciler{
				ctrlClient: fakeclient,
			}

			for i := range tt.configMaps {
				g.Expect(fakeclient.Create(ctx, &tt.configMaps[i])).To(Succeed())
			}

			got, err := inst.configmapRepository(context.TODO(), provider)
			if len(tt.wantErr) > 0 {
				g.Expect(err).Should(MatchError(tt.wantErr))
				return
			}
			g.Expect(err).To(Succeed())
			g.Expect(got.GetFile(got.DefaultVersion(), got.ComponentsPath())).To(Equal([]byte(components)))
			g.Expect(got.GetFile(got.DefaultVersion(), "metadata.yaml")).To(Equal([]byte(metadata)))
			g.Expect(got.DefaultVersion()).To(Equal(tt.wantDefaultVersion))
		})
	}
}

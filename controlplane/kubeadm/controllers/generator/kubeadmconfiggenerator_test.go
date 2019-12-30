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

package generator

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
)

func TestGenerateKubeadmConfigWithOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate"
	clusterName := "foo"
	spec := &bootstrapv1.KubeadmConfigSpec{}
	owner := &metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       clusterName,
	}
	expectedReference := &corev1.ObjectReference{
		Name:       "",
		Namespace:  namespace,
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "KubeadmConfig",
	}
	expectedConfig := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    namePrefix + "-",
			Namespace:       namespace,
			ResourceVersion: "1",
			Labels:          map[string]string{clusterv1.ClusterLabelName: "foo"},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec:   *spec,
		Status: bootstrapv1.KubeadmConfigStatus{},
	}

	kcg := &KubeadmConfigGenerator{}
	got, err := kcg.GenerateKubeadmConfig(
		context.Background(),
		fakeClient,
		namespace,
		namePrefix,
		clusterName,
		spec,
		owner,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got).To(gomega.Equal(expectedReference))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, bootstrapConfig)).To(gomega.Succeed())
	g.Expect(bootstrapConfig).To(gomega.Equal(expectedConfig))
}

func TestGenerateKubeadmConfigWithoutOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generated"
	clusterName := "foo"
	spec := &bootstrapv1.KubeadmConfigSpec{}
	expectedReference := &corev1.ObjectReference{
		Name:       "",
		Namespace:  namespace,
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "KubeadmConfig",
	}
	expectedConfig := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    namePrefix + "-",
			Namespace:       namespace,
			ResourceVersion: "1",
			Labels:          map[string]string{clusterv1.ClusterLabelName: "foo"},
		},
		Spec:   *spec,
		Status: bootstrapv1.KubeadmConfigStatus{},
	}

	kcg := &KubeadmConfigGenerator{}
	got, err := kcg.GenerateKubeadmConfig(
		context.Background(),
		fakeClient,
		namespace,
		namePrefix,
		clusterName,
		spec,
		nil,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got).To(gomega.Equal(expectedReference))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, bootstrapConfig)).To(gomega.Succeed())
	g.Expect(bootstrapConfig).To(gomega.Equal(expectedConfig))
}

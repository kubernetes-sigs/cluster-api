/*
Copyright 2022 The Kubernetes Authors.

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

package log

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_AddObjectHierarchy(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	md := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "development-3961-md-0-l4zn6",
		},
	}
	mdOwnerRef := metav1.OwnerReference{
		APIVersion: md.APIVersion,
		Kind:       md.Kind,
		Name:       md.Name,
	}

	ms := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       metav1.NamespaceDefault,
			Name:            "development-3961-md-0-l4zn6-758c9b7677",
			OwnerReferences: []metav1.OwnerReference{mdOwnerRef},
		},
	}
	msOwnerRef := metav1.OwnerReference{
		APIVersion: ms.APIVersion,
		Kind:       ms.Kind,
		Name:       ms.Name,
	}

	tests := []struct {
		name                  string
		obj                   metav1.Object
		objects               []client.Object
		expectedKeysAndValues []interface{}
	}{
		{
			name: "MachineSet owning Machine is added",
			// MachineSet does not exist in Kubernetes so only MachineSet is added
			// and MachineDeployment is not.
			obj: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{msOwnerRef},
					Namespace:       metav1.NamespaceDefault,
				},
			},
			expectedKeysAndValues: []interface{}{
				"MachineSet",
				klog.ObjectRef{Namespace: ms.Namespace, Name: ms.Name},
			},
		},
		{
			name: "MachineDeployment and MachineSet owning Machine is added",
			obj: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{msOwnerRef},
					Namespace:       metav1.NamespaceDefault,
				},
			},
			objects: []client.Object{ms},
			expectedKeysAndValues: []interface{}{
				"MachineSet",
				klog.ObjectRef{Namespace: ms.Namespace, Name: ms.Name},
				"MachineDeployment",
				klog.ObjectRef{Namespace: md.Namespace, Name: md.Name},
			},
		},
		{
			name: "MachineDeployment owning MachineSet is added",
			obj: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{mdOwnerRef},
					Namespace:       metav1.NamespaceDefault,
				},
			},
			expectedKeysAndValues: []interface{}{
				"MachineDeployment",
				klog.ObjectRef{Namespace: md.Namespace, Name: md.Name},
			},
		},
		{
			name: "KubeadmControlPlane and Machine owning DockerMachine are added",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "DockerMachine",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": clusterv1.GroupVersion.String(),
								"kind":       "Machine",
								"name":       "development-3961-4flkb-gzxnb",
							},
							map[string]interface{}{
								"apiVersion": clusterv1.GroupVersion.String(),
								"kind":       "KubeadmControlPlane",
								"name":       "development-3961-4flkb",
							},
						},
						"namespace": metav1.NamespaceDefault,
					},
				},
			},
			expectedKeysAndValues: []interface{}{
				"Machine",
				klog.ObjectRef{Namespace: metav1.NamespaceDefault, Name: "development-3961-4flkb-gzxnb"},
				"KubeadmControlPlane",
				klog.ObjectRef{Namespace: metav1.NamespaceDefault, Name: "development-3961-4flkb"},
			},
		},
		{
			name: "Duplicate Cluster ownerRef should be deduplicated",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "DockerCluster",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": clusterv1.GroupVersion.String(),
								"kind":       "Cluster",
								"name":       "development-3961",
							},
							map[string]interface{}{
								"apiVersion": clusterv1.GroupVersion.String(),
								"kind":       "Cluster",
								"name":       "development-3961",
							},
						},
						"namespace": metav1.NamespaceDefault,
					},
				},
			},
			expectedKeysAndValues: []interface{}{
				"Cluster",
				klog.ObjectRef{Namespace: metav1.NamespaceDefault, Name: "development-3961"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			// Create fake log sink so we can later verify the added k/v pairs.
			ctx := ctrl.LoggerInto(context.Background(), logr.New(&fakeLogSink{}))

			_, logger, err := AddOwners(ctx, c, tt.obj)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(logger.GetSink().(fakeLogSink).keysAndValues).To(Equal(tt.expectedKeysAndValues))
		})
	}
}

type fakeLogSink struct {
	// Embedding NullLogSink so we don't have to implement all funcs
	// of the LogSink interface.
	log.NullLogSink
	keysAndValues []interface{}
}

// WithValues stores keysAndValues so we can later check if the
// right keysAndValues have been added.
func (f fakeLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	f.keysAndValues = keysAndValues
	return f
}

/*
Copyright 2024 The Kubernetes Authors.

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

// Package finalizers implements finalizer helper functions.
package finalizers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func TestEnsureFinalizer(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	testFinalizer := "cluster.x-k8s.io/test-finalizer"

	tests := []struct {
		name                  string
		obj                   client.Object
		wantErr               bool
		wantFinalizersUpdated bool
		wantFinalizer         bool
	}{
		{
			name: "should not add finalizer if object has deletionTimestamp",
			obj: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{"some-other-finalizer"},
				},
			},
			wantErr:               false,
			wantFinalizersUpdated: false,
			wantFinalizer:         false,
		},
		{
			name: "should not add finalizer if finalizer is already set",
			obj: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cluster",
					Finalizers: []string{testFinalizer},
				},
			},
			wantErr:               false,
			wantFinalizersUpdated: false,
			wantFinalizer:         true,
		},
		{
			name: "should add finalizer if finalizer is not already set",
			obj: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cluster",
					Finalizers: []string{"some-other-finalizer"},
				},
			},
			wantErr:               false,
			wantFinalizersUpdated: true,
			wantFinalizer:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.obj).Build()

			gotFinalizersUpdated, gotErr := EnsureFinalizer(ctx, c, tt.obj, testFinalizer)
			g.Expect(gotErr != nil).To(Equal(tt.wantErr))
			g.Expect(gotFinalizersUpdated).To(Equal(tt.wantFinalizersUpdated))

			gotObj := tt.obj.DeepCopyObject().(client.Object)
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(gotObj), gotObj)).To(Succeed())
			g.Expect(controllerutil.ContainsFinalizer(gotObj, testFinalizer)).To(Equal(tt.wantFinalizer))
		})
	}
}

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

package controllers

import (
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeadmv1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	v1alpha2.AddToScheme(scheme)
	kubeadmv1alpha2.AddToScheme(scheme)
	return scheme
}

func TestSuccessfulReconcileShouldNotRequeue(t *testing.T) {
	objects := []runtime.Object{
		&kubeadmv1alpha2.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "cfg",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       "Machine",
						APIVersion: v1alpha2.SchemeGroupVersion.String(),
						Name:       "my-machine",
					},
				},
			},
		},
		&v1alpha2.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "my-machine",
				Labels: map[string]string{
					v1alpha2.MachineClusterLabelName: "my-cluster",
				},
			},
		},
		&v1alpha2.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "my-cluster",
			},
			Status: v1alpha2.ClusterStatus{
				InfrastructureReady: true,
				APIEndpoints: []v1alpha2.APIEndpoint{
					{
						Host: "example.com",
						Port: 6443,
					},
				},
			},
		},
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.ZapLogger(true),
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

func TestNoErrorIfNoMachineRefIsFound(t *testing.T) {
	objects := []runtime.Object{
		&kubeadmv1alpha2.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "some non machine kind",
					},
				},
			},
		},
	}
	myclient := fake.NewFakeClientWithScheme(setupScheme(), objects...)

	k := &KubeadmConfigReconciler{
		Log:    log.ZapLogger(true),
		Client: myclient,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "ns",
			Name:      "cfg",
		},
	}
	result, err := k.Reconcile(request)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to reconcile:\n %+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

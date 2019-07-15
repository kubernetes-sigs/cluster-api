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
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeadmv1alpha1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type log struct{}

func (l *log) Info(msg string, keysAndValues ...interface{})             {}
func (l *log) Enabled() bool                                             { return false }
func (l *log) Error(err error, msg string, keysAndValues ...interface{}) {}
func (l *log) V(level int) logr.InfoLogger                               { return l }
func (l *log) WithValues(keysAndValues ...interface{}) logr.Logger       { return l }
func (l *log) WithName(name string) logr.Logger                          { return l }

type myClient struct {
	db map[string]runtime.Object
}

// Very basic implementation of copying pointers of interfaces around, stolen from controller runtime
func (c *myClient) Get(ctx context.Context, key client.ObjectKey, out runtime.Object) error {
	obj, ok := c.db[key.String()]
	if !ok {
		return errors.New("object not found")
	}
	obj = obj.(runtime.Object).DeepCopyObject()
	outVal := reflect.ValueOf(out)
	objVal := reflect.ValueOf(obj)
	reflect.Indirect(outVal).Set(reflect.Indirect(objVal))
	return nil
}
func (c *myClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOptionFunc) error {
	return nil
}
func (c *myClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOptionFunc) error {
	return nil
}
func (c *myClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOptionFunc) error {
	return nil
}
func (c *myClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOptionFunc) error {
	return nil
}
func (c *myClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOptionFunc) error {
	return nil
}
func (c *myClient) Status() client.StatusWriter {
	return c
}

func TestSuccessfulReconcileShouldNotRequeue(t *testing.T) {
	machineKind := v1alpha2.SchemeGroupVersion.WithKind("Machine").String()

	myclient := &myClient{
		db: map[string]runtime.Object{
			"ns/cfg": &kubeadmv1alpha1.KubeadmBootstrapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "cfg",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: machineKind,
						},
					},
				},
			},
		},
	}

	k := &KubeadmBootstrapConfigReconciler{
		Log:    &log{},
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
		t.Fatal(fmt.Sprintf("%+v", err))
	}
	if result.Requeue == true {
		t.Fatal("did not expect to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

func TestRequeueIfNoMachineRefIsFound(t *testing.T) {
	myclient := &myClient{
		db: map[string]runtime.Object{
			"ns/cfg": &kubeadmv1alpha1.KubeadmBootstrapConfig{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "some non machine kind",
						},
					},
				},
			},
		},
	}

	k := &KubeadmBootstrapConfigReconciler{
		Log:    &log{},
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
		t.Fatal(fmt.Sprintf("%+v", err))
	}
	if result.Requeue == false {
		t.Fatal("expected to requeue")
	}
	if result.RequeueAfter != time.Duration(0) {
		t.Fatal("did not expect to requeue after")
	}
}

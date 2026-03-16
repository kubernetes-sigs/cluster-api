/*
Copyright 2025 The Kubernetes Authors.

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

package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewFakeController() *FakeController {
	return &FakeController{
		Deferrals: map[reconcile.Request]time.Time{},
	}
}

type FakeController struct {
	controller.Controller
	Deferrals map[reconcile.Request]time.Time
}

func (f *FakeController) DeferNextReconcile(req reconcile.Request, reconcileAfter time.Time) {
	f.Deferrals[req] = reconcileAfter
}

func (f *FakeController) DeferNextReconcileForObject(obj metav1.Object, reconcileAfter time.Time) {
	f.Deferrals[reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
	}] = reconcileAfter
}

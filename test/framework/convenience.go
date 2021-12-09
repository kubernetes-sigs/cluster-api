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

package framework

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

// TryAddDefaultSchemes tries to add the following schemes:
//   * Kubernetes corev1
//   * Kubernetes appsv1
//   * CAPI core
//   * Kubeadm Bootstrapper
//   * Kubeadm ControlPlane
// Any error that occurs when trying to add the schemes is ignored.
func TryAddDefaultSchemes(scheme *runtime.Scheme) {
	// Add the core schemes.
	_ = corev1.AddToScheme(scheme)

	// Add the apps schemes.
	_ = appsv1.AddToScheme(scheme)

	// Add the core CAPI scheme.
	_ = clusterv1.AddToScheme(scheme)

	// Add the CAPI experiments scheme.
	_ = expv1.AddToScheme(scheme)
	_ = addonsv1.AddToScheme(scheme)

	// Add the core CAPI v1alpha3 scheme.
	_ = clusterv1alpha3.AddToScheme(scheme)

	// Add the kubeadm bootstrapper scheme.
	_ = bootstrapv1.AddToScheme(scheme)

	// Add the kubeadm controlplane scheme.
	_ = controlplanev1.AddToScheme(scheme)

	// Add the api extensions (CRD) to the scheme.
	_ = apiextensionsv1beta.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	// Add rbac to the scheme.
	_ = rbacv1.AddToScheme(scheme)
}

// ObjectToKind returns the Kind without the package prefix. Pass in a pointer to a struct
// This will panic if used incorrectly.
func ObjectToKind(i runtime.Object) string {
	return reflect.ValueOf(i).Elem().Type().Name()
}

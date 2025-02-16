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

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
)

// TryAddDefaultSchemes tries to add the following schemes:
//   - Kubernetes corev1
//   - Kubernetes appsv1
//   - CAPI core
//   - Kubeadm Bootstrapper
//   - Kubeadm ControlPlane
//
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

	// Add the CAPI clusterctl scheme.
	_ = clusterctlv1.AddToScheme(scheme)

	// Add the kubeadm bootstrapper scheme.
	_ = bootstrapv1.AddToScheme(scheme)

	// Add the kubeadm controlplane scheme.
	_ = controlplanev1.AddToScheme(scheme)

	// Add the IPAM scheme.
	_ = ipamv1.AddToScheme(scheme)

	// Add the api extensions (CRD) to the scheme.
	_ = apiextensionsv1beta.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	// Add the admission registration scheme (Mutating-, ValidatingWebhookConfiguration).
	_ = admissionregistrationv1.AddToScheme(scheme)

	// Add RuntimeSDK to the scheme.
	_ = runtimev1.AddToScheme(scheme)

	// Add rbac to the scheme.
	_ = rbacv1.AddToScheme(scheme)

	// Add coordination to the schema
	// Note: This is e.g. used to trigger kube-controller-manager restarts by stealing its lease.
	_ = coordinationv1.AddToScheme(scheme)

	// Add storagev1 to the scheme
	_ = storagev1.AddToScheme(scheme)
}

// ObjectToKind returns the Kind without the package prefix. Pass in a pointer to a struct
// This will panic if used incorrectly.
func ObjectToKind(i runtime.Object) string {
	return reflect.ValueOf(i).Elem().Type().Name()
}

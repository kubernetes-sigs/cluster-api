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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallComponents is a helper function that applies components, generally to a management cluster.
func InstallComponents(ctx context.Context, mgmt Applier, components ...ComponentGenerator) {
	Describe("Installing the provider components", func() {
		for _, component := range components {
			By(fmt.Sprintf("installing %s", component.GetName()))
			c, err := component.Manifests(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgmt.Apply(ctx, c)).To(Succeed())
		}
	})
}

// WaitForAPIServiceAvailable will wait for an an APIService to be available.
// For example, kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io
func WaitForAPIServiceAvailable(ctx context.Context, mgmt Waiter, serviceName string) {
	By(fmt.Sprintf("waiting for api service %q to be available", serviceName))
	err := mgmt.Wait(ctx, "--for", "condition=Available", "--timeout", "300s", "apiservice", serviceName)
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)
}

// WaitForPodsReadyInNamespace will wait for all pods to be Ready in the
// specified namespace.
// For example, kubectl wait --for=condition=Ready --timeout=300s --namespace capi-system pods --all
func WaitForPodsReadyInNamespace(ctx context.Context, cluster Waiter, namespace string) {
	By(fmt.Sprintf("waiting for pods to be ready in namespace %q", namespace))
	err := cluster.Wait(ctx, "--for", "condition=Ready", "--timeout", "300s", "--namespace", namespace, "pods", "--all")
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)
}

// EnsureNamespace verifies if a namespaces exists. If it doesn't it will
// create the namespace.
func EnsureNamespace(ctx context.Context, mgmt client.Client, namespace string) {
	ns := &corev1.Namespace{}
	err := mgmt.Get(ctx, client.ObjectKey{Name: namespace}, ns)
	if err != nil && apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(mgmt.Create(ctx, ns)).To(Succeed())
	} else {
		Fail(err.Error())
	}
}

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

	// Add the experiments CAPI scheme.
	_ = expv1.AddToScheme(scheme)

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

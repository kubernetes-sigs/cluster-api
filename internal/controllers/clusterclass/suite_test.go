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

package clusterclass

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/component-base/featuregate"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
	env        *envtest.Environment
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
}
func TestMain(m *testing.M) {
	if err := feature.Gates.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", feature.ClusterTopology, true)); err != nil {
		panic(fmt.Sprintf("unable to set ClusterTopology feature gate: %v", err))
	}
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}
	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		if err := (&Reconciler{
			Client:        mgr.GetClient(),
			RuntimeClient: fakeruntimeclient.NewRuntimeClientBuilder().Build(),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 5}); err != nil {
			panic(fmt.Sprintf("unable to create clusterclass reconciler: %v", err))
		}
	}
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(30 * time.Second)
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                   m,
		ManagerUncachedObjs: []client.Object{},
		SetupEnv:            func(e *envtest.Environment) { env = e },
		SetupIndexes:        setupIndexes,
		SetupReconcilers:    setupReconcilers,
	}))
}

// ownerReferenceTo converts an object to an OwnerReference.
// Note: We pass in gvk explicitly as we can't rely on GVK being set on all objects
// (only on Unstructured).
func ownerReferenceTo(obj client.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}
}

// referenceExistsWithCorrectKindAndAPIVersion asserts that the passed ObjectReference is not nil and that it has the correct kind and apiVersion.
func referenceExistsWithCorrectKindAndAPIVersion(reference *corev1.ObjectReference, kind string, apiVersion schema.GroupVersion) error {
	if reference == nil {
		return fmt.Errorf("object reference passed was nil")
	}
	if reference.Kind != kind {
		return fmt.Errorf("object reference kind %v does not match expected %v", reference.Kind, kind)
	}
	if reference.APIVersion != apiVersion.String() {
		return fmt.Errorf("apiVersion %v does not match expected %v", reference.APIVersion, apiVersion.String())
	}
	return nil
}

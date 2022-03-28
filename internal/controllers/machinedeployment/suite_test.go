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

package machinedeployment

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	// +kubebuilder:scaffold:imports
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/remote"
	machinesetcontroller "sigs.k8s.io/cluster-api/internal/controllers/machineset"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

const (
	timeout = time.Second * 30
)

var (
	env        *envtest.Environment
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
}

func TestMain(m *testing.M) {
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	setupReconcilers := func(ctx context.Context, mgr ctrl.Manager) {
		// Set up a ClusterCacheTracker and ClusterCacheReconciler to provide to controllers
		// requiring a connection to a remote cluster
		log := ctrl.Log.WithName("remote").WithName("ClusterCacheTracker")
		tracker, err := remote.NewClusterCacheTracker(
			mgr,
			remote.ClusterCacheTrackerOptions{
				Log:     &log,
				Indexes: remote.DefaultIndexes,
			},
		)
		if err != nil {
			panic(fmt.Sprintf("unable to create cluster cache tracker: %v", err))
		}
		if err := (&remote.ClusterCacheReconciler{
			Client:  mgr.GetClient(),
			Tracker: tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start ClusterCacheReconciler: %v", err))
		}
		if err := (&machinesetcontroller.Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
			Tracker:   tracker,
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineSetReconciler: %v", err))
		}
		if err := (&Reconciler{
			Client:    mgr.GetClient(),
			APIReader: mgr.GetAPIReader(),
		}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
			panic(fmt.Sprintf("Failed to start MMachineDeploymentReconciler: %v", err))
		}
	}

	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultEventuallyTimeout(timeout)

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                m,
		SetupEnv:         func(e *envtest.Environment) { env = e },
		SetupIndexes:     setupIndexes,
		SetupReconcilers: setupReconcilers,
	}))
}

func intOrStrPtr(i int32) *intstr.IntOrString {
	// FromInt takes an int that must not be greater than int32...
	res := intstr.FromInt(int(i))
	return &res
}

func fakeInfrastructureRefReady(ref corev1.ObjectReference, base map[string]interface{}, g *WithT) string {
	iref := (&unstructured.Unstructured{Object: base}).DeepCopy()
	g.Eventually(func() error {
		return env.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, iref)
	}).Should(Succeed())

	irefPatch := client.MergeFrom(iref.DeepCopy())
	providerID := fmt.Sprintf("test:////%v", uuid.NewUUID())
	g.Expect(unstructured.SetNestedField(iref.Object, providerID, "spec", "providerID")).To(Succeed())
	g.Expect(env.Patch(ctx, iref, irefPatch)).To(Succeed())

	irefPatch = client.MergeFrom(iref.DeepCopy())
	g.Expect(unstructured.SetNestedField(iref.Object, true, "status", "ready")).To(Succeed())
	g.Expect(env.Status().Patch(ctx, iref, irefPatch)).To(Succeed())
	return providerID
}

func fakeMachineNodeRef(m *clusterv1.Machine, pid string, g *WithT) {
	g.Eventually(func() error {
		key := client.ObjectKey{Name: m.Name, Namespace: m.Namespace}
		return env.Get(ctx, key, &clusterv1.Machine{})
	}).Should(Succeed())

	if m.Status.NodeRef != nil {
		return
	}

	// Create a new fake Node.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: m.Name + "-",
		},
		Spec: corev1.NodeSpec{
			ProviderID: pid,
		},
	}
	g.Expect(env.Create(ctx, node)).To(Succeed())

	g.Eventually(func() error {
		key := client.ObjectKey{Name: node.Name, Namespace: node.Namespace}
		return env.Get(ctx, key, &corev1.Node{})
	}).Should(Succeed())

	// Patch the node and make it look like ready.
	patchNode := client.MergeFrom(node.DeepCopy())
	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue})
	g.Expect(env.Status().Patch(ctx, node, patchNode)).To(Succeed())

	// Patch the Machine.
	patchMachine := client.MergeFrom(m.DeepCopy())
	m.Spec.ProviderID = pointer.StringPtr(pid)
	g.Expect(env.Patch(ctx, m, patchMachine)).To(Succeed())

	patchMachine = client.MergeFrom(m.DeepCopy())
	m.Status.NodeRef = &corev1.ObjectReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
	}
	g.Expect(env.Status().Patch(ctx, m, patchMachine)).To(Succeed())
}

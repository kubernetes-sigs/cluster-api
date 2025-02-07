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

package crdmigrator

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	t1v1beta1 "sigs.k8s.io/cluster-api/controllers/crdmigrator/test/t1/v1beta1"
	t2v1beta2 "sigs.k8s.io/cluster-api/controllers/crdmigrator/test/t2/v1beta2"
	t3v1beta2 "sigs.k8s.io/cluster-api/controllers/crdmigrator/test/t3/v1beta2"
	t4v1beta2 "sigs.k8s.io/cluster-api/controllers/crdmigrator/test/t4/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

func TestReconcile(t *testing.T) {
	crdName := "testclusters.test.cluster.x-k8s.io"
	crdObjectKey := client.ObjectKey{Name: crdName}

	tests := []struct {
		name                   string
		skipCRDMigrationPhases sets.Set[string]
		useCache               bool
	}{
		{
			name:                   "run both StorageVersionMigration and CleanupManagedFields with cache",
			skipCRDMigrationPhases: nil,
			useCache:               true,
		},
		{
			name:                   "run both StorageVersionMigration and CleanupManagedFields without cache",
			skipCRDMigrationPhases: nil,
			useCache:               false,
		},
		{
			name:                   "run only CleanupManagedFields with cache",
			skipCRDMigrationPhases: sets.Set[string]{}.Insert(string(StorageVersionMigrationPhase)),
			useCache:               true,
		},
		{
			name:                   "run only CleanupManagedFields without cache",
			skipCRDMigrationPhases: sets.Set[string]{}.Insert(string(StorageVersionMigrationPhase)),
			useCache:               false,
		},
		{
			name:                   "run only StorageVersionMigration with cache",
			skipCRDMigrationPhases: sets.Set[string]{}.Insert(string(CleanupManagedFieldsPhase)),
			useCache:               true,
		},
		{
			name:                   "run only StorageVersionMigration without cache",
			skipCRDMigrationPhases: sets.Set[string]{}.Insert(string(CleanupManagedFieldsPhase)),
			useCache:               false,
		},
		{
			name:                   "skip all",
			skipCRDMigrationPhases: sets.Set[string]{}.Insert(string(AllPhase)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fieldOwner := client.FieldOwner("unit-test-client")

			// The test will go through the entire lifecycle of an apiVersion
			//
			// T1: only v1beta1 exists
			// * v1beta1: served: true, storage: true
			//
			// T2: v1beta2 is added
			// * v1beta1: served: true, storage: false
			// * v1beta2: served: true, storage: true
			//
			// T3: v1beta1 is unserved
			// * v1beta1: served: false, storage: false
			// * v1beta2: served: true, storage: true
			//
			// T4: v1beta1 is removed
			// * v1beta2: served: true, storage: true

			// Create manager for all steps.
			skipCRDMigrationPhases := strings.Join(tt.skipCRDMigrationPhases.UnsortedList(), ",")
			managerT1, err := createManagerWithCRDMigrator(skipCRDMigrationPhases, map[client.Object]ByObjectConfig{
				&t1v1beta1.TestCluster{}: {UseCache: tt.useCache},
			}, t1v1beta1.AddToScheme)
			g.Expect(err).ToNot(HaveOccurred())
			managerT2, err := createManagerWithCRDMigrator(skipCRDMigrationPhases, map[client.Object]ByObjectConfig{
				&t2v1beta2.TestCluster{}: {UseCache: tt.useCache},
			}, t2v1beta2.AddToScheme)
			g.Expect(err).ToNot(HaveOccurred())
			managerT3, err := createManagerWithCRDMigrator(skipCRDMigrationPhases, map[client.Object]ByObjectConfig{
				&t3v1beta2.TestCluster{}: {UseCache: tt.useCache},
			}, t3v1beta2.AddToScheme)
			g.Expect(err).ToNot(HaveOccurred())
			managerT4, err := createManagerWithCRDMigrator(skipCRDMigrationPhases, map[client.Object]ByObjectConfig{
				&t4v1beta2.TestCluster{}: {UseCache: tt.useCache},
			}, t4v1beta2.AddToScheme)
			g.Expect(err).ToNot(HaveOccurred())

			defer func() {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				crd.SetName(crdName)
				g.Expect(env.CleanupAndWait(ctx, crd)).To(Succeed())
			}()

			t.Logf("T1: Install CRDs")
			g.Expect(installCRDs(ctx, env.GetClient(), "test/t1/crd")).To(Succeed())
			validateStoredVersions(g, crdObjectKey, "v1beta1")

			t.Logf("T1: Start Manager")
			cancelManager, managerStopped := startManager(ctx, managerT1)

			t.Logf("T1: Validate")
			if tt.skipCRDMigrationPhases.Has(string(AllPhase)) {
				// If all phases are skipped, the controller should do nothing.
				g.Consistently(func(g Gomega) {
					crd := &apiextensionsv1.CustomResourceDefinition{}
					g.Expect(env.GetAPIReader().Get(ctx, crdObjectKey, crd)).To(Succeed())
					g.Expect(crd.Status.StoredVersions).To(ConsistOf("v1beta1"))
					g.Expect(crd.Annotations).ToNot(HaveKey(clusterv1.CRDMigrationObservedGenerationAnnotation))
				}).WithTimeout(2 * time.Second).Should(Succeed())
				stopManager(cancelManager, managerStopped)
				return
			}
			// Stored versions didn't change.
			validateStoredVersions(g, crdObjectKey, "v1beta1")
			validateObservedGeneration(g, crdObjectKey, 1)

			// Deploy test-cluster-1 and test-cluster-2.
			testClusterT1 := unstructuredTestCluster("test-cluster-1", t1v1beta1.GroupVersion.WithKind("TestCluster"))
			g.Expect(unstructured.SetNestedField(testClusterT1.Object, "foo-value", "spec", "foo")).To(Succeed())
			g.Expect(managerT1.GetClient().Patch(ctx, testClusterT1, client.Apply, fieldOwner)).To(Succeed())
			testClusterT1 = unstructuredTestCluster("test-cluster-2", t1v1beta1.GroupVersion.WithKind("TestCluster"))
			g.Expect(unstructured.SetNestedField(testClusterT1.Object, "foo-value", "spec", "foo")).To(Succeed())
			g.Expect(managerT1.GetClient().Patch(ctx, testClusterT1, client.Apply, fieldOwner)).To(Succeed())
			validateManagedFields(g, "v1beta1", map[string][]string{
				"test-cluster-1": {"test.cluster.x-k8s.io/v1beta1"},
				"test-cluster-2": {"test.cluster.x-k8s.io/v1beta1"},
			})

			t.Logf("T1: Stop Manager")
			stopManager(cancelManager, managerStopped)

			t.Logf("T2: Install CRDs")
			g.Expect(installCRDs(ctx, env.GetClient(), "test/t2/crd")).To(Succeed())
			validateStoredVersions(g, crdObjectKey, "v1beta1", "v1beta2")

			t.Logf("T2: Start Manager")
			cancelManager, managerStopped = startManager(ctx, managerT2)

			if tt.skipCRDMigrationPhases.Has(string(StorageVersionMigrationPhase)) {
				// If storage version migration is skipped, stored versions didn't change.
				validateStoredVersions(g, crdObjectKey, "v1beta1", "v1beta2")
			} else {
				// If storage version migration is run, CRs are now stored as v1beta2.
				validateStoredVersions(g, crdObjectKey, "v1beta2")
			}
			validateObservedGeneration(g, crdObjectKey, 2)

			// Set an additional field with a different field manager and v1beta2 apiVersion in test-cluster-2
			testClusterT2 := unstructuredTestCluster("test-cluster-2", t2v1beta2.GroupVersion.WithKind("TestCluster"))
			g.Expect(unstructured.SetNestedField(testClusterT2.Object, "bar-value", "spec", "bar")).To(Succeed())
			g.Expect(managerT2.GetClient().Patch(ctx, testClusterT2, client.Apply, client.FieldOwner("different-unit-test-client"))).To(Succeed())
			// Deploy test-cluster-3.
			testClusterT2 = unstructuredTestCluster("test-cluster-3", t2v1beta2.GroupVersion.WithKind("TestCluster"))
			g.Expect(unstructured.SetNestedField(testClusterT2.Object, "foo-value", "spec", "foo")).To(Succeed())
			g.Expect(managerT2.GetClient().Patch(ctx, testClusterT2, client.Apply, fieldOwner)).To(Succeed())
			// At this point we have clusters with all combinations of managedField apiVersions.
			validateManagedFields(g, "v1beta2", map[string][]string{
				"test-cluster-1": {"test.cluster.x-k8s.io/v1beta1"},
				"test-cluster-2": {"test.cluster.x-k8s.io/v1beta1", "test.cluster.x-k8s.io/v1beta2"},
				"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
			})

			t.Logf("T2: Stop Manager")
			stopManager(cancelManager, managerStopped)

			t.Logf("T3: Install CRDs")
			g.Expect(installCRDs(ctx, env.GetClient(), "test/t3/crd")).To(Succeed())
			// Stored versions didn't change.
			if tt.skipCRDMigrationPhases.Has(string(StorageVersionMigrationPhase)) {
				validateStoredVersions(g, crdObjectKey, "v1beta1", "v1beta2")
			} else {
				validateStoredVersions(g, crdObjectKey, "v1beta2")
			}

			t.Logf("T3: Start Manager")
			cancelManager, managerStopped = startManager(ctx, managerT3)

			// Stored versions didn't change.
			if tt.skipCRDMigrationPhases.Has(string(StorageVersionMigrationPhase)) {
				validateStoredVersions(g, crdObjectKey, "v1beta1", "v1beta2")
			} else {
				validateStoredVersions(g, crdObjectKey, "v1beta2")
			}
			validateObservedGeneration(g, crdObjectKey, 3)

			if tt.skipCRDMigrationPhases.Has(string(CleanupManagedFieldsPhase)) {
				// If managedField cleanup is skipped, managedField apiVersions didn't change.
				validateManagedFields(g, "v1beta2", map[string][]string{
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta1"},
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta1", "test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			} else {
				// If managedField cleanup is run, CRs now only have v1beta2 managedFields.
				validateManagedFields(g, "v1beta2", map[string][]string{
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			}

			t.Logf("T3: Stop Manager")
			stopManager(cancelManager, managerStopped)

			t.Logf("T4: Install CRDs")
			err = installCRDs(ctx, env.GetClient(), "test/t4/crd")
			if tt.skipCRDMigrationPhases.Has(string(StorageVersionMigrationPhase)) {
				// If storage version migration was skipped before, we now cannot deploy CRDs that remove v1beta1.
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("status.storedVersions[0]: Invalid value: \"v1beta1\": must appear in spec.versions"))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			validateStoredVersions(g, crdObjectKey, "v1beta2")

			t.Logf("T4: Start Manager")
			cancelManager, managerStopped = startManager(ctx, managerT4)

			// Stored versions didn't change.
			validateStoredVersions(g, crdObjectKey, "v1beta2")
			validateObservedGeneration(g, crdObjectKey, 4)

			// managedField apiVersions didn't change.
			// This also verifies we can still read the test-cluster CRs, which means the CRs are now stored in v1beta2.
			if tt.skipCRDMigrationPhases.Has(string(CleanupManagedFieldsPhase)) {
				validateManagedFields(g, "v1beta2", map[string][]string{
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta1"},
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta1", "test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			} else {
				validateManagedFields(g, "v1beta2", map[string][]string{
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			}

			for _, clusterName := range []string{"test-cluster-1", "test-cluster-2", "test-cluster-3"} {
				// Try to patch the test-clusters CRs with SSA.
				testClusterT4 := unstructuredTestCluster(clusterName, t4v1beta2.GroupVersion.WithKind("TestCluster"))
				g.Expect(unstructured.SetNestedField(testClusterT4.Object, "new-foo-value", "spec", "foo")).To(Succeed())
				err = managerT4.GetClient().Patch(ctx, testClusterT4, client.Apply, fieldOwner)

				// If managedField cleanup was skipped before, the SSA patch will fail for the clusters which still have v1beta1 managedFields.
				if tt.skipCRDMigrationPhases.Has(string(CleanupManagedFieldsPhase)) && (clusterName == "test-cluster-1" || clusterName == "test-cluster-2") {
					g.Expect(err).To(HaveOccurred())
					g.Expect(err.Error()).To(ContainSubstring("request to convert CR to an invalid group/version: test.cluster.x-k8s.io/v1beta1"))
					continue
				}
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.skipCRDMigrationPhases.Has(string(CleanupManagedFieldsPhase)) {
				// managedField apiVersions didn't change.
				validateManagedFields(g, "v1beta2", map[string][]string{
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta1"},
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta1", "test.cluster.x-k8s.io/v1beta2"},
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			} else {
				validateManagedFields(g, "v1beta2", map[string][]string{
					// 1 entry .spec.foo (field manager: unit-test-client)
					"test-cluster-1": {"test.cluster.x-k8s.io/v1beta2"},
					// 1 entry .spec.foo (field manager: unit-test-client), 1 entry for .spec.bar (field manager: different-unit-test-client)
					"test-cluster-2": {"test.cluster.x-k8s.io/v1beta2", "test.cluster.x-k8s.io/v1beta2"},
					// 1 entry .spec.foo (field manager: unit-test-client)
					"test-cluster-3": {"test.cluster.x-k8s.io/v1beta2"},
				})
			}

			t.Logf("T4: Stop Manager")
			stopManager(cancelManager, managerStopped)
		})
	}
}

func unstructuredTestCluster(clusterName string, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(metav1.NamespaceDefault)
	u.SetName(clusterName)
	return u
}

func validateStoredVersions(g *WithT, crdObjectKey client.ObjectKey, storedVersions ...string) {
	g.Eventually(func(g Gomega) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		g.Expect(env.GetAPIReader().Get(ctx, crdObjectKey, crd)).To(Succeed())
		g.Expect(crd.Status.StoredVersions).To(ConsistOf(storedVersions))
	}).WithTimeout(5 * time.Second).Should(Succeed())
}

func validateObservedGeneration(g *WithT, crdObjectKey client.ObjectKey, observedGeneration int) {
	g.Eventually(func(g Gomega) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		g.Expect(env.GetAPIReader().Get(ctx, crdObjectKey, crd)).To(Succeed())
		g.Expect(crd.Annotations[clusterv1.CRDMigrationObservedGenerationAnnotation]).To(Equal(strconv.Itoa(observedGeneration)))
	}).WithTimeout(5 * time.Second).Should(Succeed())
}

func validateManagedFields(g *WithT, apiVersion string, expectedManagedFields map[string][]string) {
	// Create a client that has v1beta1 & v1beta2 TestCluster registered in its scheme.
	scheme := runtime.NewScheme()
	_ = t2v1beta2.AddToScheme(scheme)
	restConfig := env.GetConfig()
	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	g.Expect(err).ToNot(HaveOccurred())

	for clusterName, expectedAPIVersions := range expectedManagedFields {
		testCluster := unstructuredTestCluster(clusterName, schema.GroupVersionKind{
			Group:   "test.cluster.x-k8s.io",
			Version: apiVersion,
			Kind:    "TestCluster",
		})
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(testCluster), testCluster)).To(Succeed())

		g.Expect(testCluster.GetManagedFields()).To(HaveLen(len(expectedAPIVersions)))
		for _, expectedAPIVersion := range expectedAPIVersions {
			g.Expect(testCluster.GetManagedFields()).To(ContainElement(HaveField("APIVersion", expectedAPIVersion)))
		}
	}
}

func createManagerWithCRDMigrator(skipCRDMigrationPhases string, crdMigratorConfig map[client.Object]ByObjectConfig, addToSchemeFuncs ...func(s *runtime.Scheme) error) (manager.Manager, error) {
	// Create scheme and register the test-cluster types.
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	for _, f := range addToSchemeFuncs {
		if err := f(scheme); err != nil {
			return nil, err
		}
	}

	req, _ := labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Exists, nil)
	clusterSecretCacheSelector := labels.NewSelector().Add(*req)
	options := manager.Options{
		Controller: config.Controller{
			SkipNameValidation: ptr.To(true), // Has to be skipped as we create multiple controllers with the same name.
			UsePriorityQueue:   ptr.To(feature.Gates.Enabled(feature.PriorityQueue)),
		},
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
				Unstructured: true,
			},
		},
		WebhookServer: &noopWebhookServer{}, // Use noop webhook server to avoid opening unnecessary ports.
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Label: clusterSecretCacheSelector,
				},
			},
		},
	}

	mgr, err := ctrl.NewManager(env.Config, options)
	if err != nil {
		return nil, err
	}

	if err := (&CRDMigrator{
		Client:                 mgr.GetClient(),
		APIReader:              mgr.GetAPIReader(),
		SkipCRDMigrationPhases: skipCRDMigrationPhases,
		Config:                 crdMigratorConfig,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		return nil, err
	}

	return mgr, nil
}

func startManager(ctx context.Context, mgr manager.Manager) (context.CancelFunc, chan struct{}) {
	ctx, cancel := context.WithCancel(ctx)
	managerStopped := make(chan struct{})
	go func() {
		if err := mgr.Start(ctx); err != nil {
			panic("Failed to start test manager")
		}
		close(managerStopped)
	}()
	return cancel, managerStopped
}

func stopManager(cancelManager context.CancelFunc, managerStopped chan struct{}) {
	cancelManager()
	<-managerStopped
}

func installCRDs(ctx context.Context, c client.Client, crdPath string) error {
	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled

	installOpts := envtest.CRDInstallOptions{
		Scheme:       env.GetScheme(),
		MaxTime:      10 * time.Second,
		PollInterval: 100 * time.Millisecond,
		Paths: []string{
			filepath.Join(path.Dir(filename), "..", "..", "controllers", "crdmigrator", crdPath),
		},
		ErrorIfPathMissing: true,
	}

	// Read the CRD YAMLs into options.CRDs.
	if err := readCRDFiles(&installOpts); err != nil {
		return fmt.Errorf("unable to read CRD files: %w", err)
	}

	// Apply the CRDs.
	if err := applyCRDs(ctx, c, installOpts.CRDs); err != nil {
		return fmt.Errorf("unable to create CRD instances: %w", err)
	}

	// Wait for the CRDs to appear in discovery.
	if err := envtest.WaitForCRDs(env.GetConfig(), installOpts.CRDs, installOpts); err != nil {
		return fmt.Errorf("something went wrong waiting for CRDs to appear as API resources: %w", err)
	}

	return nil
}

func applyCRDs(ctx context.Context, c client.Client, crds []*apiextensionsv1.CustomResourceDefinition) error {
	for _, crd := range crds {
		existingCrd := crd.DeepCopy()
		err := c.Get(ctx, client.ObjectKey{Name: crd.GetName()}, existingCrd)
		switch {
		case apierrors.IsNotFound(err):
			if err := c.Create(ctx, crd); err != nil {
				return fmt.Errorf("unable to create CRD %s: %w", crd.GetName(), err)
			}
		case err != nil:
			return fmt.Errorf("unable to get CRD %s to check if it exists: %w", crd.GetName(), err)
		default:
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := c.Get(ctx, client.ObjectKey{Name: crd.GetName()}, existingCrd); err != nil {
					return err
				}
				// Note: Intentionally only overwriting spec and thus preserving metadata labels, annotations, etc.
				existingCrd.Spec = crd.Spec
				return c.Update(ctx, existingCrd)
			}); err != nil {
				return fmt.Errorf("unable to update CRD %s: %w", crd.GetName(), err)
			}
		}
	}
	return nil
}

type noopWebhookServer struct {
	webhook.Server
}

func (s *noopWebhookServer) Start(_ context.Context) error {
	return nil // Do nothing.
}

// TODO(sbueringer): The following will be dropped once we adopt: https://github.com/kubernetes-sigs/controller-runtime/pull/3129

// readCRDFiles reads the directories of CRDs in options.Paths and adds the CRD structs to options.CRDs.
func readCRDFiles(options *envtest.CRDInstallOptions) error {
	if len(options.Paths) > 0 {
		crdList, err := renderCRDs(options)
		if err != nil {
			return err
		}

		options.CRDs = append(options.CRDs, crdList...)
	}
	return nil
}

// renderCRDs iterate through options.Paths and extract all CRD files.
func renderCRDs(options *envtest.CRDInstallOptions) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	type GVKN struct {
		GVK  schema.GroupVersionKind
		Name string
	}

	crds := map[GVKN]*apiextensionsv1.CustomResourceDefinition{}

	for _, path := range options.Paths {
		var (
			err      error
			info     os.FileInfo
			files    []string
			filePath = path
		)

		// Return the error if ErrorIfPathMissing exists
		if info, err = os.Stat(path); os.IsNotExist(err) {
			if options.ErrorIfPathMissing {
				return nil, err
			}
			continue
		}

		if !info.IsDir() {
			filePath, files = filepath.Dir(path), []string{info.Name()}
		} else {
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				files = append(files, e.Name())
			}
		}

		crdList, err := readCRDs(filePath, files)
		if err != nil {
			return nil, err
		}

		for i, crd := range crdList {
			gvkn := GVKN{GVK: crd.GroupVersionKind(), Name: crd.GetName()}
			// We always use the CRD definition that we found last.
			crds[gvkn] = crdList[i]
		}
	}

	// Converting map to a list to return
	res := []*apiextensionsv1.CustomResourceDefinition{}
	for _, obj := range crds {
		res = append(res, obj)
	}
	return res, nil
}

// readCRDs reads the CRDs from files and Unmarshals them into structs.
func readCRDs(basePath string, files []string) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	var crds []*apiextensionsv1.CustomResourceDefinition

	// White list the file extensions that may contain CRDs
	crdExts := sets.NewString(".json", ".yaml", ".yml")

	for _, file := range files {
		// Only parse allowlisted file types
		if !crdExts.Has(filepath.Ext(file)) {
			continue
		}

		// Unmarshal CRDs from file into structs
		docs, err := readDocuments(filepath.Join(basePath, file))
		if err != nil {
			return nil, err
		}

		for _, doc := range docs {
			crd := &apiextensionsv1.CustomResourceDefinition{}
			if err = yaml.Unmarshal(doc, crd); err != nil {
				return nil, err
			}

			if crd.Kind != "CustomResourceDefinition" || crd.Spec.Names.Kind == "" || crd.Spec.Group == "" {
				continue
			}
			crds = append(crds, crd)
		}
	}
	return crds, nil
}

// readDocuments reads documents from file.
func readDocuments(fp string) ([][]byte, error) {
	b, err := os.ReadFile(fp) //nolint:gosec // No security issue here
	if err != nil {
		return nil, err
	}

	docs := [][]byte{}
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(b)))
	for {
		// Read document
		doc, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

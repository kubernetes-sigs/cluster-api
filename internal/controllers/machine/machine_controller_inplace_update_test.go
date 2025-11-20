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

package machine

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/cache"
)

func TestReconcileInPlaceUpdate(t *testing.T) {
	tests := []struct {
		name            string
		featureEnabled  bool
		setup           func(*testing.T) (*Reconciler, *scope)
		wantResult      ctrl.Result
		wantErr         bool
		wantErrContains string
		wantReason      string
		wantMessage     string
		verify          func(*testing.T, *WithT, context.Context, *Reconciler, *scope)
	}{
		{
			name:           "feature gate disabled returns immediately",
			featureEnabled: false,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				return &Reconciler{}, &scope{machine: newTestMachine()}
			},
			wantResult: ctrl.Result{},
		},
		{
			name:           "cleans up orphaned hook and annotations",
			featureEnabled: true,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()

				scheme := runtime.NewScheme()
				if err := clusterv1.AddToScheme(scheme); err != nil {
					t.Fatalf("failed to add clusterv1 to scheme: %v", err)
				}

				machine := newTestMachine()
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)

				infra := newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")
				infra.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})

				bootstrap := newTestUnstructured("GenericBootstrapConfig", "bootstrap.cluster.x-k8s.io/v1beta2", "bootstrap")
				bootstrap.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine, infra, bootstrap).Build()

				return &Reconciler{Client: client, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{
					machine:         machine,
					infraMachine:    infra,
					bootstrapConfig: bootstrap,
				}
			},
			wantResult: ctrl.Result{},
			verify: func(t *testing.T, g *WithT, ctx context.Context, r *Reconciler, s *scope) {
				t.Helper()

				updatedMachine := &clusterv1.Machine{}
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Annotations).ToNot(HaveKey(runtimev1.PendingHooksAnnotation))

				updatedInfra := &unstructured.Unstructured{}
				updatedInfra.SetGroupVersionKind(s.infraMachine.GroupVersionKind())
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.infraMachine), updatedInfra)).To(Succeed())
				g.Expect(updatedInfra.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))

				if s.bootstrapConfig != nil {
					updatedBootstrap := &unstructured.Unstructured{}
					updatedBootstrap.SetGroupVersionKind(s.bootstrapConfig.GroupVersionKind())
					g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.bootstrapConfig), updatedBootstrap)).To(Succeed())
					g.Expect(updatedBootstrap.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
				}
			},
		},
		{
			name:           "waits for pending hook to be marked",
			featureEnabled: true,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				return &Reconciler{}, &scope{machine: machine}
			},
			wantResult: ctrl.Result{},
		},
		{
			name:           "fails when infra machine is missing",
			featureEnabled: true,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
				machine.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				machine.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: "foo"}
				return &Reconciler{}, &scope{machine: machine}
			},
			wantResult:      ctrl.Result{},
			wantErr:         true,
			wantErrContains: "InfraMachine not found",
			wantReason:      clusterv1.MachineInPlaceUpdateFailedReason,
			wantMessage:     "In-place update not possible: InfraMachine not found",
		},
		{
			name:           "requeues while UpdateMachine hook is in progress",
			featureEnabled: true,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()

				catalog := runtimecatalog.New()
				if err := runtimehooksv1.AddToCatalog(catalog); err != nil {
					t.Fatalf("failed to add hooks to catalog: %v", err)
				}
				updateGVH, err := catalog.GroupVersionHook(runtimehooksv1.UpdateMachine)
				if err != nil {
					t.Fatalf("failed to look up UpdateMachine hook: %v", err)
				}

				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"test-extension"},
					}).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						updateGVH: &runtimehooksv1.UpdateMachineResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{
									Status:  runtimehooksv1.ResponseStatusSuccess,
									Message: "processing",
								},
								RetryAfterSeconds: 30,
							},
						},
					}).
					Build()

				scheme := runtime.NewScheme()
				if err := clusterv1.AddToScheme(scheme); err != nil {
					t.Fatalf("failed to add clusterv1 to scheme: %v", err)
				}

				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
				machine.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				machine.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: "foo"}

				infra := newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")
				infra.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine, infra).Build()

				return &Reconciler{
						Client:        client,
						RuntimeClient: runtimeClient,
						hookCache:     cache.New[cache.HookEntry](cache.HookCacheDefaultTTL),
					}, &scope{
						machine:      machine,
						infraMachine: infra,
					}
			},
			wantResult:  ctrl.Result{RequeueAfter: 30 * time.Second},
			wantReason:  clusterv1.MachineInPlaceUpdatingReason,
			wantMessage: "In-place update in progress: processing",
		},
		{
			name:           "completes successfully and cleans annotations",
			featureEnabled: true,
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()

				catalog := runtimecatalog.New()
				if err := runtimehooksv1.AddToCatalog(catalog); err != nil {
					t.Fatalf("failed to add hooks to catalog: %v", err)
				}
				updateGVH, err := catalog.GroupVersionHook(runtimehooksv1.UpdateMachine)
				if err != nil {
					t.Fatalf("failed to look up UpdateMachine hook: %v", err)
				}

				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"test-extension"},
					}).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						updateGVH: &runtimehooksv1.UpdateMachineResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{
									Status:  runtimehooksv1.ResponseStatusSuccess,
									Message: "done",
								},
								RetryAfterSeconds: 0,
							},
						},
					}).
					Build()

				scheme := runtime.NewScheme()
				if err := clusterv1.AddToScheme(scheme); err != nil {
					t.Fatalf("failed to add clusterv1 to scheme: %v", err)
				}

				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
				machine.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				machine.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: "foo"}

				infra := newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")
				infra.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})

				bootstrap := newTestUnstructured("GenericBootstrapConfig", "bootstrap.cluster.x-k8s.io/v1beta2", "bootstrap")
				bootstrap.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine, infra, bootstrap).Build()

				return &Reconciler{
						Client:        client,
						RuntimeClient: runtimeClient,
						hookCache:     cache.New[cache.HookEntry](cache.HookCacheDefaultTTL),
					}, &scope{
						machine:         machine,
						infraMachine:    infra,
						bootstrapConfig: bootstrap,
					}
			},
			wantResult: ctrl.Result{},
			verify: func(t *testing.T, g *WithT, ctx context.Context, r *Reconciler, s *scope) {
				t.Helper()

				updatedMachine := &clusterv1.Machine{}
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
				g.Expect(updatedMachine.Annotations).ToNot(HaveKey(runtimev1.PendingHooksAnnotation))

				updatedInfra := &unstructured.Unstructured{}
				updatedInfra.SetGroupVersionKind(s.infraMachine.GroupVersionKind())
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.infraMachine), updatedInfra)).To(Succeed())
				g.Expect(updatedInfra.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))

				if s.bootstrapConfig != nil {
					updatedBootstrap := &unstructured.Unstructured{}
					updatedBootstrap.SetGroupVersionKind(s.bootstrapConfig.GroupVersionKind())
					g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.bootstrapConfig), updatedBootstrap)).To(Succeed())
					g.Expect(updatedBootstrap.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, tt.featureEnabled)

			r, scope := tt.setup(t)
			ctx := context.Background()

			result, err := r.reconcileInPlaceUpdate(ctx, scope)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrContains != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.wantErrContains))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(result).To(Equal(tt.wantResult))

			if tt.wantReason != "" {
				g.Expect(scope.updatingReason).To(Equal(tt.wantReason))
			} else {
				g.Expect(scope.updatingReason).To(BeEmpty())
			}

			if tt.wantMessage != "" {
				g.Expect(scope.updatingMessage).To(Equal(tt.wantMessage))
			} else {
				g.Expect(scope.updatingMessage).To(BeEmpty())
			}

			if tt.verify != nil {
				tt.verify(t, g, ctx, r, scope)
			}
		})
	}
}

func TestCallUpdateMachineHook(t *testing.T) {
	catalog := runtimecatalog.New()
	if err := runtimehooksv1.AddToCatalog(catalog); err != nil {
		t.Fatalf("failed to add hooks to catalog: %v", err)
	}
	updateGVH, err := catalog.GroupVersionHook(runtimehooksv1.UpdateMachine)
	if err != nil {
		t.Fatalf("failed to determine UpdateMachine hook: %v", err)
	}

	tests := []struct {
		name               string
		setup              func(*testing.T) (*Reconciler, *scope)
		wantResult         ctrl.Result
		wantMessage        string
		wantErr            bool
		wantErrSubstrings  []string
		wantHookCacheEntry *cache.HookEntry
	}{
		{
			name: "fails if no extensions registered",
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{}).
					Build()
				return &Reconciler{RuntimeClient: runtimeClient, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{machine: newTestMachine(), infraMachine: newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")}
			},
			wantErr:           true,
			wantErrSubstrings: []string{"no extensions registered for UpdateMachine hook"},
		},
		{
			name: "fails if multiple extensions registered",
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"ext-a", "ext-b"},
					}).
					Build()
				return &Reconciler{RuntimeClient: runtimeClient, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{machine: newTestMachine(), infraMachine: newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")}
			},
			wantErr:           true,
			wantErrSubstrings: []string{"found multiple UpdateMachine hooks (ext-a,ext-b): only one hook is supported"},
		},
		{
			name: "fails when hook invocation returns error",
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"ext"},
					}).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						updateGVH: &runtimehooksv1.UpdateMachineResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusFailure},
							},
						},
					}).
					Build()
				return &Reconciler{RuntimeClient: runtimeClient, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{machine: newTestMachine(), infraMachine: newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")}
			},
			wantErr:           true,
			wantErrSubstrings: []string{"runtime hook", "UpdateMachine", "failed"},
		},
		{
			name: "returns requeue when hook succeeds with retry",
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"ext"},
					}).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						updateGVH: &runtimehooksv1.UpdateMachineResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{
									Status:  runtimehooksv1.ResponseStatusSuccess,
									Message: "processing",
								},
								RetryAfterSeconds: 45,
							},
						},
					}).
					Build()
				return &Reconciler{RuntimeClient: runtimeClient, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{machine: newTestMachine(), infraMachine: newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")}
			},
			wantResult:  ctrl.Result{RequeueAfter: 45 * time.Second},
			wantMessage: "processing",
			wantHookCacheEntry: ptr.To(cache.NewHookEntry(newTestMachine(), runtimehooksv1.UpdateMachine,
				time.Now().Add(45*time.Second), "processing")),
		},
		{
			name: "returns message when hook succeeds",
			setup: func(t *testing.T) (*Reconciler, *scope) {
				t.Helper()
				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"ext"},
					}).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						updateGVH: &runtimehooksv1.UpdateMachineResponse{
							CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
								CommonResponse: runtimehooksv1.CommonResponse{
									Status:  runtimehooksv1.ResponseStatusSuccess,
									Message: "done",
								},
							},
						},
					}).
					Build()
				return &Reconciler{RuntimeClient: runtimeClient, hookCache: cache.New[cache.HookEntry](cache.HookCacheDefaultTTL)}, &scope{machine: newTestMachine(), infraMachine: newTestUnstructured("GenericInfrastructureMachine", "infrastructure.cluster.x-k8s.io/v1beta2", "infra")}
			},
			wantResult:  ctrl.Result{},
			wantMessage: "done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r, scope := tt.setup(t)
			result, message, err := r.callUpdateMachineHook(context.Background(), scope)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				for _, substr := range tt.wantErrSubstrings {
					g.Expect(err.Error()).To(ContainSubstring(substr))
				}
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(tt.wantResult))
			g.Expect(message).To(Equal(tt.wantMessage))

			if tt.wantHookCacheEntry != nil {
				// Verify the cache entry.
				cacheEntry, ok := r.hookCache.Has(tt.wantHookCacheEntry.Key())
				g.Expect(ok).To(BeTrue())
				g.Expect(cacheEntry.ObjectKey).To(Equal(tt.wantHookCacheEntry.ObjectKey))
				g.Expect(cacheEntry.HookName).To(Equal(tt.wantHookCacheEntry.HookName))
				g.Expect(cacheEntry.ReconcileAfter).To(BeTemporally("~", tt.wantHookCacheEntry.ReconcileAfter, 5*time.Second))
				g.Expect(cacheEntry.ResponseMessage).To(Equal(tt.wantHookCacheEntry.ResponseMessage))

				// Call callUpdateMachineHook again and verify the cache hit.
				secondResult, message, err := r.callUpdateMachineHook(context.Background(), scope)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(message).To(Equal(tt.wantHookCacheEntry.ResponseMessage))
				// RequeueAfter should be now < then the previous RequeueAfter.
				g.Expect(secondResult.RequeueAfter).To(BeNumerically("<", result.RequeueAfter))
			} else {
				g.Expect(r.hookCache.Len()).To(Equal(0))
			}
		})
	}
}

func TestRemoveInPlaceUpdateAnnotation(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T) (*Reconciler, ctrlclient.Client, *clusterv1.Machine)
		verify func(*WithT, context.Context, ctrlclient.Client, *clusterv1.Machine)
	}{
		{
			name: "removes annotation when present",
			setup: func(t *testing.T) (*Reconciler, ctrlclient.Client, *clusterv1.Machine) {
				t.Helper()
				scheme := runtime.NewScheme()
				if err := clusterv1.AddToScheme(scheme); err != nil {
					t.Fatalf("failed to add clusterv1 to scheme: %v", err)
				}

				machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine", Namespace: "default", Annotations: map[string]string{clusterv1.UpdateInProgressAnnotation: ""}}}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine).Build()
				return &Reconciler{Client: client}, client, machine
			},
			verify: func(g *WithT, ctx context.Context, c ctrlclient.Client, machine *clusterv1.Machine) {
				updated := &clusterv1.Machine{}
				g.Expect(c.Get(ctx, ctrlclient.ObjectKeyFromObject(machine), updated)).To(Succeed())
				g.Expect(updated.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
			},
		},
		{
			name: "no-op when annotation missing",
			setup: func(t *testing.T) (*Reconciler, ctrlclient.Client, *clusterv1.Machine) {
				t.Helper()
				scheme := runtime.NewScheme()
				if err := clusterv1.AddToScheme(scheme); err != nil {
					t.Fatalf("failed to add clusterv1 to scheme: %v", err)
				}

				machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine", Namespace: "default"}}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine).Build()
				return &Reconciler{Client: client}, client, machine
			},
			verify: func(g *WithT, ctx context.Context, c ctrlclient.Client, machine *clusterv1.Machine) {
				updated := &clusterv1.Machine{}
				g.Expect(c.Get(ctx, ctrlclient.ObjectKeyFromObject(machine), updated)).To(Succeed())
				g.Expect(updated.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r, client, machine := tt.setup(t)
			ctx := context.Background()
			g.Expect(r.removeInPlaceUpdateAnnotation(ctx, machine)).To(Succeed())

			if tt.verify != nil {
				tt.verify(g, ctx, client, machine)
			}
		})
	}
}

func TestCompleteInPlaceUpdate_MissingInfra(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add clusterv1 to scheme: %v", err)
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "machine",
			Namespace:   "default",
			Annotations: map[string]string{},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine).Build()

	r := &Reconciler{Client: client}
	scope := &scope{machine: machine, infraMachine: nil}

	err := r.completeInPlaceUpdate(context.Background(), scope)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestCleanupMachine(t *testing.T) {
	g := NewWithT(t)

	original := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "machine",
			Namespace:   "default",
			Labels:      map[string]string{"key": "value"},
			Annotations: map[string]string{"anno": "value"},
		},
	}
	original.Status.Phase = "Running"

	cleaned := cleanupMachine(original)

	g.Expect(cleaned.APIVersion).To(Equal(clusterv1.GroupVersion.String()))
	g.Expect(cleaned.Kind).To(Equal("Machine"))
	g.Expect(cleaned.Name).To(Equal("machine"))
	g.Expect(cleaned.Namespace).To(Equal("default"))
	g.Expect(cleaned.Labels).To(HaveKeyWithValue("key", "value"))
	g.Expect(cleaned.Annotations).To(HaveKeyWithValue("anno", "value"))
	g.Expect(cleaned.Status).To(BeZero())
}

func TestCleanupUnstructured(t *testing.T) {
	g := NewWithT(t)

	original := &unstructured.Unstructured{Object: map[string]interface{}{}}
	original.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1beta2")
	original.SetKind("GenericInfrastructureMachine")
	original.SetName("infra")
	original.SetNamespace("default")
	original.SetLabels(map[string]string{"key": "value"})
	original.SetAnnotations(map[string]string{"anno": "value"})
	original.Object["spec"] = map[string]interface{}{"field": "value"}
	original.Object["status"] = map[string]interface{}{"state": "ready"}

	cleaned := cleanupUnstructured(original)

	g.Expect(cleaned.GetAPIVersion()).To(Equal(original.GetAPIVersion()))
	g.Expect(cleaned.GetKind()).To(Equal(original.GetKind()))
	g.Expect(cleaned.GetName()).To(Equal(original.GetName()))
	g.Expect(cleaned.GetNamespace()).To(Equal(original.GetNamespace()))
	g.Expect(cleaned.GetLabels()).To(HaveKeyWithValue("key", "value"))
	g.Expect(cleaned.GetAnnotations()).To(HaveKeyWithValue("anno", "value"))

	spec, found, err := unstructured.NestedMap(cleaned.Object, "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(spec).To(HaveKeyWithValue("field", "value"))

	_, found, err = unstructured.NestedFieldCopy(cleaned.Object, "status")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeFalse())
}

func newTestMachine() *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "machine",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: clusterv1.MachineSpec{},
	}
}

func newTestUnstructured(kind, apiVersion, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	u.SetNamespace("default")
	u.SetName(name)
	u.SetLabels(map[string]string{})
	u.SetAnnotations(map[string]string{})
	u.Object["spec"] = map[string]interface{}{"field": "value"}
	return u
}

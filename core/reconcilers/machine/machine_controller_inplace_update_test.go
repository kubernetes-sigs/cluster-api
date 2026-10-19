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
	"maps"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/api/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/core/setup"
	"sigs.k8s.io/cluster-api/feature"
	contractapi "sigs.k8s.io/cluster-api/internal/contract/api"
	contractv1 "sigs.k8s.io/cluster-api/internal/contract/api/v1beta2"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/pkg/dynamiccache"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestReconcileInPlaceUpdate(t *testing.T) {
	now := time.Now()
	machineKey := ctrlclient.ObjectKeyFromObject(newTestMachine())

	infraMachineGVK := builder.InfrastructureGroupVersion.WithKind(builder.GenericInfrastructureMachineKind)
	bootstrapConfigGVK := builder.BootstrapGroupVersion.WithKind(builder.GenericBootstrapConfigKind)

	tests := []struct {
		name                  string
		featureEnabled        bool
		machine               *clusterv1.Machine
		infraMachine          contractapi.InfraMachine
		bootstrapConfig       contractapi.BootstrapConfig
		updateMachineResponse *runtimehooksv1.UpdateMachineResponse
		wantResult            ctrl.Result
		wantErr               bool
		wantErrContains       string
		wantReason            string
		wantMessage           string
		wantCacheEntry        *UpdateMachineCacheEntry
		verify                func(context.Context, *testing.T, *WithT, *Reconciler, *scope)
	}{
		{
			name:           "feature gate disabled returns immediately",
			featureEnabled: false,
			machine:        newTestMachine(),
			wantResult:     ctrl.Result{},
		},
		{
			name:           "cleans up orphaned hook and annotations",
			featureEnabled: true,
			machine: func() *clusterv1.Machine {
				machine := newTestMachine()
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
				return machine
			}(),
			infraMachine:    newInPlaceUpdatingTestInfraMachine(),
			bootstrapConfig: newInPlaceUpdatingTestBootstrapConfig(),
			wantResult:      ctrl.Result{},
			verify:          verifyInPlaceUpdateCompleted,
		},
		{
			name:           "waits for pending hook to be marked",
			featureEnabled: true,
			machine: func() *clusterv1.Machine {
				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				return machine
			}(),
			wantResult: ctrl.Result{},
		},
		{
			name:           "fails when infra machine is missing",
			featureEnabled: true,
			machine: func() *clusterv1.Machine {
				machine := newTestMachine()
				machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
				machine.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				machine.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: "foo"}
				return machine
			}(),
			wantResult:      ctrl.Result{},
			wantErr:         true,
			wantErrContains: "InfraMachine not found",
			wantReason:      clusterv1.MachineInPlaceUpdateInternalErrorReason,
			wantMessage:     "In-place update not possible: InfraMachine not found",
		},
		{
			name:                  "surfaces the message and returns an error when UpdateMachine hook returns status Failure",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusFailure, "", "internal problem", 0),
			wantResult:            ctrl.Result{},
			wantErr:               true,
			wantErrContains:       "internal problem",
			wantReason:            clusterv1.MachineInPlaceUpdateInternalErrorReason,
			wantMessage:           `In-place update failed: failed to call extension handler "test-extension": got failure response with message: internal problem`,
			verify:                verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "returns an error when UpdateMachine hook returns an unknown updateStatus",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "Foo", "", 0),
			wantResult:            ctrl.Result{},
			wantErr:               true,
			wantErrContains:       `unknown updateStatus "Foo"`,
			wantReason:            clusterv1.MachineInPlaceUpdateInternalErrorReason,
			wantMessage:           `In-place update failed: invalid response from extension handler "test-extension": unknown updateStatus "Foo"`,
			verify:                verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues while UpdateMachine hook is in progress",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "", "processing", 30),
			wantResult:            ctrl.Result{RequeueAfter: 30 * time.Second},
			wantReason:            clusterv1.MachineInPlaceUpdatingReason,
			wantMessage:           "In-place update in progress (affects availability): processing",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(30 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
					messages:     []string{"processing"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues while UpdateMachine hook is in progress (does not affect availability)",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(map[string]string{clusterv1.UpdateInProgressAnnotation: "{\"affectsAvailability\":false}"}),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "", "processing", 30),
			wantResult:            ctrl.Result{RequeueAfter: 30 * time.Second},
			wantReason:            clusterv1.MachineInPlaceUpdatingReason,
			wantMessage:           "In-place update in progress (does not affect availability): processing",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(30 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
					messages:     []string{"processing"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "completes successfully and cleans annotations",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachineWithBootstrapConfig(),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			bootstrapConfig:       newInPlaceUpdatingTestBootstrapConfig(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "", "done", 0),
			wantResult:            ctrl.Result{},
			verify:                verifyInPlaceUpdateCompleted,
		},
		// Test cases with updateStatus set.
		{
			name:                  "requeues with default retry while UpdateMachine hook is in progress without retryAfterSeconds",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusInProgress, "processing", 0),
			wantResult:            ctrl.Result{RequeueAfter: updateMachineCacheDuration},
			wantReason:            clusterv1.MachineInPlaceUpdatingReason,
			wantMessage:           "In-place update in progress (affects availability): processing",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
					messages:     []string{"processing"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues and surfaces InPlaceUpdateFailed when UpdateMachine hook returns updateStatus Failed",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusFailed, "something failed", 15),
			wantResult:            ctrl.Result{RequeueAfter: 15 * time.Second},
			wantReason:            clusterv1.MachineInPlaceUpdateFailedReason,
			wantMessage:           "In-place update failed: something failed",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(15 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusFailed,
					messages:     []string{"something failed"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues with default retry when UpdateMachine hook returns updateStatus Failed without retryAfterSeconds",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusFailed, "something failed", 0),
			wantResult:            ctrl.Result{RequeueAfter: updateMachineCacheDuration},
			wantReason:            clusterv1.MachineInPlaceUpdateFailedReason,
			wantMessage:           "In-place update failed: something failed",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusFailed,
					messages:     []string{"something failed"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues and surfaces InPlaceUpdateBlocked when UpdateMachine hook returns updateStatus Blocked",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusBlocked, "waiting for something", 20),
			wantResult:            ctrl.Result{RequeueAfter: 20 * time.Second},
			wantReason:            clusterv1.MachineInPlaceUpdateBlockedReason,
			wantMessage:           "In-place update blocked: waiting for something",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(20 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusBlocked,
					messages:     []string{"waiting for something"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "requeues with default retry when UpdateMachine hook returns updateStatus Blocked without retryAfterSeconds",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusBlocked, "waiting for something", 0),
			wantResult:            ctrl.Result{RequeueAfter: updateMachineCacheDuration},
			wantReason:            clusterv1.MachineInPlaceUpdateBlockedReason,
			wantMessage:           "In-place update blocked: waiting for something",
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusBlocked,
					messages:     []string{"waiting for something"},
				},
			},
			verify: verifyInPlaceUpdateNotCompleted,
		},
		{
			name:                  "completes and removes dirty annotation when UpdateMachine hook returns updateStatus Completed",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(map[string]string{clusterv1.UpdateDirtyAnnotation: ""}),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusCompleted, "done", 0),
			wantResult:            ctrl.Result{},
			verify: func(ctx context.Context, t *testing.T, g *WithT, r *Reconciler, s *scope) {
				t.Helper()

				verifyInPlaceUpdateCompleted(ctx, t, g, r, s)

				updatedMachine := &clusterv1.Machine{}
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Annotations).ToNot(HaveKey(clusterv1.UpdateDirtyAnnotation))
			},
		},
		{
			name:                  "completes and sets dirty annotation when UpdateMachine hook returns updateStatus CompletedButDirty",
			featureEnabled:        true,
			machine:               newInPlaceUpdatingTestMachine(nil),
			infraMachine:          newInPlaceUpdatingTestInfraMachine(),
			updateMachineResponse: newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusCompletedButDirty, "dirty", 0),
			wantResult:            ctrl.Result{},
			verify: func(ctx context.Context, t *testing.T, g *WithT, r *Reconciler, s *scope) {
				t.Helper()

				verifyInPlaceUpdateCompleted(ctx, t, g, r, s)

				updatedMachine := &clusterv1.Machine{}
				g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
				g.Expect(updatedMachine.Annotations).To(HaveKeyWithValue(clusterv1.UpdateDirtyAnnotation, ""))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, tt.featureEnabled)

			scheme := runtime.NewScheme()
			g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			scheme.AddKnownTypeWithName(infraMachineGVK, &contractv1.InfraMachine{})
			scheme.AddKnownTypeWithName(bootstrapConfigGVK, &contractv1.BootstrapConfig{})
			var objects []ctrlclient.Object
			if tt.machine != nil {
				objects = append(objects, tt.machine)
			}
			if tt.infraMachine != nil {
				objects = append(objects, tt.infraMachine, builder.GenericInfrastructureMachineCRD.DeepCopy())
			}
			if tt.bootstrapConfig != nil {
				objects = append(objects, tt.bootstrapConfig, builder.GenericBootstrapConfigCRD.DeepCopy())
			}

			var runtimeClient *fakeruntimeclient.RuntimeClient
			if tt.updateMachineResponse != nil {
				catalog := runtimecatalog.New()
				g.Expect(runtimehooksv1.AddToCatalog(catalog)).To(Succeed())
				updateGVH, err := catalog.GroupVersionHook(runtimehooksv1.UpdateMachine)
				g.Expect(err).ToNot(HaveOccurred())

				runtimeClient = fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
						updateGVH: {"test-extension"},
					}).
					WithCallExtensionResponses(map[string]runtimehooksv1.ResponseObject{
						"test-extension": tt.updateMachineResponse,
					}).
					Build()
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			r := &Reconciler{
				Client:               fakeClient,
				APIReader:            fakeClient,
				DynamicCache:         dynamiccache.NewFakeDynamicCache(fakeClient, setup.DynamicCacheOptions()),
				RuntimeClient:        runtimeClient,
				updateMachineCache:   cache.New[UpdateMachineCacheEntry](ctx, 30*time.Minute),
				externalObjectsCache: cache.New[externalObjectsCacheEntry](ctx, 1*time.Hour),
				overrideNowFunc:      func() time.Time { return now },
			}
			s := &scope{
				machine:            tt.machine,
				infraMachine:       tt.infraMachine,
				infraMachineGVK:    infraMachineGVK,
				bootstrapConfig:    tt.bootstrapConfig,
				bootstrapConfigGVK: bootstrapConfigGVK,
			}

			result, err := r.reconcileInPlaceUpdate(t.Context(), s)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.wantErrContains != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.wantErrContains))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(result).To(Equal(tt.wantResult))
			g.Expect(s.updatingReason).To(Equal(tt.wantReason))
			g.Expect(s.updatingMessage).To(Equal(tt.wantMessage))

			entry, ok := r.updateMachineCache.Has(UpdateMachineCacheEntry{Machine: machineKey}.Key())
			if tt.wantCacheEntry == nil {
				g.Expect(ok).To(BeFalse())
			} else {
				g.Expect(ok).To(BeTrue())
				g.Expect(entry).To(BeComparableTo(*tt.wantCacheEntry, cmp.AllowUnexported(UpdateMachineCacheEntry{}, updateMachineResult{})))
			}

			if tt.verify != nil {
				tt.verify(ctx, t, g, r, s)
			}
		})
	}
}

// newInPlaceUpdatingTestMachineWithBootstrapConfig returns the same Machine as newInPlaceUpdatingTestMachine,
// but with a reference to a BootstrapConfig.
func newInPlaceUpdatingTestMachineWithBootstrapConfig() *clusterv1.Machine {
	bootstrapConfigGVK := builder.BootstrapGroupVersion.WithKind(builder.GenericBootstrapConfigKind)

	machine := newInPlaceUpdatingTestMachine(nil)
	machine.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: bootstrapConfigGVK.Group,
		Kind:     bootstrapConfigGVK.Kind,
		Name:     "bootstrap",
	}
	return machine
}

// newInPlaceUpdatingTestMachine returns a Machine for which an in-place update is in progress (hook pending, annotation set)
// and all preconditions for calling the UpdateMachine hook are met.
func newInPlaceUpdatingTestMachine(additionalAnnotations map[string]string) *clusterv1.Machine {
	infraMachineGVK := builder.InfrastructureGroupVersion.WithKind(builder.GenericInfrastructureMachineKind)

	machine := newTestMachine()
	machine.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
	machine.Annotations[runtimev1.PendingHooksAnnotation] = runtimecatalog.HookName(runtimehooksv1.UpdateMachine)
	maps.Copy(machine.Annotations, additionalAnnotations)
	machine.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: infraMachineGVK.Group,
		Kind:     infraMachineGVK.Kind,
		Name:     "infra",
	}
	machine.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	machine.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
	machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: "foo"}
	return machine
}

// newInPlaceUpdatingTestInfraMachine returns the InfraMachine matching newInPlaceUpdatingTestMachine.
func newInPlaceUpdatingTestInfraMachine() contractapi.InfraMachine {
	infra := newTestInfraMachine("infra")
	infra.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})
	return infra
}

// newInPlaceUpdatingTestBootstrapConfig returns the BootstrapConfig matching newInPlaceUpdatingTestMachineWithBootstrapConfig.
func newInPlaceUpdatingTestBootstrapConfig() contractapi.BootstrapConfig {
	bootstrap := newTestBootstrapConfig("bootstrap")
	bootstrap.SetAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})
	return bootstrap
}

func newUpdateMachineResponse(status runtimehooksv1.ResponseStatus, updateStatus runtimehooksv1.MachineUpdateStatus, message string, retryAfterSeconds int32) *runtimehooksv1.UpdateMachineResponse {
	return &runtimehooksv1.UpdateMachineResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status:  status,
				Message: message,
			},
			RetryAfterSeconds: retryAfterSeconds,
		},
		UpdateStatus: updateStatus,
	}
}

// verifyInPlaceUpdateNotCompleted verifies that the in-place update annotation and the pending hook annotation
// were not touched.
func verifyInPlaceUpdateNotCompleted(ctx context.Context, t *testing.T, g *WithT, r *Reconciler, s *scope) {
	t.Helper()

	updatedMachine := &clusterv1.Machine{}
	g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
	g.Expect(updatedMachine.Annotations).To(HaveKey(clusterv1.UpdateInProgressAnnotation))
	g.Expect(updatedMachine.Annotations).To(HaveKey(runtimev1.PendingHooksAnnotation))

	updatedInfra := &unstructured.Unstructured{}
	updatedInfra.SetGroupVersionKind(s.infraMachineGVK)
	g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.infraMachine), updatedInfra)).To(Succeed())
	g.Expect(updatedInfra.GetAnnotations()).To(HaveKey(clusterv1.UpdateInProgressAnnotation))

	if s.bootstrapConfig != nil {
		updatedBootstrap := &unstructured.Unstructured{}
		updatedBootstrap.SetGroupVersionKind(s.bootstrapConfigGVK)
		g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.bootstrapConfig), updatedBootstrap)).To(Succeed())
		g.Expect(updatedBootstrap.GetAnnotations()).To(HaveKey(clusterv1.UpdateInProgressAnnotation))
	}
}

// verifyInPlaceUpdateCompleted verifies that the in-place update annotation and the pending hook were removed.
func verifyInPlaceUpdateCompleted(ctx context.Context, t *testing.T, g *WithT, r *Reconciler, s *scope) {
	t.Helper()

	updatedMachine := &clusterv1.Machine{}
	g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.machine), updatedMachine)).To(Succeed())
	g.Expect(updatedMachine.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
	g.Expect(updatedMachine.Annotations).ToNot(HaveKey(runtimev1.PendingHooksAnnotation))

	updatedInfra := &unstructured.Unstructured{}
	updatedInfra.SetGroupVersionKind(s.infraMachineGVK)
	g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.infraMachine), updatedInfra)).To(Succeed())
	g.Expect(updatedInfra.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))

	if s.bootstrapConfig != nil {
		updatedBootstrap := &unstructured.Unstructured{}
		updatedBootstrap.SetGroupVersionKind(s.bootstrapConfigGVK)
		g.Expect(r.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(s.bootstrapConfig), updatedBootstrap)).To(Succeed())
		g.Expect(updatedBootstrap.GetAnnotations()).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
	}
}

func TestUpdateMachine(t *testing.T) {
	now := time.Now()
	machineKey := ctrlclient.ObjectKeyFromObject(newTestMachine())

	catalog := runtimecatalog.New()
	if err := runtimehooksv1.AddToCatalog(catalog); err != nil {
		t.Fatalf("failed to add hooks to catalog: %v", err)
	}
	updateGVH, err := catalog.GroupVersionHook(runtimehooksv1.UpdateMachine)
	if err != nil {
		t.Fatalf("failed to determine UpdateMachine hook: %v", err)
	}

	tests := []struct {
		name                     string
		getAllExtensionResponses map[runtimecatalog.GroupVersionHook][]string
		callExtensionResponses   map[string]runtimehooksv1.ResponseObject
		wantResult               updateMachineResult
		wantErr                  string
		wantCacheEntry           *UpdateMachineCacheEntry
	}{
		{
			name:                     "fails if no extensions registered",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{},
			wantErr:                  "no extensions registered for UpdateMachine hook",
		},
		{
			name: "fails if multiple extensions registered",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext-a", "ext-b"},
			},
			wantErr: "found multiple UpdateMachine hooks (ext-a,ext-b): only one hook is supported",
		},
		{
			name: "fails when hook invocation returns error",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusFailure, "", "extension failed", 0),
			},
			wantErr: `failed to call extension handler "ext": got failure response with message: extension failed`,
		},
		{
			name: "fails when hook returns an unknown updateStatus",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "Foo", "", 0),
			},
			wantErr: `invalid response from extension handler "ext": unknown updateStatus "Foo"`,
		},
		{
			name: "returns requeue when hook succeeds with retry",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "", "processing", 45),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(45 * time.Second),
				updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
				messages:     []string{"processing"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(45 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
					messages:     []string{"processing"},
				},
			},
		},
		{
			name: "returns message when hook succeeds",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, "", "done", 0),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(updateMachineCacheDuration),
				updateStatus: runtimehooksv1.MachineUpdateStatusCompleted,
				messages:     []string{"done"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusCompleted,
					messages:     []string{"done"},
				},
			},
		},
		// Test cases with updateStatus set.
		{
			name: "returns InProgress when hook returns updateStatus InProgress",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusInProgress, "processing", 15),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(15 * time.Second),
				updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
				messages:     []string{"processing"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(15 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusInProgress,
					messages:     []string{"processing"},
				},
			},
		},
		{
			name: "returns Failed when hook returns updateStatus Failed",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusFailed, "failed", 15),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(15 * time.Second),
				updateStatus: runtimehooksv1.MachineUpdateStatusFailed,
				messages:     []string{"failed"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(15 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusFailed,
					messages:     []string{"failed"},
				},
			},
		},
		{
			name: "returns Blocked when hook returns updateStatus Blocked",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusBlocked, "blocked", 20),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(20 * time.Second),
				updateStatus: runtimehooksv1.MachineUpdateStatusBlocked,
				messages:     []string{"blocked"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(20 * time.Second),
					updateStatus: runtimehooksv1.MachineUpdateStatusBlocked,
					messages:     []string{"blocked"},
				},
			},
		},
		{
			name: "returns Completed when hook returns updateStatus Completed",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusCompleted, "done", 0),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(updateMachineCacheDuration),
				updateStatus: runtimehooksv1.MachineUpdateStatusCompleted,
				messages:     []string{"done"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusCompleted,
					messages:     []string{"done"},
				},
			},
		},
		{
			name: "returns CompletedButDirty when hook returns updateStatus CompletedButDirty",
			getAllExtensionResponses: map[runtimecatalog.GroupVersionHook][]string{
				updateGVH: {"ext"},
			},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"ext": newUpdateMachineResponse(runtimehooksv1.ResponseStatusSuccess, runtimehooksv1.MachineUpdateStatusCompletedButDirty, "dirty", 0),
			},
			wantResult: updateMachineResult{
				retryAfter:   now.Add(updateMachineCacheDuration),
				updateStatus: runtimehooksv1.MachineUpdateStatusCompletedButDirty,
				messages:     []string{"dirty"},
			},
			wantCacheEntry: &UpdateMachineCacheEntry{
				Machine: machineKey,
				updateMachineResult: updateMachineResult{
					retryAfter:   now.Add(updateMachineCacheDuration),
					updateStatus: runtimehooksv1.MachineUpdateStatusCompletedButDirty,
					messages:     []string{"dirty"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Note: currentTime can be moved forward by the test without affecting the expected values based on now.
			currentTime := now

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithGetAllExtensionResponses(tt.getAllExtensionResponses).
				WithCallExtensionResponses(tt.callExtensionResponses).
				Build()

			r := &Reconciler{
				RuntimeClient:        runtimeClient,
				updateMachineCache:   cache.New[UpdateMachineCacheEntry](ctx, 30*time.Minute),
				externalObjectsCache: cache.New[externalObjectsCacheEntry](ctx, 1*time.Hour),
				overrideNowFunc:      func() time.Time { return currentTime },
			}
			infraMachineGVK := builder.InfrastructureGroupVersion.WithKind(builder.GenericInfrastructureMachineKind)
			bootstrapConfigGVK := builder.BootstrapGroupVersion.WithKind(builder.GenericBootstrapConfigKind)

			scope := &scope{
				machine:            newTestMachine(),
				infraMachine:       newTestInfraMachine("infra"),
				infraMachineGVK:    infraMachineGVK,
				bootstrapConfig:    newTestBootstrapConfig("bootstrap"),
				bootstrapConfigGVK: bootstrapConfigGVK,
			}
			// Note: Warm up the externalObjectsCache to also test the code path with a warmed up cache.
			// Without a cache hit updateMachine below would fail because Reconciler.APIReader is not set.
			// TestReconcileInPlaceUpdate in contrast is testing the code path with an empty cache.
			warmUpExternalObjectsCache(r, scope)

			result, err := r.updateMachine(t.Context(), scope)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErr))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(BeComparableTo(tt.wantResult, cmp.AllowUnexported(updateMachineResult{})))

			entry, ok := r.updateMachineCache.Has(UpdateMachineCacheEntry{Machine: machineKey}.Key())
			if tt.wantCacheEntry == nil {
				g.Expect(ok).To(BeFalse())
				return
			}

			// Verify the cache entry.
			g.Expect(ok).To(BeTrue())
			g.Expect(entry).To(BeComparableTo(*tt.wantCacheEntry, cmp.AllowUnexported(UpdateMachineCacheEntry{}, updateMachineResult{})))

			// Call updateMachine again before retryAfter and verify the cache hit.
			currentTime = now.Add(10 * time.Second)
			secondResult, err := r.updateMachine(t.Context(), scope)
			g.Expect(err).ToNot(HaveOccurred())
			// The cached result is returned as is, i.e. retryAfter is not recomputed based on the new time.
			g.Expect(secondResult).To(BeComparableTo(tt.wantResult, cmp.AllowUnexported(updateMachineResult{})))

			// Call updateMachine again after retryAfter and verify the cache entry is not used anymore.
			// Note: A completed result removes the objects from the externalObjectsCache, so we have to warm it up again.
			warmUpExternalObjectsCache(r, scope)
			currentTime = tt.wantResult.retryAfter.Add(time.Second)
			thirdResult, err := r.updateMachine(t.Context(), scope)
			g.Expect(err).ToNot(HaveOccurred())
			// retryAfter is recomputed based on the new time.
			g.Expect(thirdResult.retryAfter).To(Equal(currentTime.Add(tt.wantResult.retryAfter.Sub(now))))
		})
	}
}

// warmUpExternalObjectsCache adds the InfraMachine and BootstrapConfig of the scope to the externalObjectsCache.
func warmUpExternalObjectsCache(r *Reconciler, s *scope) {
	infraMachineUnstructured := newTestUnstructured(s.infraMachineGVK, s.infraMachine.GetName())
	infraMachineUnstructured.SetResourceVersion(s.infraMachine.GetResourceVersion())
	r.externalObjectsCache.Add(externalObjectsCacheEntry{obj: infraMachineUnstructured})
	bootstrapConfigUnstructured := newTestUnstructured(s.bootstrapConfigGVK, s.bootstrapConfig.GetName())
	bootstrapConfigUnstructured.SetResourceVersion(s.bootstrapConfig.GetResourceVersion())
	r.externalObjectsCache.Add(externalObjectsCacheEntry{obj: bootstrapConfigUnstructured})
}

func TestRemoveInPlaceUpdateAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name: "removes annotation when present",
			annotations: map[string]string{
				clusterv1.UpdateInProgressAnnotation: "",
			},
		},
		{
			name:        "no-op when annotation missing",
			annotations: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "machine",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(machine).Build()

			r := &Reconciler{Client: client}

			g.Expect(r.removeInPlaceUpdateAnnotation(t.Context(), client, "Machine", machine)).To(Succeed())

			g.Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(machine), machine)).To(Succeed())
			g.Expect(machine.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
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

	r := &Reconciler{
		Client:             client,
		updateMachineCache: cache.New[UpdateMachineCacheEntry](ctx, 30*time.Minute),
	}
	scope := &scope{machine: machine, infraMachine: nil}

	err := r.completeInPlaceUpdate(context.Background(), scope, "")
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

func newTestInfraMachine(name string) contractapi.InfraMachine {
	return &contractv1.InfraMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            name,
			ResourceVersion: "2",
		},
		Spec: contractv1.InfraMachineSpec{
			ProviderID: "provider-id",
		},
	}
}

func newTestBootstrapConfig(name string) contractapi.BootstrapConfig {
	return &contractv1.BootstrapConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            name,
			ResourceVersion: "3",
		},
		Status: contractv1.BootstrapConfigStatus{
			DataSecretName: "data-secret-name",
		},
	}
}

func newTestUnstructured(gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetAPIVersion(gvk.GroupVersion().String())
	u.SetKind(gvk.Kind)
	u.SetNamespace("default")
	u.SetName(name)
	u.SetLabels(map[string]string{})
	u.SetAnnotations(map[string]string{})
	u.Object["spec"] = map[string]interface{}{"field": "value"}
	return u
}

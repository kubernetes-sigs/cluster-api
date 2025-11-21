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

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/defaulting"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/patch"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_canUpdateMachine(t *testing.T) {
	machineToInPlaceUpdate := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-to-in-place-update",
		},
	}
	nonEmptyMachineUpToDateResult := internal.UpToDateResult{
		// No real content needed for this, fields should just not be nil,
		EligibleForInPlaceUpdate: true,
		DesiredMachine:           &clusterv1.Machine{},
		CurrentInfraMachine:      &unstructured.Unstructured{},
		DesiredInfraMachine:      &unstructured.Unstructured{},
		CurrentKubeadmConfig:     &bootstrapv1.KubeadmConfig{},
		DesiredKubeadmConfig:     &bootstrapv1.KubeadmConfig{},
	}
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)
	canUpdateMachineGVH, err := catalog.GroupVersionHook(runtimehooksv1.CanUpdateMachine)
	if err != nil {
		panic("unable to compute GVH")
	}

	tests := []struct {
		name                                 string
		machineUpToDateResult                internal.UpToDateResult
		enableInPlaceUpdatesFeatureGate      bool
		canExtensionsUpdateMachineFunc       func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult, extensionHandlers []string) (bool, []string, error)
		getAllExtensionsResponses            map[runtimecatalog.GroupVersionHook][]string
		wantCanExtensionsUpdateMachineCalled bool
		wantCanUpdateMachine                 bool
		wantError                            bool
		wantErrorMessage                     string
	}{
		{
			name:                            "Return false if feature gate is not enabled",
			enableInPlaceUpdatesFeatureGate: false,
			wantCanUpdateMachine:            false,
		},
		{
			name:                            "Return false if objects in machineUpToDateResult are nil",
			enableInPlaceUpdatesFeatureGate: true,
			wantCanUpdateMachine:            false,
		},
		{
			name:                            "Return false if no CanUpdateMachine extensions registered",
			enableInPlaceUpdatesFeatureGate: true,
			machineUpToDateResult:           nonEmptyMachineUpToDateResult,
			getAllExtensionsResponses:       map[runtimecatalog.GroupVersionHook][]string{},
			wantCanUpdateMachine:            false,
		},
		{
			name:                            "Return error if more than one CanUpdateMachine extensions registered",
			enableInPlaceUpdatesFeatureGate: true,
			machineUpToDateResult:           nonEmptyMachineUpToDateResult,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineGVH: {"test-update-extension-1", "test-update-extension-2"},
			},
			wantError:        true,
			wantErrorMessage: "found multiple CanUpdateMachine hooks (test-update-extension-1,test-update-extension-2): only one hook is supported",
		},
		{
			name:                            "Return false if canExtensionsUpdateMachine returns false",
			enableInPlaceUpdatesFeatureGate: true,
			machineUpToDateResult:           nonEmptyMachineUpToDateResult,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult, extensionHandlers []string) (bool, []string, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return false, nil, errors.Errorf("unexpected error")
				}
				return false, []string{"can not update"}, nil
			},
			wantCanExtensionsUpdateMachineCalled: true,
			wantCanUpdateMachine:                 false,
		},
		{
			name:                            "Return true if canExtensionsUpdateMachine returns true",
			enableInPlaceUpdatesFeatureGate: true,
			machineUpToDateResult:           nonEmptyMachineUpToDateResult,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult, extensionHandlers []string) (bool, []string, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return false, nil, errors.Errorf("unexpected error")
				}
				return true, nil, nil
			},
			wantCanExtensionsUpdateMachineCalled: true,
			wantCanUpdateMachine:                 true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.enableInPlaceUpdatesFeatureGate {
				utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)
			}

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithGetAllExtensionResponses(tt.getAllExtensionsResponses).
				Build()

			var canExtensionsUpdateMachineCalled bool
			r := &KubeadmControlPlaneReconciler{
				RuntimeClient: runtimeClient,
				overrideCanExtensionsUpdateMachine: func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult, extensionHandlers []string) (bool, []string, error) {
					canExtensionsUpdateMachineCalled = true
					return tt.canExtensionsUpdateMachineFunc(ctx, machine, machineUpToDateResult, extensionHandlers)
				},
			}

			canUpdateMachine, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, tt.machineUpToDateResult)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(canUpdateMachine).To(Equal(tt.wantCanUpdateMachine))

			g.Expect(canExtensionsUpdateMachineCalled).To(Equal(tt.wantCanExtensionsUpdateMachineCalled), "canExtensionsUpdateMachineCalled: actual: %t expected: %t", canExtensionsUpdateMachineCalled, tt.wantCanExtensionsUpdateMachineCalled)
		})
	}
}

func Test_canExtensionsUpdateMachine(t *testing.T) {
	currentMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Version: "v1.30.0",
		},
	}
	desiredMachine := currentMachine.DeepCopy()
	desiredMachine.Spec.Version = "v1.31.0"

	currentKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						ImageTag: "3.5.0-0",
					},
				},
			},
		},
	}
	desiredKubeadmConfig := currentKubeadmConfig.DeepCopy()
	desiredKubeadmConfig.Spec.ClusterConfiguration.Etcd.Local.ImageTag = "3.6.4-0"

	currentInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineKind,
			"metadata": map[string]interface{}{
				"name":      "machine-to-in-place-update",
				"namespace": metav1.NamespaceDefault,
				"annotations": map[string]interface{}{
					clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template-1",
					clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
				},
			},
			"spec": map[string]interface{}{
				"hello": "world",
			},
		},
	}
	desiredInfraMachine := currentInfraMachine.DeepCopy()
	_ = unstructured.SetNestedField(desiredInfraMachine.Object, "in-place updated world", "spec", "hello")

	responseWithEmptyPatches := &runtimehooksv1.CanUpdateMachineResponse{
		CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
		MachinePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONPatchType,
			Patch:     []byte("[]"),
		},
		InfrastructureMachinePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte{},
		},
		BootstrapConfigPatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte("{}"),
		},
	}
	patchToUpdateMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/version","value":"v1.31.0"}]`),
	}
	patchToUpdateKubeadmConfig := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/clusterConfiguration/etcd/local/imageTag","value":"3.6.4-0"}]`),
	}
	patchToUpdateInfraMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/hello","value":"in-place updated world"}]`),
	}
	emptyPatch := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte{},
	}

	tests := []struct {
		name                         string
		machineUpToDateResult        internal.UpToDateResult
		extensionHandlers            []string
		callExtensionResponses       map[string]runtimehooksv1.ResponseObject
		callExtensionExpectedChanges map[string]func(runtime.Object)
		wantCanUpdateMachine         bool
		wantReasons                  []string
		wantError                    bool
		wantErrorMessage             string
	}{
		{
			name: "Return true if current and desired objects are equal and no patches are returned",
			// Note: canExtensionsUpdateMachine should never be called if the objects are equal, but this is a simple first test case.
			machineUpToDateResult: internal.UpToDateResult{
				DesiredMachine:       currentMachine,
				CurrentInfraMachine:  currentInfraMachine,
				DesiredInfraMachine:  currentInfraMachine,
				CurrentKubeadmConfig: currentKubeadmConfig,
				DesiredKubeadmConfig: currentKubeadmConfig,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": responseWithEmptyPatches,
			},
			wantCanUpdateMachine: true,
		},
		{
			name: "Return false if current and desired objects are not equal and no patches are returned",
			machineUpToDateResult: internal.UpToDateResult{
				DesiredMachine:       desiredMachine,
				CurrentInfraMachine:  currentInfraMachine,
				DesiredInfraMachine:  desiredInfraMachine,
				CurrentKubeadmConfig: currentKubeadmConfig,
				DesiredKubeadmConfig: desiredKubeadmConfig,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": responseWithEmptyPatches,
			},
			wantCanUpdateMachine: false,
			wantReasons: []string{
				`Machine cannot be updated in-place: &v1beta2.Machine{
    TypeMeta:   {},
    ObjectMeta: {},
    Spec: v1beta2.MachineSpec{
      ClusterName:       "",
      Bootstrap:         {},
      InfrastructureRef: {},
-     Version:           "v1.30.0",
+     Version:           "v1.31.0",
      ProviderID:        "",
      FailureDomain:     "",
      ... // 4 identical fields
    },
    Status: {},
  }`,
				`KubeadmConfig cannot be updated in-place: &unstructured.Unstructured{
    Object: map[string]any{
      "spec": map[string]any{
-       "clusterConfiguration": map[string]any{"etcd": map[string]any{"local": map[string]any{"imageTag": string("3.5.0-0")}}},
+       "clusterConfiguration": map[string]any{"etcd": map[string]any{"local": map[string]any{"imageTag": string("3.6.4-0")}}},
        "format":               string("cloud-config"),
        "initConfiguration":    map[string]any{"nodeRegistration": map[string]any{"imagePullPolicy": string("IfNotPresent")}},
        "joinConfiguration":    map[string]any{"nodeRegistration": map[string]any{"imagePullPolicy": string("IfNotPresent")}},
      },
    },
  }`,
				`TestInfrastructureMachine cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{"spec": map[string]any{"hello": string("world")}},
+   Object: map[string]any{"spec": map[string]any{"hello": string("in-place updated world")}},
  }`,
			},
		},
		{
			name: "Return true if current and desired objects are not equal and patches are returned that account for all diffs",
			machineUpToDateResult: internal.UpToDateResult{
				DesiredMachine:       desiredMachine,
				CurrentInfraMachine:  currentInfraMachine,
				DesiredInfraMachine:  desiredInfraMachine,
				CurrentKubeadmConfig: currentKubeadmConfig,
				DesiredKubeadmConfig: desiredKubeadmConfig,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": &runtimehooksv1.CanUpdateMachineResponse{
					CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachinePatch:               patchToUpdateMachine,
					InfrastructureMachinePatch: patchToUpdateInfraMachine,
					BootstrapConfigPatch:       patchToUpdateKubeadmConfig,
				},
			},
			wantCanUpdateMachine: true,
		},
		{
			name: "Return true if current and desired objects are not equal and patches are returned that account for all diffs (multiple extensions)",
			machineUpToDateResult: internal.UpToDateResult{
				DesiredMachine:       desiredMachine,
				CurrentInfraMachine:  currentInfraMachine,
				DesiredInfraMachine:  desiredInfraMachine,
				CurrentKubeadmConfig: currentKubeadmConfig,
				DesiredKubeadmConfig: desiredKubeadmConfig,
			},
			extensionHandlers: []string{"test-update-extension-1", "test-update-extension-2", "test-update-extension-3"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension-1": &runtimehooksv1.CanUpdateMachineResponse{
					CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachinePatch:   patchToUpdateMachine,
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineResponse{
					CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					InfrastructureMachinePatch: patchToUpdateInfraMachine,
				},
				"test-update-extension-3": &runtimehooksv1.CanUpdateMachineResponse{
					CommonResponse:       runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					BootstrapConfigPatch: patchToUpdateKubeadmConfig,
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machine, ok := object.(*clusterv1.Machine); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachine is already applied.
						machine.Spec.Version = "v1.31.0"
					}
				},
				"test-update-extension-3": func(object runtime.Object) {
					if machine, ok := object.(*clusterv1.Machine); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachine is already applied.
						machine.Spec.Version = "v1.31.0"
					}
					if infraMachine, ok := object.(*unstructured.Unstructured); ok {
						// After the call to test-update-extension-2 we expect that patchToUpdateInfraMachine is already applied.
						_ = unstructured.SetNestedField(infraMachine.Object, "in-place updated world", "spec", "hello")
					}
				},
			},
			wantCanUpdateMachine: true,
		},
		{
			name: "Return false if current and desired objects are not equal and patches are returned that only account for some diffs",
			machineUpToDateResult: internal.UpToDateResult{
				DesiredMachine:       desiredMachine,
				CurrentInfraMachine:  currentInfraMachine,
				DesiredInfraMachine:  desiredInfraMachine,
				CurrentKubeadmConfig: currentKubeadmConfig,
				DesiredKubeadmConfig: desiredKubeadmConfig,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": &runtimehooksv1.CanUpdateMachineResponse{
					CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachinePatch:               patchToUpdateMachine,
					InfrastructureMachinePatch: emptyPatch,
					BootstrapConfigPatch:       emptyPatch,
				},
			},
			wantCanUpdateMachine: false,
			wantReasons: []string{
				`KubeadmConfig cannot be updated in-place: &unstructured.Unstructured{
    Object: map[string]any{
      "spec": map[string]any{
-       "clusterConfiguration": map[string]any{"etcd": map[string]any{"local": map[string]any{"imageTag": string("3.5.0-0")}}},
+       "clusterConfiguration": map[string]any{"etcd": map[string]any{"local": map[string]any{"imageTag": string("3.6.4-0")}}},
        "format":               string("cloud-config"),
        "initConfiguration":    map[string]any{"nodeRegistration": map[string]any{"imagePullPolicy": string("IfNotPresent")}},
        "joinConfiguration":    map[string]any{"nodeRegistration": map[string]any{"imagePullPolicy": string("IfNotPresent")}},
      },
    },
  }`,
				`TestInfrastructureMachine cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{"spec": map[string]any{"hello": string("world")}},
+   Object: map[string]any{"spec": map[string]any{"hello": string("in-place updated world")}},
  }`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().
				WithObjects(currentMachine, currentInfraMachine, currentKubeadmConfig).
				Build()

			catalog := runtimecatalog.New()
			_ = runtimehooksv1.AddToCatalog(catalog)
			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithCallExtensionValidations(validateCanUpdateMachineRequests(currentMachine, tt.machineUpToDateResult, tt.callExtensionExpectedChanges)).
				WithCallExtensionResponses(tt.callExtensionResponses).
				Build()

			r := &KubeadmControlPlaneReconciler{
				Client:        fakeClient,
				RuntimeClient: runtimeClient,
			}

			canUpdateMachine, reasons, err := r.canExtensionsUpdateMachine(ctx, currentMachine, tt.machineUpToDateResult, tt.extensionHandlers)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(canUpdateMachine).To(Equal(tt.wantCanUpdateMachine))
			g.Expect(reasons).To(BeComparableTo(tt.wantReasons))
		})
	}
}

func validateCanUpdateMachineRequests(currentMachine *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult, callExtensionExpectedChanges map[string]func(runtime.Object)) func(name string, object runtimehooksv1.RequestObject) error {
	return func(name string, req runtimehooksv1.RequestObject) error {
		switch req := req.(type) {
		case *runtimehooksv1.CanUpdateMachineRequest:
			// Compare Machine
			currentMachine := currentMachine.DeepCopy()
			currentMachine.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
			currentMachine.ResourceVersion = "" // cleanupMachine drops ResourceVersion.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentMachine)
			}
			if d := diff(req.Current.Machine, *currentMachine); d != "" {
				return fmt.Errorf("expected currentMachine to be equal, got diff: %s", d)
			}
			desiredMachine := machineUpToDateResult.DesiredMachine.DeepCopy()
			desiredMachine.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
			desiredMachine.ResourceVersion = "" // cleanupMachine drops ResourceVersion.
			if d := diff(req.Desired.Machine, *desiredMachine); d != "" {
				return fmt.Errorf("expected desiredMachine to be equal, got diff: %s", d)
			}

			// Compare KubeadmConfig
			currentKubeadmConfig := machineUpToDateResult.CurrentKubeadmConfig.DeepCopy()
			currentKubeadmConfig.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig"))
			currentKubeadmConfig.ResourceVersion = ""                                 // cleanupKubeadmConfig drops ResourceVersion.
			defaulting.ApplyPreviousKubeadmConfigDefaults(&currentKubeadmConfig.Spec) // PrepareKubeadmConfigsForDiff applies defaults.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentKubeadmConfig)
			}
			currentKubeadmConfigBytes, _ := json.Marshal(currentKubeadmConfig)
			if d := diff(req.Current.BootstrapConfig.Raw, currentKubeadmConfigBytes); d != "" {
				return fmt.Errorf("expected currentKubeadmConfig to be equal, got diff: %s", d)
			}
			desiredKubeadmConfig := machineUpToDateResult.DesiredKubeadmConfig.DeepCopy()
			desiredKubeadmConfig.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig"))
			desiredKubeadmConfig.ResourceVersion = ""                                 // cleanupKubeadmConfig drops ResourceVersion.
			defaulting.ApplyPreviousKubeadmConfigDefaults(&desiredKubeadmConfig.Spec) // PrepareKubeadmConfigsForDiff applies defaults.
			desiredKubeadmConfigBytes, _ := json.Marshal(desiredKubeadmConfig)
			if d := diff(req.Desired.BootstrapConfig.Raw, desiredKubeadmConfigBytes); d != "" {
				return fmt.Errorf("expected desiredKubeadmConfig to be equal, got diff: %s", d)
			}

			// Compare InfraMachine
			currentInfraMachine := machineUpToDateResult.CurrentInfraMachine.DeepCopy()
			currentInfraMachine.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentInfraMachine)
			}
			currentInfraMachineBytes, _ := json.Marshal(currentInfraMachine)
			reqCurrentInfraMachineBytes := bytes.TrimSuffix(req.Current.InfrastructureMachine.Raw, []byte("\n")) // Note: Somehow Patch introduces a trailing \n.
			if d := diff(reqCurrentInfraMachineBytes, currentInfraMachineBytes); d != "" {
				return fmt.Errorf("expected currentInfraMachine to be equal, got diff: %s", d)
			}
			desiredInfraMachine := machineUpToDateResult.DesiredInfraMachine.DeepCopy()
			desiredInfraMachine.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			desiredInfraMachineBytes, _ := json.Marshal(desiredInfraMachine)
			if d := diff(req.Desired.InfrastructureMachine.Raw, desiredInfraMachineBytes); d != "" {
				return fmt.Errorf("expected desiredInfraMachine to be equal, got diff: %s", d)
			}

			return nil
		default:
			return fmt.Errorf("unhandled request type %T", req)
		}
	}
}

func Test_createRequest(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "in-place-create-request")
	g.Expect(err).ToNot(HaveOccurred())

	currentMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: ns.Name,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "cluster-1",
				"label-1":                  "label-value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "annotation-value-1",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "cluster-1",
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: bootstrapv1.GroupVersion.Group,
					Kind:     "KubeadmConfig",
					Name:     "machine-to-in-place-update",
				},
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Kind:     builder.TestInfrastructureMachineKind,
				Name:     "machine-to-in-place-update",
			},
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDeletionTimeoutSeconds: ptr.To[int32](10),
			},
			Version: "v1.30.0",
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: "machine-to-in-place-update",
			},
		},
	}
	currentMachineCleanedUp := currentMachine.DeepCopy()
	currentMachineCleanedUp.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine")) // cleanupMachine adds GVK.
	currentMachineCleanedUp.Status = clusterv1.MachineStatus{}                              // cleanupMachine drops status.
	currentMachineWithFieldsSetByMachineController := currentMachine.DeepCopy()
	currentMachineWithFieldsSetByMachineController.Spec.ProviderID = "test://provider-id"
	currentMachineWithFieldsSetByMachineController.Spec.Bootstrap.DataSecretName = ptr.To("data-secret")
	currentMachineWithFieldsSetByMachineControllerCleanedUp := currentMachineCleanedUp.DeepCopy()
	currentMachineWithFieldsSetByMachineControllerCleanedUp.Spec.ProviderID = "test://provider-id"
	currentMachineWithFieldsSetByMachineControllerCleanedUp.Spec.Bootstrap.DataSecretName = ptr.To("data-secret")

	desiredMachine := currentMachine.DeepCopy()
	desiredMachine.Spec.Version = "v1.31.0"
	desiredMachineCleanedUp := desiredMachine.DeepCopy()
	desiredMachineCleanedUp.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine")) // cleanupMachine adds GVK.
	desiredMachineCleanedUp.Status = clusterv1.MachineStatus{}                              // cleanupMachine drops status.
	desiredMachineWithFieldsSetByMachineControllerCleanedUp := desiredMachineCleanedUp.DeepCopy()
	desiredMachineWithFieldsSetByMachineControllerCleanedUp.Spec.ProviderID = "test://provider-id"
	desiredMachineWithFieldsSetByMachineControllerCleanedUp.Spec.Bootstrap.DataSecretName = ptr.To("data-secret")

	currentKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: ns.Name,
			Labels: map[string]string{
				"label-1": "label-value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "annotation-value-1",
			},
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						ImageTag: "3.5.0-0",
					},
				},
			},
			JoinConfiguration: bootstrapv1.JoinConfiguration{
				// This field is technically set by CABPK, but adding it here so that matchesKubeadmConfig detects this correctly as a join KubeadmConfig.
				Discovery: bootstrapv1.Discovery{
					BootstrapToken: bootstrapv1.BootstrapTokenDiscovery{
						APIServerEndpoint: "1.2.3.4:6443",
					},
				},
				NodeRegistration: bootstrapv1.NodeRegistrationOptions{
					KubeletExtraArgs: []bootstrapv1.Arg{{
						Name:  "v",
						Value: ptr.To("8"),
					}},
				},
			},
		},
		Status: bootstrapv1.KubeadmConfigStatus{
			ObservedGeneration: 5,
		},
	}
	currentKubeadmConfigCleanedUp := currentKubeadmConfig.DeepCopy()
	currentKubeadmConfigCleanedUp.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig")) // cleanupKubeadmConfig adds GVK.
	currentKubeadmConfigCleanedUp.Status = bootstrapv1.KubeadmConfigStatus{}                              // cleanupKubeadmConfig drops status.
	defaulting.ApplyPreviousKubeadmConfigDefaults(&currentKubeadmConfigCleanedUp.Spec)                    // PrepareKubeadmConfigsForDiff applies defaults.
	currentKubeadmConfigCleanedUp.Spec.JoinConfiguration.Discovery = bootstrapv1.Discovery{}              // PrepareKubeadmConfigsForDiff cleans up Discovery.
	currentKubeadmConfigWithOutdatedLabelsAndAnnotations := currentKubeadmConfig.DeepCopy()
	currentKubeadmConfigWithOutdatedLabelsAndAnnotations.Labels["outdated-label-1"] = "outdated-label-value-1"
	currentKubeadmConfigWithOutdatedLabelsAndAnnotations.Annotations["outdated-annotation-1"] = "outdated-annotation-value-1"
	currentKubeadmConfigWithInitConfiguration := currentKubeadmConfig.DeepCopy()
	currentKubeadmConfigWithInitConfiguration.Spec.InitConfiguration.NodeRegistration = currentKubeadmConfigWithInitConfiguration.Spec.JoinConfiguration.NodeRegistration
	currentKubeadmConfigWithInitConfiguration.Spec.JoinConfiguration = bootstrapv1.JoinConfiguration{}

	desiredKubeadmConfig := currentKubeadmConfig.DeepCopy()
	desiredKubeadmConfig.Spec.ClusterConfiguration.Etcd.Local.ImageTag = "3.6.4-0"
	desiredKubeadmConfigCleanedUp := desiredKubeadmConfig.DeepCopy()
	desiredKubeadmConfigCleanedUp.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig")) // cleanupKubeadmConfig adds GVK.
	desiredKubeadmConfigCleanedUp.Status = bootstrapv1.KubeadmConfigStatus{}                              // cleanupKubeadmConfig drops status.
	defaulting.ApplyPreviousKubeadmConfigDefaults(&desiredKubeadmConfigCleanedUp.Spec)                    // PrepareKubeadmConfigsForDiff applies defaults.
	desiredKubeadmConfigCleanedUp.Spec.JoinConfiguration.Discovery = bootstrapv1.Discovery{}              // PrepareKubeadmConfigsForDiff cleans up Discovery.

	currentInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineKind,
			"metadata": map[string]interface{}{
				"name":      "machine-to-in-place-update",
				"namespace": ns.Name,
				"labels": map[string]interface{}{
					"label-1": "label-value-1",
				},
				"annotations": map[string]interface{}{
					clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template-1",
					clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
				},
			},
			"spec": map[string]interface{}{
				"foo": "hello world",
			},
			"status": map[string]interface{}{
				"foo": "hello world",
			},
		},
	}
	currentInfraMachineCleanedUp := currentInfraMachine.DeepCopy()
	unstructured.RemoveNestedField(currentInfraMachineCleanedUp.Object, "status") // cleanupUnstructured drops status.
	currentInfraMachineWithOutdatedLabelsAndAnnotations := currentInfraMachine.DeepCopy()
	currentInfraMachineWithOutdatedLabelsAndAnnotations.SetLabels(map[string]string{"outdated-label-1": "outdated-label-value-1"})
	currentInfraMachineWithOutdatedLabelsAndAnnotations.SetAnnotations(map[string]string{"outdated-annotation-1": "outdated-annotation-value-1"})
	currentInfraMachineWithFieldsSetByMachineController := currentInfraMachine.DeepCopy()
	g.Expect(unstructured.SetNestedField(currentInfraMachineWithFieldsSetByMachineController.Object, "hello world from the infra machine controller", "spec", "bar")).To(Succeed())
	currentInfraMachineWithFieldsSetByMachineControllerCleanedUp := currentInfraMachineCleanedUp.DeepCopy()
	g.Expect(unstructured.SetNestedField(currentInfraMachineWithFieldsSetByMachineControllerCleanedUp.Object, "hello world from the infra machine controller", "spec", "bar")).To(Succeed())

	desiredInfraMachine := currentInfraMachine.DeepCopy()
	g.Expect(unstructured.SetNestedField(desiredInfraMachine.Object, "hello in-place updated world", "spec", "foo")).To(Succeed())
	desiredInfraMachineCleanedUp := desiredInfraMachine.DeepCopy()
	unstructured.RemoveNestedField(desiredInfraMachineCleanedUp.Object, "status") // cleanupUnstructured drops status.
	desiredInfraMachineWithFieldsSetByMachineControllerCleanedUp := desiredInfraMachineCleanedUp.DeepCopy()
	g.Expect(unstructured.SetNestedField(desiredInfraMachineWithFieldsSetByMachineControllerCleanedUp.Object, "hello world from the infra machine controller", "spec", "bar")).To(Succeed())

	tests := []struct {
		name                          string
		currentMachine                *clusterv1.Machine
		currentInfraMachine           *unstructured.Unstructured
		currentKubeadmConfig          *bootstrapv1.KubeadmConfig
		desiredMachine                *clusterv1.Machine
		desiredInfraMachine           *unstructured.Unstructured
		desiredKubeadmConfig          *bootstrapv1.KubeadmConfig
		modifyMachineAfterCreate      func(ctx context.Context, c client.Client, machine *clusterv1.Machine) error
		modifyInfraMachineAfterCreate func(ctx context.Context, c client.Client, infraMachine *unstructured.Unstructured) error
		modifyUpToDateResult          func(result *internal.UpToDateResult)
		wantReq                       *runtimehooksv1.CanUpdateMachineRequest
		wantError                     bool
		wantErrorMessage              string
	}{
		{
			name:                 "Should prepare all objects for diff",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				// Objects have been cleaned up for the diff.
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfigCleanedUp),
				},
				Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *desiredMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(desiredInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(desiredKubeadmConfigCleanedUp),
				},
			},
		},
		{
			name:                 "Should prepare all objects for diff: syncs BootstrapConfig/InfraMachine labels and annotations",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
			modifyUpToDateResult: func(result *internal.UpToDateResult) {
				// Modify the UpToDateResult before it is passed into createRequest.
				// This covers the scenario where the "local" current objects are outdated.
				result.CurrentInfraMachine = currentInfraMachineWithOutdatedLabelsAndAnnotations
				result.CurrentKubeadmConfig = currentKubeadmConfigWithOutdatedLabelsAndAnnotations
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				// Current / desired BootstrapConfig / InfraMachine all contain the latest labels / annotations.
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfigCleanedUp),
				},
				Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *desiredMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(desiredInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(desiredKubeadmConfigCleanedUp),
				},
			},
		},
		{
			name:                 "Should prepare all objects for diff: desiredMachine picks up changes from currentMachine via SSA dry-run",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
			modifyMachineAfterCreate: func(ctx context.Context, c client.Client, machine *clusterv1.Machine) error {
				// Write additional fields like the Machine controller would do.
				machineOrig := machine.DeepCopy()
				machine.Spec.ProviderID = "test://provider-id"
				machine.Spec.Bootstrap.DataSecretName = ptr.To("data-secret")
				return c.Patch(ctx, machine, client.MergeFrom(machineOrig))
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					// currentMachine always contained the fields written by the Machine controller
					// as we pass the Machine object after modifyMachineAfterCreate into createRequest.
					Machine:               *currentMachineWithFieldsSetByMachineControllerCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfigCleanedUp),
				},
				Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
					// desiredMachine picked up the fields written by the Machine controller via SSA dry-run.
					Machine:               *desiredMachineWithFieldsSetByMachineControllerCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(desiredInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(desiredKubeadmConfigCleanedUp),
				},
			},
		},
		{
			name:                 "Should prepare all objects for diff: desiredInfraMachine picks up changes from currentInfraMachine via SSA dry-run",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
			modifyInfraMachineAfterCreate: func(ctx context.Context, c client.Client, infraMachine *unstructured.Unstructured) error {
				// Write additional fields like the Infra Machine controller would do.
				infraMachineOrig := infraMachine.DeepCopy()
				g.Expect(unstructured.SetNestedField(infraMachine.Object, "hello world from the infra machine controller", "spec", "bar")).To(Succeed())
				return c.Patch(ctx, infraMachine, client.MergeFrom(infraMachineOrig))
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine: *currentMachineCleanedUp,
					// currentInfraMachine always contained the fields written by the InfraMachine controller
					// as we pass the InfraMachine object after modifyInfraMachineAfterCreate into createRequest.
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachineWithFieldsSetByMachineControllerCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfigCleanedUp),
				},
				Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine: *desiredMachineCleanedUp,
					// desiredInfraMachine picked up the fields written by the InfraMachine controller via SSA dry-run.
					InfrastructureMachine: mustConvertToRawExtension(desiredInfraMachineWithFieldsSetByMachineControllerCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(desiredKubeadmConfigCleanedUp),
				},
			},
		},
		{
			name:                 "Should prepare all objects for diff: currentKubeadmConfig & desiredKubeadmConfig are prepared for diff",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfigWithInitConfiguration,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachineCleanedUp),
					// currentKubeadmConfig was converted from InitConfiguration to JoinConfiguration via PrepareKubeadmConfigsForDiff.
					BootstrapConfig: mustConvertToRawExtension(currentKubeadmConfigCleanedUp),
				},
				Desired: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *desiredMachineCleanedUp,
					InfrastructureMachine: mustConvertToRawExtension(desiredInfraMachineCleanedUp),
					BootstrapConfig:       mustConvertToRawExtension(desiredKubeadmConfigCleanedUp),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewWithT(t)

			// Create Machine (same as in createMachine)
			currentMachineForPatch := tt.currentMachine.DeepCopy()
			g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, currentMachineForPatch)).To(Succeed())
			t.Cleanup(func() {
				g.Expect(env.CleanupAndWait(context.Background(), tt.currentMachine)).To(Succeed())
			})

			// Create InfraMachine (same as in createInfraMachine)
			currentInfraMachineForPatch := tt.currentInfraMachine.DeepCopy()
			g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, currentInfraMachineForPatch)).To(Succeed())
			g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), currentInfraMachineForPatch, kcpManagerName)).To(Succeed())
			t.Cleanup(func() {
				g.Expect(env.CleanupAndWait(context.Background(), tt.currentInfraMachine)).To(Succeed())
			})

			// Create KubeadmConfig (same as in createKubeadmConfig)
			currentKubeadmConfigForPatch := tt.currentKubeadmConfig.DeepCopy()
			g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, currentKubeadmConfigForPatch)).To(Succeed())
			g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), currentKubeadmConfigForPatch, kcpManagerName)).To(Succeed())
			t.Cleanup(func() {
				g.Expect(env.CleanupAndWait(context.Background(), tt.currentKubeadmConfig)).To(Succeed())
			})

			if tt.modifyMachineAfterCreate != nil {
				g.Expect(tt.modifyMachineAfterCreate(ctx, env.Client, currentMachineForPatch)).To(Succeed())
			}
			if tt.modifyInfraMachineAfterCreate != nil {
				g.Expect(tt.modifyInfraMachineAfterCreate(ctx, env.Client, currentInfraMachineForPatch)).To(Succeed())
			}

			upToDateResult := internal.UpToDateResult{
				CurrentInfraMachine:  currentInfraMachineForPatch,
				CurrentKubeadmConfig: currentKubeadmConfigForPatch,
				DesiredMachine:       tt.desiredMachine,
				DesiredInfraMachine:  tt.desiredInfraMachine,
				DesiredKubeadmConfig: tt.desiredKubeadmConfig,
			}
			if tt.modifyUpToDateResult != nil {
				tt.modifyUpToDateResult(&upToDateResult)
			}

			req, err := createRequest(ctx, env.Client, currentMachineForPatch, upToDateResult)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(req).To(BeComparableTo(tt.wantReq))
		})
	}
}

func Test_applyPatchesToRequest(t *testing.T) {
	currentMachine := &clusterv1.Machine{
		// Set GVK because this is required by convertToRawExtension.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Version: "v1.30.0",
		},
	}
	patchedMachine := currentMachine.DeepCopy()
	patchedMachine.Spec.Version = "v1.31.0"

	currentKubeadmConfig := &bootstrapv1.KubeadmConfig{
		// Set GVK because this is required by convertToRawExtension.
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "KubeadmConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-to-in-place-update",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						ImageTag: "3.5.0-0",
					},
				},
			},
		},
	}
	patchedKubeadmConfig := currentKubeadmConfig.DeepCopy()
	patchedKubeadmConfig.Spec.ClusterConfiguration.Etcd.Local.ImageTag = "3.6.4-0"

	currentInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineKind,
			"metadata": map[string]interface{}{
				"name":      "machine-to-in-place-update",
				"namespace": metav1.NamespaceDefault,
				"annotations": map[string]interface{}{
					clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template-1",
					clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate.infrastructure.cluster.x-k8s.io",
				},
			},
			"spec": map[string]interface{}{
				"hello": "world",
			},
		},
	}
	patchedInfraMachine := currentInfraMachine.DeepCopy()
	_ = unstructured.SetNestedField(patchedInfraMachine.Object, "in-place updated world", "spec", "hello")

	responseWithEmptyPatches := &runtimehooksv1.CanUpdateMachineResponse{
		CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
		MachinePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONPatchType,
			Patch:     []byte("[]"),
		},
		InfrastructureMachinePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte{},
		},
		BootstrapConfigPatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte("{}"),
		},
	}
	patchToUpdateMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/version","value":"v1.31.0"}]`),
	}
	patchToUpdateKubeadmConfig := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/clusterConfiguration/etcd/local/imageTag","value":"3.6.4-0"}]`),
	}
	patchToUpdateInfraMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/hello","value":"in-place updated world"}]`),
	}
	jsonMergePatchToUpdateMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"version":"v1.31.0"}}`),
	}
	jsonMergePatchToUpdateKubeadmConfig := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"clusterConfiguration":{"etcd":{"local":{"imageTag":"3.6.4-0"}}}}}`),
	}
	jsonMergePatchToUpdateInfraMachine := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"hello":"in-place updated world"}}`),
	}
	patchToUpdateMachineStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}
	patchToUpdateKubeadmConfigStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}
	patchToUpdateInfraMachineStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}

	tests := []struct {
		name             string
		req              *runtimehooksv1.CanUpdateMachineRequest
		resp             *runtimehooksv1.CanUpdateMachineResponse
		wantReq          *runtimehooksv1.CanUpdateMachineRequest
		wantError        bool
		wantErrorMessage string
	}{
		{
			name: "No changes with no patches",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: responseWithEmptyPatches,
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
		},
		{
			name: "Changes with patches",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineResponse{
				CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachinePatch:               patchToUpdateMachine,
				InfrastructureMachinePatch: patchToUpdateInfraMachine,
				BootstrapConfigPatch:       patchToUpdateKubeadmConfig,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *patchedMachine,
					InfrastructureMachine: mustConvertToRawExtension(patchedInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(patchedKubeadmConfig),
				},
			},
		},
		{
			name: "Changes with JSON merge patches",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineResponse{
				CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachinePatch:               jsonMergePatchToUpdateMachine,
				InfrastructureMachinePatch: jsonMergePatchToUpdateInfraMachine,
				BootstrapConfigPatch:       jsonMergePatchToUpdateKubeadmConfig,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *patchedMachine,
					InfrastructureMachine: mustConvertToRawExtension(patchedInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(patchedKubeadmConfig),
				},
			},
		},
		{
			name: "No changes with status patches",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineResponse{
				CommonResponse:             runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachinePatch:               patchToUpdateMachineStatus,
				InfrastructureMachinePatch: patchToUpdateInfraMachineStatus,
				BootstrapConfigPatch:       patchToUpdateKubeadmConfigStatus,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
		},
		{
			name: "Error if PatchType is not set but Patch is",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineResponse{
				CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachinePatch: runtimehooksv1.Patch{
					// PatchType is missing
					Patch: []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
				},
			},
			wantError:        true,
			wantErrorMessage: "failed to apply patch: patchType is not set",
		},
		{
			name: "Error if PatchType is set to an unknown value",
			req: &runtimehooksv1.CanUpdateMachineRequest{
				Current: runtimehooksv1.CanUpdateMachineRequestObjects{
					Machine:               *currentMachine,
					InfrastructureMachine: mustConvertToRawExtension(currentInfraMachine),
					BootstrapConfig:       mustConvertToRawExtension(currentKubeadmConfig),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineResponse{
				CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachinePatch: runtimehooksv1.Patch{
					PatchType: "UnknownType",
					Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
				},
			},
			wantError:        true,
			wantErrorMessage: "failed to apply patch: unknown patchType UnknownType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := applyPatchesToRequest(ctx, tt.req, tt.resp)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Compare only the objects and avoid comparing runtime.RawExtension.Raw because
			// Raw is slightly non-deterministic because it doesn't guarantee order of map keys.
			g.Expect(tt.req.Current.Machine).To(BeComparableTo(tt.wantReq.Current.Machine))
			g.Expect(tt.req.Current.InfrastructureMachine.Object).To(BeComparableTo(tt.wantReq.Current.InfrastructureMachine.Object))
			g.Expect(tt.req.Current.BootstrapConfig.Object).To(BeComparableTo(tt.wantReq.Current.BootstrapConfig.Object))
		})
	}
}

func diff(a, b any) string {
	_, d, err := compare.Diff(a, b)
	if err != nil {
		return fmt.Sprintf("error during diff: %v", err)
	}
	return d
}

func mustConvertToRawExtension(object runtime.Object) runtime.RawExtension {
	raw, err := patch.ConvertToRawExtension(object)
	if err != nil {
		panic(err)
	}
	return raw
}

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

package machinedeployment

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/contract"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/patch"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_canUpdateMachineSetInPlace(t *testing.T) {
	ns := metav1.NamespaceDefault
	oldMS := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-machineset",
			Namespace: ns,
		},
	}
	newMS := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-machineset",
			Namespace: ns,
		},
	}
	oldMSInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(ns, "infrastructure-machine-template-1").Build()
	newMSInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(ns, "infrastructure-machine-template-2").Build()
	oldMSBootstrapConfigTemplate := builder.BootstrapTemplate(ns, "bootstrap-config-template-1").Build()
	newMSBootstrapConfigTemplate := builder.BootstrapTemplate(ns, "bootstrap-config-template-2").Build()

	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)
	canUpdateMachineSetGVH, err := catalog.GroupVersionHook(runtimehooksv1.CanUpdateMachineSet)
	if err != nil {
		panic("unable to compute GVH")
	}

	tests := []struct {
		name                                    string
		oldMS                                   *clusterv1.MachineSet
		newMS                                   *clusterv1.MachineSet
		oldMSInfrastructureMachineTemplate      *unstructured.Unstructured
		newMSInfrastructureMachineTemplate      *unstructured.Unstructured
		oldMSBootstrapConfigTemplate            *unstructured.Unstructured
		newMSBootstrapConfigTemplate            *unstructured.Unstructured
		canExtensionsUpdateMachineSetFunc       func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (bool, []string, error)
		getAllExtensionsResponses               map[runtimecatalog.GroupVersionHook][]string
		wantCanExtensionsUpdateMachineSetCalled bool
		wantCanUpdateMachineSet                 bool
		wantError                               bool
		wantErrorMessage                        string
		wantCacheEntry                          *CanUpdateMachineSetCacheEntry
	}{
		{
			name:             "Return error if oldMS infrastructureRef is not set",
			oldMS:            oldMS,
			newMS:            newMS,
			wantError:        true,
			wantErrorMessage: "failed to get InfrastructureMachineTemplate from MachineSet old-machineset: cannot get object - object reference not set",
		},
		{
			name:                               "Return error if newMS infrastructureRef is not set",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			wantError:                          true,
			wantErrorMessage:                   "failed to get InfrastructureMachineTemplate from MachineSet new-machineset: cannot get object - object reference not set",
		},
		{
			name:                               "Return false if bootstrap.configRef is only set on oldMS",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			wantCanUpdateMachineSet:            false,
		},
		{
			name:                               "Return false if bootstrap.configRef is only set on newMS",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			wantCanUpdateMachineSet:            false,
		},
		{
			name:                               "Return false if no CanUpdateMachineSet extensions registered",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses:          map[runtimecatalog.GroupVersionHook][]string{},
			wantCanUpdateMachineSet:            false,
		},
		{
			name:                               "Return error if more than one CanUpdateMachineSet extensions registered",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension-1", "test-update-extension-2"},
			},
			wantError:        true,
			wantErrorMessage: "found multiple CanUpdateMachineSet hooks (test-update-extension-1,test-update-extension-2): only one hook is supported",
		},
		{
			name:                               "Return false if canExtensionsUpdateMachineSet returns false",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (bool, []string, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return false, nil, errors.Errorf("unexpected error")
				}
				return false, []string{"can not update"}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSet:                 false,
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:               client.ObjectKeyFromObject(oldMS),
				NewMS:               client.ObjectKeyFromObject(newMS),
				CanUpdateMachineSet: false,
			},
		},
		{
			name:                               "Return true if canExtensionsUpdateMachineSet returns true",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (bool, []string, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return false, nil, errors.Errorf("unexpected error")
				}
				return true, nil, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSet:                 true,
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:               client.ObjectKeyFromObject(oldMS),
				NewMS:               client.ObjectKeyFromObject(newMS),
				CanUpdateMachineSet: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithGetAllExtensionResponses(tt.getAllExtensionsResponses).
				Build()

			oldMS := tt.oldMS.DeepCopy()
			newMS := tt.newMS.DeepCopy()

			objs := []client.Object{
				builder.GenericInfrastructureMachineTemplateCRD,
				builder.GenericBootstrapConfigTemplateCRD,
			}
			if tt.oldMSInfrastructureMachineTemplate != nil {
				objs = append(objs, tt.oldMSInfrastructureMachineTemplate)
				oldMS.Spec.Template.Spec.InfrastructureRef = contract.ObjToContractVersionedObjectReference(tt.oldMSInfrastructureMachineTemplate)
			}
			if tt.newMSInfrastructureMachineTemplate != nil {
				objs = append(objs, tt.newMSInfrastructureMachineTemplate)
				newMS.Spec.Template.Spec.InfrastructureRef = contract.ObjToContractVersionedObjectReference(tt.newMSInfrastructureMachineTemplate)
			}
			if tt.oldMSBootstrapConfigTemplate != nil {
				objs = append(objs, tt.oldMSBootstrapConfigTemplate)
				oldMS.Spec.Template.Spec.Bootstrap.ConfigRef = contract.ObjToContractVersionedObjectReference(tt.oldMSBootstrapConfigTemplate)
			}
			if tt.newMSBootstrapConfigTemplate != nil {
				objs = append(objs, tt.newMSBootstrapConfigTemplate)
				newMS.Spec.Template.Spec.Bootstrap.ConfigRef = contract.ObjToContractVersionedObjectReference(tt.newMSBootstrapConfigTemplate)
			}

			fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()

			canUpdateMachineSetCache := cache.New[CanUpdateMachineSetCacheEntry](cache.HookCacheDefaultTTL)

			var canExtensionsUpdateMachineSetCalled bool
			p := &rolloutPlanner{
				RuntimeClient:            runtimeClient,
				Client:                   fakeClient,
				canUpdateMachineSetCache: canUpdateMachineSetCache,
				overrideCanExtensionsUpdateMachineSet: func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (bool, []string, error) {
					canExtensionsUpdateMachineSetCalled = true
					return tt.canExtensionsUpdateMachineSetFunc(ctx, oldMS, newMS, templateObjects, extensionHandlers)
				},
			}

			canUpdateMachineSet, err := p.canUpdateMachineSetInPlace(ctx, oldMS, newMS)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(canUpdateMachineSet).To(Equal(tt.wantCanUpdateMachineSet))

			g.Expect(canExtensionsUpdateMachineSetCalled).To(Equal(tt.wantCanExtensionsUpdateMachineSetCalled), "canExtensionsUpdateMachineSetCalled: actual: %t expected: %t", canExtensionsUpdateMachineSetCalled, tt.wantCanExtensionsUpdateMachineSetCalled)

			cacheEntry, ok := canUpdateMachineSetCache.Has(CanUpdateMachineSetCacheEntry{
				OldMS: client.ObjectKeyFromObject(oldMS),
				NewMS: client.ObjectKeyFromObject(newMS),
			}.Key())
			if tt.wantCacheEntry == nil {
				g.Expect(ok).To(BeFalse())
				g.Expect(cacheEntry).To(Equal(CanUpdateMachineSetCacheEntry{}))
			} else {
				// Verify the cache entry.
				g.Expect(ok).To(BeTrue())
				g.Expect(cacheEntry).To(Equal(*tt.wantCacheEntry))

				// Call canUpdateMachineSetInPlace again and verify the cache hit.
				canExtensionsUpdateMachineSetCalled = false
				canUpdateMachineSet, err := p.canUpdateMachineSetInPlace(ctx, oldMS, newMS)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(canUpdateMachineSet).To(Equal(tt.wantCanUpdateMachineSet))
				g.Expect(canExtensionsUpdateMachineSetCalled).To(BeFalse())
			}
		})
	}
}

func Test_canExtensionsUpdateMachineSet(t *testing.T) {
	currentMachineSet := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "old-machineset",
			Namespace:       metav1.NamespaceDefault,
			ResourceVersion: "1", // This is set so we can verify cleanupMachineSet later.
		},
		Spec: clusterv1.MachineSetSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version: "v1.30.0",
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: builder.BootstrapGroupVersion.Group,
							Kind:     builder.TestBootstrapConfigTemplateKind,
							Name:     "bootstrap-config-template-1",
						},
					},
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachineTemplateKind,
						Name:     "infrastructure-machine-template-1",
					},
				},
			},
		},
	}
	desiredMachineSet := currentMachineSet.DeepCopy()
	desiredMachineSet.Name = "new-machineset"
	desiredMachineSet.Spec.Template.Spec.Version = "v1.31.0"
	// Note: Changes in refs should not influence the in-place rollout decision.
	desiredMachineSet.Spec.Template.Spec.Bootstrap.ConfigRef.Name = "bootstrap-config-template-2"
	desiredMachineSet.Spec.Template.Spec.InfrastructureRef.Name = "infrastructure-machine-template-2"

	currentBootstrapConfigTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"kind":       builder.TestBootstrapConfigTemplateKind,
			"metadata": map[string]interface{}{
				"name":            "bootstrap-config-template-1",
				"namespace":       metav1.NamespaceDefault,
				"resourceVersion": "2", // This is set so we can verify cleanupUnstructured later.
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world BootstrapConfigTemplate",
					},
				},
			},
		},
	}
	desiredBootstrapConfigTemplate := currentBootstrapConfigTemplate.DeepCopy()
	desiredBootstrapConfigTemplate.SetName("bootstrap-config-template-2")
	_ = unstructured.SetNestedField(desiredBootstrapConfigTemplate.Object, "in-place updated world BootstrapConfigTemplate", "spec", "template", "spec", "hello")

	currentInfraMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineTemplateKind,
			"metadata": map[string]interface{}{
				"name":            "infrastructure-machine-template-1",
				"namespace":       metav1.NamespaceDefault,
				"resourceVersion": "2", // This is set so we can verify cleanupUnstructured later.
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world InfraMachineTemplate",
					},
				},
			},
		},
	}
	desiredInfraMachineTemplate := currentInfraMachineTemplate.DeepCopy()
	desiredInfraMachineTemplate.SetName("infrastructure-machine-template-2")
	_ = unstructured.SetNestedField(desiredInfraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")

	responseWithEmptyPatches := &runtimehooksv1.CanUpdateMachineSetResponse{
		CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
		MachineSetPatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONPatchType,
			Patch:     []byte("[]"),
		},
		InfrastructureMachineTemplatePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte{},
		},
		BootstrapConfigTemplatePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte("{}"),
		},
	}
	patchToUpdateMachineSet := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/version","value":"v1.31.0"}]`),
	}
	patchToUpdateBootstrapConfigTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/hello","value":"in-place updated world BootstrapConfigTemplate"}]`),
	}
	patchToUpdateInfraMachineTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/hello","value":"in-place updated world InfraMachineTemplate"}]`),
	}
	emptyPatch := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte{},
	}

	tests := []struct {
		name                         string
		newMS                        *clusterv1.MachineSet
		templateObjects              *templateObjects
		extensionHandlers            []string
		callExtensionResponses       map[string]runtimehooksv1.ResponseObject
		callExtensionExpectedChanges map[string]func(runtime.Object)
		wantCanUpdateMachineSet      bool
		wantReasons                  []string
		wantError                    bool
		wantErrorMessage             string
	}{
		{
			name: "Return true if current and desired objects are equal and no patches are returned",
			// Note: canExtensionsUpdateMachineSet should never be called if the objects are equal, but this is a simple first test case.
			newMS: currentMachineSet,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    currentInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: currentBootstrapConfigTemplate,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": responseWithEmptyPatches,
			},
			wantCanUpdateMachineSet: true,
		},
		{
			name:  "Return false if current and desired objects are not equal and no patches are returned",
			newMS: desiredMachineSet,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    desiredInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: desiredBootstrapConfigTemplate,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": responseWithEmptyPatches,
			},
			wantCanUpdateMachineSet: false,
			wantReasons: []string{
				`MachineSet cannot be updated in-place: &v1beta2.MachineSet{
    TypeMeta:   {},
    ObjectMeta: {},
    Spec: v1beta2.MachineSetSpec{
      ClusterName: "",
      Replicas:    nil,
      Selector:    {},
      Template: v1beta2.MachineTemplateSpec{
        ObjectMeta: {},
        Spec: v1beta2.MachineSpec{
          ClusterName:       "",
          Bootstrap:         {},
          InfrastructureRef: {},
-         Version:           "v1.30.0",
+         Version:           "v1.31.0",
          ProviderID:        "",
          FailureDomain:     "",
          ... // 4 identical fields
        },
      },
      MachineNaming: {},
      Deletion:      {},
    },
    Status: {},
  }`,
				`TestBootstrapConfigTemplate cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{
-     "spec": map[string]any{
-       "template": map[string]any{"spec": map[string]any{"hello": string("world BootstrapConfigTemplate")}},
-     },
-   },
+   Object: map[string]any{
+     "spec": map[string]any{
+       "template": map[string]any{
+         "spec": map[string]any{"hello": string("in-place updated world BootstrapConfigTemplate")},
+       },
+     },
+   },
  }`,
				`TestInfrastructureMachineTemplate cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{
-     "spec": map[string]any{
-       "template": map[string]any{"spec": map[string]any{"hello": string("world InfraMachineTemplate")}},
-     },
-   },
+   Object: map[string]any{
+     "spec": map[string]any{
+       "template": map[string]any{
+         "spec": map[string]any{"hello": string("in-place updated world InfraMachineTemplate")},
+       },
+     },
+   },
  }`,
			},
		},
		{
			name:  "Return true if current and desired objects are not equal and patches are returned that account for all diffs",
			newMS: desiredMachineSet,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    desiredInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: desiredBootstrapConfigTemplate,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
				},
			},
			wantCanUpdateMachineSet: true,
		},
		{
			name:  "Return true if current and desired objects are not equal and patches are returned that account for all diffs (multiple extensions)",
			newMS: desiredMachineSet,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    desiredInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: desiredBootstrapConfigTemplate,
			},
			extensionHandlers: []string{"test-update-extension-1", "test-update-extension-2", "test-update-extension-3"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension-1": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonResponse:  runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachineSetPatch: patchToUpdateMachineSet,
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
				},
				"test-update-extension-3": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonResponse:               runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					BootstrapConfigTemplatePatch: patchToUpdateBootstrapConfigTemplate,
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machine, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machine.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
				"test-update-extension-3": func(object runtime.Object) {
					if machine, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machine.Spec.Template.Spec.Version = "v1.31.0"
					}
					if infraMachineTemplate, ok := object.(*unstructured.Unstructured); ok {
						if infraMachineTemplate.GroupVersionKind().Kind == builder.TestInfrastructureMachineTemplateKind {
							// After the call to test-update-extension-2 we expect that patchToUpdateInfraMachineTemplate is already applied.
							_ = unstructured.SetNestedField(infraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")
						}
					}
				},
			},
			wantCanUpdateMachineSet: true,
		},
		{
			name:  "Return false if current and desired objects are not equal and patches are returned that only account for some diffs",
			newMS: desiredMachineSet,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    desiredInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: desiredBootstrapConfigTemplate,
			},
			extensionHandlers: []string{"test-update-extension"},
			callExtensionResponses: map[string]runtimehooksv1.ResponseObject{
				"test-update-extension": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: emptyPatch,
					BootstrapConfigTemplatePatch:       emptyPatch,
				},
			},
			wantCanUpdateMachineSet: false,
			wantReasons: []string{
				`TestBootstrapConfigTemplate cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{
-     "spec": map[string]any{
-       "template": map[string]any{"spec": map[string]any{"hello": string("world BootstrapConfigTemplate")}},
-     },
-   },
+   Object: map[string]any{
+     "spec": map[string]any{
+       "template": map[string]any{
+         "spec": map[string]any{"hello": string("in-place updated world BootstrapConfigTemplate")},
+       },
+     },
+   },
  }`,
				`TestInfrastructureMachineTemplate cannot be updated in-place: &unstructured.Unstructured{
-   Object: map[string]any{
-     "spec": map[string]any{
-       "template": map[string]any{"spec": map[string]any{"hello": string("world InfraMachineTemplate")}},
-     },
-   },
+   Object: map[string]any{
+     "spec": map[string]any{
+       "template": map[string]any{
+         "spec": map[string]any{"hello": string("in-place updated world InfraMachineTemplate")},
+       },
+     },
+   },
  }`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			catalog := runtimecatalog.New()
			_ = runtimehooksv1.AddToCatalog(catalog)
			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithCallExtensionValidations(validateCanUpdateMachineSetRequests(currentMachineSet, tt.newMS, tt.templateObjects, tt.callExtensionExpectedChanges)).
				WithCallExtensionResponses(tt.callExtensionResponses).
				Build()

			p := &rolloutPlanner{
				RuntimeClient: runtimeClient,
			}

			canUpdateMachineSet, reasons, err := p.canExtensionsUpdateMachineSet(ctx, currentMachineSet, tt.newMS, tt.templateObjects, tt.extensionHandlers)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(canUpdateMachineSet).To(Equal(tt.wantCanUpdateMachineSet))
			g.Expect(reasons).To(BeComparableTo(tt.wantReasons))
		})
	}
}

func validateCanUpdateMachineSetRequests(currentMachineSet, newMS *clusterv1.MachineSet, templateObjects *templateObjects, callExtensionExpectedChanges map[string]func(runtime.Object)) func(name string, object runtimehooksv1.RequestObject) error {
	return func(name string, req runtimehooksv1.RequestObject) error {
		switch req := req.(type) {
		case *runtimehooksv1.CanUpdateMachineSetRequest:
			// Compare MachineSet
			currentMachineSet := currentMachineSet.DeepCopy()
			currentMachineSet.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineSet"))
			currentMachineSet.ResourceVersion = "" // cleanupMachineSet drops ResourceVersion.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentMachineSet)
			}
			if d := diff(req.Current.MachineSet, *currentMachineSet); d != "" {
				return fmt.Errorf("expected currentMachineSet to be equal, got diff: %s", d)
			}
			desiredMachineSet := newMS.DeepCopy()
			desiredMachineSet.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineSet"))
			desiredMachineSet.ResourceVersion = "" // cleanupMachineSet drops ResourceVersion.
			if d := diff(req.Desired.MachineSet, *desiredMachineSet); d != "" {
				return fmt.Errorf("expected desiredMachineSet to be equal, got diff: %s", d)
			}

			// Compare BootstrapConfigTemplate
			currentBootstrapConfigTemplate := templateObjects.CurrentBootstrapConfigTemplate.DeepCopy()
			currentBootstrapConfigTemplate.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentBootstrapConfigTemplate)
			}
			currentBootstrapConfigTemplateBytes, _ := json.Marshal(currentBootstrapConfigTemplate)
			if d := diff(req.Current.BootstrapConfigTemplate.Raw, currentBootstrapConfigTemplateBytes); d != "" {
				return fmt.Errorf("expected currentBootstrapConfigTemplate to be equal, got diff: %s", d)
			}
			desiredBootstrapConfigTemplate := templateObjects.DesiredBootstrapConfigTemplate.DeepCopy()
			desiredBootstrapConfigTemplate.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			desiredBootstrapConfigTemplateBytes, _ := json.Marshal(desiredBootstrapConfigTemplate)
			if d := diff(req.Desired.BootstrapConfigTemplate.Raw, desiredBootstrapConfigTemplateBytes); d != "" {
				return fmt.Errorf("expected desiredBootstrapConfigTemplate to be equal, got diff: %s", d)
			}

			// Compare InfraMachineTemplate
			currentInfraMachineTemplate := templateObjects.CurrentInfraMachineTemplate.DeepCopy()
			currentInfraMachineTemplate.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			if mutator, ok := callExtensionExpectedChanges[name]; ok {
				mutator(currentInfraMachineTemplate)
			}
			currentInfraMachineBytes, _ := json.Marshal(currentInfraMachineTemplate)
			reqCurrentInfraMachineTemplateBytes := bytes.TrimSuffix(req.Current.InfrastructureMachineTemplate.Raw, []byte("\n")) // Note: Somehow Patch introduces a trailing \n.
			if d := diff(reqCurrentInfraMachineTemplateBytes, currentInfraMachineBytes); d != "" {
				return fmt.Errorf("expected currentInfraMachineTemplate to be equal, got diff: %s", d)
			}
			desiredInfraMachineTemplate := templateObjects.DesiredInfraMachineTemplate.DeepCopy()
			desiredInfraMachineTemplate.SetResourceVersion("") // cleanupUnstructured drops ResourceVersion.
			desiredInfraMachineTemplateBytes, _ := json.Marshal(desiredInfraMachineTemplate)
			if d := diff(req.Desired.InfrastructureMachineTemplate.Raw, desiredInfraMachineTemplateBytes); d != "" {
				return fmt.Errorf("expected desiredInfraMachineTemplate to be equal, got diff: %s", d)
			}

			return nil
		default:
			return fmt.Errorf("unhandled request type %T", req)
		}
	}
}

func Test_applyPatchesToRequest(t *testing.T) {
	currentMachineSet := &clusterv1.MachineSet{
		// Set GVK because this is required by convertToRawExtension.
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machineset-to-in-place-update",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version: "v1.30.0",
				},
			},
		},
	}
	patchedMachineSet := currentMachineSet.DeepCopy()
	patchedMachineSet.Spec.Template.Spec.Version = "v1.31.0"

	currentBootstrapConfigTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"kind":       builder.TestBootstrapConfigTemplateKind,
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-template-1",
				"namespace": metav1.NamespaceDefault,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world BootstrapConfigTemplate",
					},
				},
			},
		},
	}
	patchedBootstrapConfigTemplate := currentBootstrapConfigTemplate.DeepCopy()
	_ = unstructured.SetNestedField(patchedBootstrapConfigTemplate.Object, "in-place updated world BootstrapConfigTemplate", "spec", "template", "spec", "hello")

	currentInfraMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineTemplateKind,
			"metadata": map[string]interface{}{
				"name":      "infrastructure-machine-template-1",
				"namespace": metav1.NamespaceDefault,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world InfraMachineTemplate",
					},
				},
			},
		},
	}
	patchedInfraMachineTemplate := currentInfraMachineTemplate.DeepCopy()
	_ = unstructured.SetNestedField(patchedInfraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")

	responseWithEmptyPatches := &runtimehooksv1.CanUpdateMachineSetResponse{
		CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
		MachineSetPatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONPatchType,
			Patch:     []byte("[]"),
		},
		InfrastructureMachineTemplatePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte{},
		},
		BootstrapConfigTemplatePatch: runtimehooksv1.Patch{
			PatchType: runtimehooksv1.JSONMergePatchType,
			Patch:     []byte("{}"),
		},
	}
	patchToUpdateMachineSet := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/version","value":"v1.31.0"}]`),
	}
	patchToUpdateBootstrapConfigTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/hello","value":"in-place updated world BootstrapConfigTemplate"}]`),
	}
	patchToUpdateInfraMachineTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"replace","path":"/spec/template/spec/hello","value":"in-place updated world InfraMachineTemplate"}]`),
	}
	jsonMergePatchToUpdateMachineSet := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"template":{"spec":{"version":"v1.31.0"}}}}`),
	}
	jsonMergePatchToUpdateBootstrapConfigTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"template":{"spec":{"hello":"in-place updated world BootstrapConfigTemplate"}}}}`),
	}
	jsonMergePatchToUpdateInfraMachineTemplate := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONMergePatchType,
		Patch:     []byte(`{"spec":{"template":{"spec":{"hello":"in-place updated world InfraMachineTemplate"}}}}`),
	}
	patchToUpdateMachineSetStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}
	patchToUpdateBootstrapConfigTemplateStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}
	patchToUpdateInfraMachineTemplateStatus := runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
	}

	tests := []struct {
		name             string
		req              *runtimehooksv1.CanUpdateMachineSetRequest
		resp             *runtimehooksv1.CanUpdateMachineSetResponse
		wantReq          *runtimehooksv1.CanUpdateMachineSetRequest
		wantError        bool
		wantErrorMessage string
	}{
		{
			name: "No changes with no patches",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: responseWithEmptyPatches,
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
		},
		{
			name: "Changes with patches",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineSetResponse{
				CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachineSetPatch:                    patchToUpdateMachineSet,
				InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
				BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *patchedMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(patchedInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(patchedBootstrapConfigTemplate),
				},
			},
		},
		{
			name: "Changes with JSON merge patches",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineSetResponse{
				CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachineSetPatch:                    jsonMergePatchToUpdateMachineSet,
				InfrastructureMachineTemplatePatch: jsonMergePatchToUpdateInfraMachineTemplate,
				BootstrapConfigTemplatePatch:       jsonMergePatchToUpdateBootstrapConfigTemplate,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *patchedMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(patchedInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(patchedBootstrapConfigTemplate),
				},
			},
		},
		{
			name: "No changes with status patches",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineSetResponse{
				CommonResponse:                     runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachineSetPatch:                    patchToUpdateMachineSetStatus,
				InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplateStatus,
				BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplateStatus,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
		},
		{
			name: "Error if PatchType is not set but Patch is",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineSetResponse{
				CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachineSetPatch: runtimehooksv1.Patch{
					// PatchType is missing
					Patch: []byte(`[{"op":"add","path":"/status","value":{"observedGeneration": 10}}]`),
				},
			},
			wantError:        true,
			wantErrorMessage: "failed to apply patch: patchType is not set",
		},
		{
			name: "Error if PatchType is set to an unknown value",
			req: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *currentMachineSet,
					InfrastructureMachineTemplate: mustConvertToRawExtension(currentInfraMachineTemplate),
					BootstrapConfigTemplate:       mustConvertToRawExtension(currentBootstrapConfigTemplate),
				},
			},
			resp: &runtimehooksv1.CanUpdateMachineSetResponse{
				CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess},
				MachineSetPatch: runtimehooksv1.Patch{
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
			g.Expect(tt.req.Current.MachineSet).To(BeComparableTo(tt.wantReq.Current.MachineSet))
			g.Expect(tt.req.Current.InfrastructureMachineTemplate.Object).To(BeComparableTo(tt.wantReq.Current.InfrastructureMachineTemplate.Object))
			g.Expect(tt.req.Current.BootstrapConfigTemplate.Object).To(BeComparableTo(tt.wantReq.Current.BootstrapConfigTemplate.Object))
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

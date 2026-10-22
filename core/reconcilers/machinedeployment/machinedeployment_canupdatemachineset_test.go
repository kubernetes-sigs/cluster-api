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
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/api/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/contract"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/internal/util/patch"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_canUpdateMachineSetInPlace(t *testing.T) {
	now := time.Now()
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
		name                               string
		oldMS                              *clusterv1.MachineSet
		newMS                              *clusterv1.MachineSet
		oldMSInfrastructureMachineTemplate *unstructured.Unstructured
		newMSInfrastructureMachineTemplate *unstructured.Unstructured
		oldMSBootstrapConfigTemplate       *unstructured.Unstructured
		newMSBootstrapConfigTemplate       *unstructured.Unstructured
		// cacheEntry is an entry that already exists in the cache when canUpdateMachineSetInPlace is called.
		cacheEntry                        *CanUpdateMachineSetCacheEntry
		canExtensionsUpdateMachineSetFunc func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error)
		getAllExtensionsResponses         map[runtimecatalog.GroupVersionHook][]string

		wantCanExtensionsUpdateMachineSetCalled bool
		wantCanUpdateMachineSetResult           canUpdateMachineSetResult
		wantCacheEntry                          *CanUpdateMachineSetCacheEntry
		wantError                               bool
		wantErrorMessage                        string
	}{
		// Templates missing

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
			wantCanUpdateMachineSetResult:      canUpdateMachineSetResult{canUpdateMachineSet: false},
		},
		{
			name:                               "Return false if bootstrap.configRef is only set on newMS",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			wantCanUpdateMachineSetResult:      canUpdateMachineSetResult{canUpdateMachineSet: false},
		},

		// Invalid extension configuration
		{
			name:                               "Return false if no CanUpdateMachineSet extensions registered",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses:          map[runtimecatalog.GroupVersionHook][]string{},
			wantCanUpdateMachineSetResult:      canUpdateMachineSetResult{canUpdateMachineSet: false},
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

		// Without cache entry, allowed operations not set
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
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: false, reasons: []string{"can not update"}, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: false, reasons: []string{"can not update"}, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: false, reasons: []string{"can not update"}, retryAfter: now.Add(5 * time.Minute)},
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
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, retryAfter: now.Add(5 * time.Minute)},
			},
		},
		{
			name:                               "Return true if canExtensionsUpdateMachineSet returns true - does not affect availability",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, retryAfter: now.Add(5 * time.Minute)},
			},
		},

		// Without cache entry, allowed operations set
		{
			name:                               "Return allowed operations and messages if canExtensionsUpdateMachineSet returns them",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}), messages: []string{"only in-place"}, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}), messages: []string{"only in-place"}, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}), messages: []string{"only in-place"}, retryAfter: now.Add(5 * time.Minute)},
			},
		},
		{
			name:                               "Return that no operation is allowed if canExtensionsUpdateMachineSet returns it",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"preflight check failed"}, reasons: []string{"no operation is allowed"}, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"preflight check failed"}, reasons: []string{"no operation is allowed"}, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"preflight check failed"}, reasons: []string{"no operation is allowed"}, retryAfter: now.Add(5 * time.Minute)},
			},
		},

		// Without cache entry, retry after
		{
			name:                               "Return retryAfter if canExtensionsUpdateMachineSet returns retryAfter with messages",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"message 1", "message 2"}, retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"message 1", "message 2"}, retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"message 1", "message 2"}, retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
			},
		},
		{
			name:                               "Return retryAfter if canExtensionsUpdateMachineSet returns retryAfter without messages",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
			},
		},

		// Error calling the extension
		{
			name:                               "Return error and don't cache if canExtensionsUpdateMachineSet returns an error",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, _ []string) (canUpdateMachineSetResult, error) {
				return canUpdateMachineSetResult{canUpdateMachineSet: false, retryAfter: now.Add(10 * time.Second)}, pkgerrors.Errorf("extension failed")
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{internalErrorMessage: "extension failed"},
			wantError:                               true,
			wantErrorMessage:                        "extension failed",
		},

		// Cache hits and misses.
		{
			name:                               "Return cached requeueAfter without calling extensions if a valid cache entry with a retry request exists",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(1 * time.Hour), retryAfterFromExtension: true, messages: []string{"waiting for ..."}},
			},
			wantCanExtensionsUpdateMachineSetCalled: false,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{retryAfter: now.Add(1 * time.Hour), retryAfterFromExtension: true, messages: []string{"waiting for ..."}},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(1 * time.Hour), retryAfterFromExtension: true, messages: []string{"waiting for ..."}},
			},
		},
		{
			name:                               "Return cached result without calling extensions if a valid cache entry exists (cannot update)",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: false, reasons: []string{"cached reason"}, messages: []string{"cached message"}},
			},
			wantCanExtensionsUpdateMachineSetCalled: false,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: false, reasons: []string{"cached reason"}, messages: []string{"cached message"}},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: false, reasons: []string{"cached reason"}, messages: []string{"cached message"}},
			},
		},
		{
			name:                               "Return cached result without calling extensions if a valid cache entry exists (can update)",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"cached message"}},
			},
			wantCanExtensionsUpdateMachineSetCalled: false,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"cached message"}},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(10 * time.Minute), canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"cached message"}},
			},
		},
		{
			name:                               "Return cached disallowed operations without calling extensions if a valid cache entry exists",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(2 * time.Minute), allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"cached message"}},
			},
			wantCanExtensionsUpdateMachineSetCalled: false,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{retryAfter: now.Add(2 * time.Minute), allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"cached message"}},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(2 * time.Minute), allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"cached message"}},
			},
		},
		{
			name:                               "Call extensions again if the cache entry is expired (previous result was a retry request)",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{retryAfter: now.Add(-1 * time.Second), retryAfterFromExtension: true, messages: []string{"waiting for the update plan"}},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)},
			},
		},
		{
			name:                               "Call extensions again if the cache entry belongs to a different newMS",
			oldMS:                              oldMS,
			newMS:                              newMS,
			oldMSInfrastructureMachineTemplate: oldMSInfrastructureMachineTemplate,
			newMSInfrastructureMachineTemplate: newMSInfrastructureMachineTemplate,
			oldMSBootstrapConfigTemplate:       oldMSBootstrapConfigTemplate,
			newMSBootstrapConfigTemplate:       newMSBootstrapConfigTemplate,
			getAllExtensionsResponses: map[runtimecatalog.GroupVersionHook][]string{
				canUpdateMachineSetGVH: {"test-update-extension"},
			},
			cacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS: client.ObjectKeyFromObject(oldMS),
				NewMS: client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "other-new-machineset"},
				canUpdateMachineSetResult: canUpdateMachineSetResult{
					retryAfter:          now.Add(10 * time.Minute),
					canUpdateMachineSet: false,
					reasons:             []string{"cached reason"},
				},
			},
			canExtensionsUpdateMachineSetFunc: func(_ context.Context, _, _ *clusterv1.MachineSet, _ *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
				if len(extensionHandlers) != 1 || extensionHandlers[0] != "test-update-extension" {
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, pkgerrors.Errorf("unexpected error")
				}
				return canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)}, nil
			},
			wantCanExtensionsUpdateMachineSetCalled: true,
			wantCanUpdateMachineSetResult:           canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)},
			wantCacheEntry: &CanUpdateMachineSetCacheEntry{
				OldMS:                     client.ObjectKeyFromObject(oldMS),
				NewMS:                     client.ObjectKeyFromObject(newMS),
				canUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, retryAfter: now.Add(5 * time.Minute)},
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

			var canExtensionsUpdateMachineSetCalled bool
			p := &rolloutPlanner{
				RuntimeClient:            runtimeClient,
				Client:                   fakeClient,
				canUpdateMachineSetCache: cache.New[CanUpdateMachineSetCacheEntry](t.Context(), 30*time.Minute),
				overrideNowFunc:          func() time.Time { return now },
				overrideCanExtensionsUpdateMachineSet: func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet, templateObjects *templateObjects, extensionHandlers []string) (canUpdateMachineSetResult, error) {
					canExtensionsUpdateMachineSetCalled = true
					return tt.canExtensionsUpdateMachineSetFunc(ctx, oldMS, newMS, templateObjects, extensionHandlers)
				},
			}
			if tt.cacheEntry != nil {
				p.canUpdateMachineSetCache.Add(*tt.cacheEntry)
			}

			res, err := p.canUpdateMachineSetInPlace(ctx, oldMS, newMS)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(BeComparableTo(tt.wantCanUpdateMachineSetResult, cmp.AllowUnexported(canUpdateMachineSetResult{})))

			g.Expect(canExtensionsUpdateMachineSetCalled).To(Equal(tt.wantCanExtensionsUpdateMachineSetCalled), "canExtensionsUpdateMachineSetCalled: actual: %t expected: %t", canExtensionsUpdateMachineSetCalled, tt.wantCanExtensionsUpdateMachineSetCalled)

			entry, ok := p.canUpdateMachineSetCache.Has(CanUpdateMachineSetCacheEntry{
				OldMS: client.ObjectKeyFromObject(oldMS),
				NewMS: client.ObjectKeyFromObject(newMS),
			}.Key())
			if tt.wantCacheEntry == nil {
				g.Expect(ok).To(BeFalse())
				return
			}
			g.Expect(ok).To(BeTrue())
			g.Expect(entry).To(BeComparableTo(*tt.wantCacheEntry, cmp.AllowUnexported(CanUpdateMachineSetCacheEntry{}, canUpdateMachineSetResult{})))
		})
	}
}

func Test_canExtensionsUpdateMachineSet(t *testing.T) {
	now := time.Now()
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
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "no changes handled"}},
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

	wantReasonsNoPatches := []string{
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
	}

	wantReasonsSomePatches := []string{
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
	}

	tests := []struct {
		name                          string
		newMS                         *clusterv1.MachineSet
		templateObjects               *templateObjects
		extensionHandlers             []string
		callExtensionResponses        map[string]runtimehooksv1.ResponseObject
		callExtensionExpectedChanges  map[string]func(runtime.Object)
		wantCanUpdateMachineSetResult canUpdateMachineSetResult
		wantError                     bool
		wantErrorMessage              string
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
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, messages: []string{"no changes handled"}, retryAfter: now.Add(5 * time.Minute)},
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
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: false, affectsAvailability: false, messages: []string{"no changes handled"}, reasons: wantReasonsNoPatches, retryAfter: now.Add(5 * time.Minute)},
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:                new(false),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, retryAfter: now.Add(5 * time.Minute)},
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					MachineSetPatch:     patchToUpdateMachineSet,
					AffectsAvailability: new(false),
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					// AffectsAvailability not set => defaults to true.
				},
				"test-update-extension-3": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:          runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					BootstrapConfigTemplatePatch: patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:          new(false),
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
				"test-update-extension-3": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
					if infraMachineTemplate, ok := object.(*unstructured.Unstructured); ok {
						if infraMachineTemplate.GroupVersionKind().Kind == builder.TestInfrastructureMachineTemplateKind {
							// After the call to test-update-extension-2 we expect that patchToUpdateInfraMachineTemplate is already applied.
							_ = unstructured.SetNestedField(infraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")
						}
					}
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, retryAfter: now.Add(5 * time.Minute)},
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "only handled the MachineSet"}},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: emptyPatch,
					BootstrapConfigTemplatePatch:       emptyPatch,
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: false, affectsAvailability: false, messages: []string{"only handled the MachineSet"}, reasons: wantReasonsSomePatches, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name:  "Return requeueAfter if the extension returns RetryAfterSeconds (patches in the response are ignored)",
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "waiting for the update plan"}, RetryAfterSeconds: 10},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:                new(false),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{messages: []string{"waiting for the update plan"}, retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
		},
		{
			name: "Return requeueAfter as soon as one of multiple extensions returns RetryAfterSeconds (following extensions are not called)",
			// Note: Calling test-update-extension-3 would panic, so we use that to verify it is not called.
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "message of extension 1"}},
					MachineSetPatch:     patchToUpdateMachineSet,
					AffectsAvailability: new(false),
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "message of extension 2"}, RetryAfterSeconds: 5},
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{messages: []string{"message of extension 1", "message of extension 2"}, retryAfter: now.Add(5 * time.Second), retryAfterFromExtension: true},
		},
		{
			name:  "Return true and disallow InPlaceUpdate if allowedOperations contains only Replace",
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "only replace"}},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:                new(false),
					AllowedOperations:                  ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace}),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace}), messages: []string{"only replace"}, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name:  "Return false and allow InPlaceUpdate if allowedOperations contains only InPlaceUpdate and patches only account for some diffs",
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "only handled the MachineSet"}},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: emptyPatch,
					BootstrapConfigTemplatePatch:       emptyPatch,
					AllowedOperations:                  ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: false, allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}), messages: []string{"only handled the MachineSet"}, reasons: wantReasonsSomePatches, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name:  "Return no operation allowed if allowedOperations is an empty list (patches in the response are ignored)",
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
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "preflight check failed"}},
					MachineSetPatch:                    patchToUpdateMachineSet,
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					BootstrapConfigTemplatePatch:       patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:                new(false),
					AllowedOperations:                  ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"preflight check failed"}, reasons: []string{"no operation is allowed by extensions"}, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name: "Return no operation allowed as soon as the allowed operations of the extensions have no operation in common (following extensions are not called)",
			// Note: Calling test-update-extension-3 would panic, so we use that to verify it is not called.
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "message of extension 1"}},
					MachineSetPatch:     patchToUpdateMachineSet,
					AffectsAvailability: new(false),
					AllowedOperations:   ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "message of extension 2"}},
					AllowedOperations:   ptr.To([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace}),
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{allowedOperations: ptr.To([]runtimehooksv1.MachineUpdateOperation{}), messages: []string{"message of extension 1", "message of extension 2"}, reasons: []string{"no operation is allowed by extensions"}, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name:  "Ignore allowedOperations if the extension returns RetryAfterSeconds",
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess, Message: "waiting for the update plan"}, RetryAfterSeconds: 10},
					AllowedOperations:   ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{messages: []string{"waiting for the update plan"}, retryAfter: now.Add(10 * time.Second), retryAfterFromExtension: true},
		},
		{
			name:  "Return error if the extension returns a failure (even if RetryAfterSeconds is set)",
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
						CommonResponse:    runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusFailure, Message: "failed"},
						RetryAfterSeconds: 10,
					},
				},
			},
			wantError:        true,
			wantErrorMessage: `failed to call extension handler "test-update-extension": got failure response with message: failed`,
		},

		{
			name:  "Return true if current and desired objects are not equal and patches are returned that account for all diffs (multiple extensions) - does not affect availability",
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					MachineSetPatch:     patchToUpdateMachineSet,
					AffectsAvailability: new(false),
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					AffectsAvailability:                new(false),
				},
				"test-update-extension-3": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:          runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					BootstrapConfigTemplatePatch: patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:          new(false),
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
				"test-update-extension-3": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
					if infraMachineTemplate, ok := object.(*unstructured.Unstructured); ok {
						if infraMachineTemplate.GroupVersionKind().Kind == builder.TestInfrastructureMachineTemplateKind {
							// After the call to test-update-extension-2 we expect that patchToUpdateInfraMachineTemplate is already applied.
							_ = unstructured.SetNestedField(infraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")
						}
					}
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false, retryAfter: now.Add(5 * time.Minute)},
		},
		{
			name:  "Return true if current and desired objects are not equal and patches are returned that account for all diffs (multiple extensions) - at least one affects availability",
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
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					MachineSetPatch:     patchToUpdateMachineSet,
					AffectsAvailability: new(false),
				},
				"test-update-extension-2": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					InfrastructureMachineTemplatePatch: patchToUpdateInfraMachineTemplate,
					AffectsAvailability:                new(true),
				},
				"test-update-extension-3": &runtimehooksv1.CanUpdateMachineSetResponse{
					CommonRetryResponse:          runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
					BootstrapConfigTemplatePatch: patchToUpdateBootstrapConfigTemplate,
					AffectsAvailability:          new(false),
				},
			},
			callExtensionExpectedChanges: map[string]func(runtime.Object){
				"test-update-extension-2": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
				},
				"test-update-extension-3": func(object runtime.Object) {
					if machineSet, ok := object.(*clusterv1.MachineSet); ok {
						// After the call to test-update-extension-1 we expect that patchToUpdateMachineSet is already applied.
						machineSet.Spec.Template.Spec.Version = "v1.31.0"
					}
					if infraMachineTemplate, ok := object.(*unstructured.Unstructured); ok {
						if infraMachineTemplate.GroupVersionKind().Kind == builder.TestInfrastructureMachineTemplateKind {
							// After the call to test-update-extension-2 we expect that patchToUpdateInfraMachineTemplate is already applied.
							_ = unstructured.SetNestedField(infraMachineTemplate.Object, "in-place updated world InfraMachineTemplate", "spec", "template", "spec", "hello")
						}
					}
				},
			},
			wantCanUpdateMachineSetResult: canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true, retryAfter: now.Add(5 * time.Minute)},
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
				RuntimeClient:   runtimeClient,
				overrideNowFunc: func() time.Time { return now },
			}

			res, err := p.canExtensionsUpdateMachineSet(ctx, currentMachineSet, tt.newMS, tt.templateObjects, tt.extensionHandlers)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(Equal(tt.wantCanUpdateMachineSetResult))
		})
	}
}

func Test_intersectAllowedOperations(t *testing.T) {
	inPlaceUpdate := runtimehooksv1.MachineUpdateOperationUpdateInPlace
	replace := runtimehooksv1.MachineUpdateOperationReplace

	tests := []struct {
		name string
		a    *[]runtimehooksv1.MachineUpdateOperation
		b    *[]runtimehooksv1.MachineUpdateOperation
		want *[]runtimehooksv1.MachineUpdateOperation
	}{
		{
			name: "No opinion if both are not set",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "No opinion of a: b is returned",
			a:    nil,
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate}),
		},
		{
			name: "No opinion of b: a is returned",
			a:    ptr.To([]runtimehooksv1.MachineUpdateOperation{replace}),
			b:    nil,
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{replace}),
		},
		{
			name: "Empty list stays empty if the other one has no opinion",
			a:    ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
			b:    nil,
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
		},
		{
			name: "Empty list blocks everything",
			a:    ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate, replace}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
		},
		{
			name: "Only operations allowed by both are returned",
			a:    ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate, replace}),
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate}),
		},
		{
			name: "No operation in common",
			a:    ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate}),
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{replace}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
		},
		{
			name: "Unknown operations are ignored",
			a:    nil,
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{"SomethingNew"}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{}),
		},
		{
			name: "Duplicates are removed",
			a:    nil,
			b:    ptr.To([]runtimehooksv1.MachineUpdateOperation{replace, inPlaceUpdate, replace}),
			want: ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate, replace}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := intersectAllowedOperations(tt.a, tt.b)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
			} else {
				g.Expect(got).ToNot(BeNil())
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func Test_isOperationAllowed(t *testing.T) {
	g := NewWithT(t)

	inPlaceUpdate := runtimehooksv1.MachineUpdateOperationUpdateInPlace
	replace := runtimehooksv1.MachineUpdateOperationReplace

	// nil means no opinion, i.e. all operations are allowed.
	g.Expect(isOperationAllowed(nil, inPlaceUpdate)).To(BeTrue())
	g.Expect(isOperationAllowed(nil, replace)).To(BeTrue())

	// An empty list means that no operation is allowed.
	noOperations := ptr.To([]runtimehooksv1.MachineUpdateOperation{})
	g.Expect(isOperationAllowed(noOperations, inPlaceUpdate)).To(BeFalse())
	g.Expect(isOperationAllowed(noOperations, replace)).To(BeFalse())

	onlyInPlaceUpdate := ptr.To([]runtimehooksv1.MachineUpdateOperation{inPlaceUpdate})
	g.Expect(isOperationAllowed(onlyInPlaceUpdate, inPlaceUpdate)).To(BeTrue())
	g.Expect(isOperationAllowed(onlyInPlaceUpdate, replace)).To(BeFalse())
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

func Test_createRequest(t *testing.T) {
	// Note: In contrast to KCP, createRequest for MachineSets is a pure function: There is no SSA dry-run, no up-to-date result
	// and no cannotUpdateMachineReason, so only the cleanup of the objects that are sent to the extension has to be covered.
	oldMS := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-machineset",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "cluster-1",
				"label-1":                  "label-value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "annotation-value-1",
			},
			// Fields that must not be part of the request.
			UID:             "old-machineset-uid",
			ResourceVersion: "1",
			Generation:      3,
			ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "manager"}},
			OwnerReferences: []metav1.OwnerReference{{Name: "machine-deployment"}},
			Finalizers:      []string{"finalizer"},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: "cluster-1",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "cluster-1",
					Version:     "v1.30.0",
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
		Status: clusterv1.MachineSetStatus{
			Replicas: ptr.To[int32](3),
		},
	}
	newMS := oldMS.DeepCopy()
	newMS.Name = "new-machineset"
	newMS.UID = "new-machineset-uid"
	newMS.Spec.Template.Spec.Version = "v1.31.0"
	newMS.Spec.Template.Spec.Bootstrap.ConfigRef.Name = "bootstrap-config-template-2"
	newMS.Spec.Template.Spec.InfrastructureRef.Name = "infrastructure-machine-template-2"

	oldMSCleanedUp := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        oldMS.Name,
			Namespace:   oldMS.Namespace,
			Labels:      oldMS.Labels,
			Annotations: oldMS.Annotations,
		},
		Spec: oldMS.Spec,
	}
	newMSCleanedUp := oldMSCleanedUp.DeepCopy()
	newMSCleanedUp.Name = newMS.Name
	newMSCleanedUp.Spec = newMS.Spec

	newTemplate := func(kind, apiVersion, name, hello string) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": apiVersion,
				"kind":       kind,
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": metav1.NamespaceDefault,
					"labels": map[string]interface{}{
						"label-1": "label-value-1",
					},
					"annotations": map[string]interface{}{
						"annotation-1": "annotation-value-1",
					},
					// Fields that must not be part of the request.
					"uid":             name + "-uid",
					"resourceVersion": "2",
					"generation":      int64(4),
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"hello": hello,
						},
					},
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
		}
	}
	cleanedUp := func(u *unstructured.Unstructured) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": u.GetAPIVersion(),
				"kind":       u.GetKind(),
				"metadata": map[string]interface{}{
					"name":      u.GetName(),
					"namespace": u.GetNamespace(),
					"labels": map[string]interface{}{
						"label-1": "label-value-1",
					},
					"annotations": map[string]interface{}{
						"annotation-1": "annotation-value-1",
					},
				},
				"spec": u.Object["spec"],
			},
		}
	}
	currentInfraMachineTemplate := newTemplate(builder.TestInfrastructureMachineTemplateKind, builder.InfrastructureGroupVersion.String(), "infrastructure-machine-template-1", "world InfraMachineTemplate")
	desiredInfraMachineTemplate := newTemplate(builder.TestInfrastructureMachineTemplateKind, builder.InfrastructureGroupVersion.String(), "infrastructure-machine-template-2", "in-place updated world InfraMachineTemplate")
	currentBootstrapConfigTemplate := newTemplate(builder.TestBootstrapConfigTemplateKind, builder.BootstrapGroupVersion.String(), "bootstrap-config-template-1", "world BootstrapConfigTemplate")
	desiredBootstrapConfigTemplate := newTemplate(builder.TestBootstrapConfigTemplateKind, builder.BootstrapGroupVersion.String(), "bootstrap-config-template-2", "in-place updated world BootstrapConfigTemplate")

	tests := []struct {
		name            string
		oldMS           *clusterv1.MachineSet
		newMS           *clusterv1.MachineSet
		templateObjects *templateObjects
		wantReq         *runtimehooksv1.CanUpdateMachineSetRequest
	}{
		{
			name:  "Should prepare all objects for diff",
			oldMS: oldMS,
			newMS: newMS,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate:    currentInfraMachineTemplate,
				DesiredInfraMachineTemplate:    desiredInfraMachineTemplate,
				CurrentBootstrapConfigTemplate: currentBootstrapConfigTemplate,
				DesiredBootstrapConfigTemplate: desiredBootstrapConfigTemplate,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				// Objects have been cleaned up for the diff.
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *oldMSCleanedUp,
					InfrastructureMachineTemplate: mustConvertToRawExtension(cleanedUp(currentInfraMachineTemplate)),
					BootstrapConfigTemplate:       mustConvertToRawExtension(cleanedUp(currentBootstrapConfigTemplate)),
				},
				Desired: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *newMSCleanedUp,
					InfrastructureMachineTemplate: mustConvertToRawExtension(cleanedUp(desiredInfraMachineTemplate)),
					BootstrapConfigTemplate:       mustConvertToRawExtension(cleanedUp(desiredBootstrapConfigTemplate)),
				},
			},
		},
		{
			// Note: This case only exists for MachineSets, the BootstrapConfigTemplate is optional (e.g. when using a static bootstrap).
			name:  "Should prepare all objects for diff: BootstrapConfigTemplate is not set",
			oldMS: oldMS,
			newMS: newMS,
			templateObjects: &templateObjects{
				CurrentInfraMachineTemplate: currentInfraMachineTemplate,
				DesiredInfraMachineTemplate: desiredInfraMachineTemplate,
			},
			wantReq: &runtimehooksv1.CanUpdateMachineSetRequest{
				Current: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *oldMSCleanedUp,
					InfrastructureMachineTemplate: mustConvertToRawExtension(cleanedUp(currentInfraMachineTemplate)),
				},
				Desired: runtimehooksv1.CanUpdateMachineSetRequestObjects{
					MachineSet:                    *newMSCleanedUp,
					InfrastructureMachineTemplate: mustConvertToRawExtension(cleanedUp(desiredInfraMachineTemplate)),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Note: The inputs are verified below to ensure createRequest does not mutate them.
			oldMSBefore := tt.oldMS.DeepCopy()
			newMSBefore := tt.newMS.DeepCopy()
			templateObjectsBefore := &templateObjects{
				CurrentInfraMachineTemplate:    tt.templateObjects.CurrentInfraMachineTemplate.DeepCopy(),
				DesiredInfraMachineTemplate:    tt.templateObjects.DesiredInfraMachineTemplate.DeepCopy(),
				CurrentBootstrapConfigTemplate: tt.templateObjects.CurrentBootstrapConfigTemplate.DeepCopy(),
				DesiredBootstrapConfigTemplate: tt.templateObjects.DesiredBootstrapConfigTemplate.DeepCopy(),
			}

			// Note: createRequest only returns an error if the objects cannot be converted to JSON, which cannot happen for valid objects.
			req, err := createRequest(tt.oldMS, tt.newMS, tt.templateObjects)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(req).To(BeComparableTo(tt.wantReq))

			g.Expect(tt.oldMS).To(BeComparableTo(oldMSBefore))
			g.Expect(tt.newMS).To(BeComparableTo(newMSBefore))
			g.Expect(tt.templateObjects).To(BeComparableTo(templateObjectsBefore))
		})
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
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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
				CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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
				CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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
				CommonRetryResponse:                runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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
				CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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
				CommonRetryResponse: runtimehooksv1.CommonRetryResponse{CommonResponse: runtimehooksv1.CommonResponse{Status: runtimehooksv1.ResponseStatusSuccess}},
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

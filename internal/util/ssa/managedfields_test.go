/*
Copyright 2023 The Kubernetes Authors.

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

// Package ssa contains utils related to ssa.
package ssa

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestRemoveManagedFieldsForLabelsAndAnnotations(t *testing.T) {
	testFieldManager := "ssa-manager"
	kubeadmConfig := &bootstrapv1.KubeadmConfig{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "KubeadmConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "kubeadmconfig-1",
			Namespace:  "default",
			Finalizers: []string{"test.com/finalizer"},
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			Format: bootstrapv1.CloudConfig,
		},
	}
	kubeadmConfigWithoutFinalizer := kubeadmConfig.DeepCopy()
	kubeadmConfigWithoutFinalizer.Finalizers = nil
	kubeadmConfigWithLabelsAndAnnotations := kubeadmConfig.DeepCopy()
	kubeadmConfigWithLabelsAndAnnotations.Labels = map[string]string{
		"label-1": "value-1",
	}
	kubeadmConfigWithLabelsAndAnnotations.Annotations = map[string]string{
		"annotation-1": "value-1",
	}

	tests := []struct {
		name                                  string
		kubeadmConfig                         *bootstrapv1.KubeadmConfig
		statusWriteAfterCreate                bool
		expectedManagedFieldsAfterCreate      []managedFieldEntry
		expectedManagedFieldsAfterStatusWrite []managedFieldEntry
		expectedManagedFieldsAfterRemoval     []managedFieldEntry
	}{
		{
			name: "no-op if there are no managedFields for labels and annotations",
			// Note: This case should never happen, but using it to test the no-op code path.
			kubeadmConfig: kubeadmConfig.DeepCopy(),
			// Note: After create testFieldManager should own all fields.
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}},
			// Note: Expect no change.
			expectedManagedFieldsAfterRemoval: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}},
		},
		{
			name: "no-op if there are no managedFields for labels and annotations (not even metadata)",
			// Note: This case should never happen, but using it to test the no-op code path.
			kubeadmConfig: kubeadmConfigWithoutFinalizer.DeepCopy(),
			// Note: After create testFieldManager should own all fields.
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:spec":{
	"f:format":{}
}}`,
			}},
			// Note: Expect no change.
			expectedManagedFieldsAfterRemoval: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:spec":{
	"f:format":{}
}}`,
			}},
		},
		{
			name:          "Remove managedFields for labels and annotations",
			kubeadmConfig: kubeadmConfigWithLabelsAndAnnotations.DeepCopy(),
			// Note: After create testFieldManager should own all fields.
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}},
			// Note: After removal testFieldManager should not own labels and annotations anymore.
			expectedManagedFieldsAfterRemoval: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}},
		},
		{
			name:                   "Remove managedFields for labels and annotations, preserve status fields and retry on conflict",
			kubeadmConfig:          kubeadmConfigWithLabelsAndAnnotations.DeepCopy(),
			statusWriteAfterCreate: true,
			// Note: After create testFieldManager should own all fields.
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}},
			// Note: After status write there is an additional entry for status.
			expectedManagedFieldsAfterStatusWrite: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:status":{
	".":{},
	"f:initialization":{
		".":{},
		"f:dataSecretCreated":{}
	}
}}`,
				Subresource: "status",
			}},
			// Note: After removal testFieldManager should not own labels and annotations anymore
			//       additionally the status entry is preserved and because of the status write
			//       the client will use the retry on conflict code path.
			expectedManagedFieldsAfterRemoval: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	"f:format":{}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:status":{
	".":{},
	"f:initialization":{
		".":{},
		"f:dataSecretCreated":{}
	}
}}`,
				Subresource: "status",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewWithT(t)

			// Note: This func is called exactly the same way as it is called when creating BootstrapConfig/InfraMachine in KCP/MS.
			g.Expect(Patch(ctx, env.Client, testFieldManager, tt.kubeadmConfig)).To(Succeed())
			t.Cleanup(func() {
				ctx := context.Background()
				objWithoutFinalizer := tt.kubeadmConfig.DeepCopyObject().(client.Object)
				objWithoutFinalizer.SetFinalizers([]string{})
				g.Expect(env.Client.Patch(ctx, objWithoutFinalizer, client.MergeFrom(tt.kubeadmConfig))).To(Succeed())
				g.Expect(env.CleanupAndWait(ctx, tt.kubeadmConfig)).To(Succeed())
			})
			g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterCreate)))

			if tt.statusWriteAfterCreate {
				// Ensure we don't update resourceVersion in tt.kubeadmConfig so RemoveManagedFieldsForLabelsAndAnnotations
				// below encounters a conflict and uses the retry on conflict code path.
				kubeadmConfig := tt.kubeadmConfig.DeepCopyObject().(*bootstrapv1.KubeadmConfig)
				origKubeadmConfig := kubeadmConfig.DeepCopyObject().(*bootstrapv1.KubeadmConfig)
				kubeadmConfig.Status.Initialization.DataSecretCreated = ptr.To(true)
				g.Expect(env.Client.Status().Patch(ctx, kubeadmConfig, client.MergeFrom(origKubeadmConfig), client.FieldOwner(classicManager))).To(Succeed())
				g.Expect(cleanupTime(kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterStatusWrite)))
			}

			// Note: This func is called exactly the same way as it is called when creating BootstrapConfig/InfraMachine in KCP/MS.
			g.Expect(RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), tt.kubeadmConfig, testFieldManager)).To(Succeed())
			g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterRemoval)))
		})
	}
}

func TestMigrateManagedFields(t *testing.T) {
	testFieldManager := "ssa-manager"
	testMetadataFieldManager := "ssa-manager-metadata"
	kubeadmConfig := &bootstrapv1.KubeadmConfig{
		// Have to set TypeMeta explicitly when using SSA with typed objects.
		TypeMeta: metav1.TypeMeta{
			APIVersion: bootstrapv1.GroupVersion.String(),
			Kind:       "KubeadmConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "kubeadmconfig-1",
			Namespace:  "default",
			Finalizers: []string{"test.com/finalizer"},
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			Format: bootstrapv1.CloudConfig,
		},
	}

	tests := []struct {
		name                                                   string
		kubeadmConfig                                          *bootstrapv1.KubeadmConfig
		labels                                                 map[string]string
		annotations                                            map[string]string
		statusWriteAfterCreate                                 bool
		updateLabelsAndAnnotationsAfterMigration               bool
		expectedManagedFieldsAfterCreate                       []managedFieldEntry
		expectedManagedFieldsAfterStatusWrite                  []managedFieldEntry
		expectedManagedFieldsAfterMigration                    []managedFieldEntry
		expectedManagedFieldsAfterUpdatingLabelsAndAnnotations []managedFieldEntry
	}{
		{
			name: "no-op if there are no managedFields for the ClusterNameLabel",
			// Note: This case should never happen, but using it to test the ClusterNameLabel check.
			kubeadmConfig: kubeadmConfig.DeepCopy(),
			labels: map[string]string{
				"label-1": "value-1",
			},
			annotations: map[string]string{
				"annotation-1": "value-1",
			},
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	".":{},
	"f:format":{}
}}`,
			}},
			// Note: Expect no change.
			expectedManagedFieldsAfterMigration: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:label-1":{}
	}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	".":{},
	"f:format":{}
}}`,
			}},
		},
		{
			name:          "Remove managedFields if there are managedFields for the ClusterNameLabel and preserve status",
			kubeadmConfig: kubeadmConfig.DeepCopy(),
			labels: map[string]string{
				"label-1":                  "value-1",
				clusterv1.ClusterNameLabel: "cluster-1",
			},
			annotations: map[string]string{
				"annotation-1": "value-1",
			},
			statusWriteAfterCreate:                   true,
			updateLabelsAndAnnotationsAfterMigration: true,
			// Note: After create testFieldManager should own labels and annotations and
			//       manager should own everything else (same as in CAPI <= v1.11).
			expectedManagedFieldsAfterCreate: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:label-1":{}
	}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	".":{},
	"f:format":{}
}}`,
			}},
			// Note: After status write there is an additional entry for status.
			expectedManagedFieldsAfterStatusWrite: []managedFieldEntry{{
				Manager:   testFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:label-1":{}
	}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:metadata":{
	"f:finalizers":{
		".":{},
		"v:\"test.com/finalizer\"":{}
	}
},
"f:spec":{
	".":{},
	"f:format":{}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:status":{
	".":{},
	"f:initialization":{
		".":{},
		"f:dataSecretCreated":{}
	}
}}`,
				Subresource: "status",
			}},
			// Note: After migration the testMetadataFieldManager should have a seed entry, spec and annotation
			//      fields are orphaned and the status entry should be preserved
			expectedManagedFieldsAfterMigration: []managedFieldEntry{{
				Manager:   testMetadataFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:name":{}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:status":{
	".":{},
	"f:initialization":{
		".":{},
		"f:dataSecretCreated":{}
	}
}}`,
				Subresource: "status",
			}},
			// Note: After updating labels and annotations testMetadataFieldManager has a new entry for
			//      them (replacing the seed entry).
			expectedManagedFieldsAfterUpdatingLabelsAndAnnotations: []managedFieldEntry{{
				Manager:   testMetadataFieldManager,
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1: `{
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{}
	},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:label-1":{}
	}
}}`,
			}, {
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: `{
"f:status":{
	".":{},
	"f:initialization":{
		".":{},
		"f:dataSecretCreated":{}
	}
}}`,
				Subresource: "status",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewWithT(t)

			// Note: Create the object like it was created it in CAPI <= v1.11 (with manager).
			g.Expect(env.Client.Create(ctx, tt.kubeadmConfig, client.FieldOwner(classicManager))).To(Succeed())
			t.Cleanup(func() {
				ctx := context.Background()
				objWithoutFinalizer := tt.kubeadmConfig.DeepCopyObject().(client.Object)
				objWithoutFinalizer.SetFinalizers([]string{})
				g.Expect(env.Client.Patch(ctx, objWithoutFinalizer, client.MergeFrom(tt.kubeadmConfig))).To(Succeed())
				g.Expect(env.CleanupAndWait(ctx, tt.kubeadmConfig)).To(Succeed())
			})

			// Note: Update labels and annotations like in CAPI <= v1.11 (with the "main" fieldManager).
			updatedObject := &unstructured.Unstructured{}
			updatedObject.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig"))
			updatedObject.SetNamespace(tt.kubeadmConfig.GetNamespace())
			updatedObject.SetName(tt.kubeadmConfig.GetName())
			updatedObject.SetUID(tt.kubeadmConfig.GetUID())
			updatedObject.SetLabels(tt.labels)
			updatedObject.SetAnnotations(tt.annotations)
			g.Expect(Patch(ctx, env.Client, testFieldManager, updatedObject)).To(Succeed())
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.kubeadmConfig), tt.kubeadmConfig)).To(Succeed())
			g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterCreate)))

			if tt.statusWriteAfterCreate {
				origKubeadmConfig := tt.kubeadmConfig.DeepCopyObject().(*bootstrapv1.KubeadmConfig)
				tt.kubeadmConfig.Status.Initialization.DataSecretCreated = ptr.To(true)
				g.Expect(env.Client.Status().Patch(ctx, tt.kubeadmConfig, client.MergeFrom(origKubeadmConfig), client.FieldOwner(classicManager))).To(Succeed())
				g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterStatusWrite)))
			}

			// Note: This func is called exactly the same way as it is called in syncMachines in KCP/MS.
			g.Expect(MigrateManagedFields(ctx, env.Client, tt.kubeadmConfig, testFieldManager, testMetadataFieldManager)).To(Succeed())
			g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterMigration)))

			if tt.updateLabelsAndAnnotationsAfterMigration {
				// Note: Update labels and annotations exactly the same way as it is done in syncMachines in KCP/MS.
				updatedObject := &unstructured.Unstructured{}
				updatedObject.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig"))
				updatedObject.SetNamespace(tt.kubeadmConfig.GetNamespace())
				updatedObject.SetName(tt.kubeadmConfig.GetName())
				updatedObject.SetUID(tt.kubeadmConfig.GetUID())
				updatedObject.SetLabels(tt.labels)
				updatedObject.SetAnnotations(tt.annotations)
				g.Expect(Patch(ctx, env.Client, testMetadataFieldManager, updatedObject)).To(Succeed())
				g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.kubeadmConfig), tt.kubeadmConfig)).To(Succeed())
				g.Expect(cleanupTime(tt.kubeadmConfig.GetManagedFields())).To(BeComparableTo(toManagedFields(tt.expectedManagedFieldsAfterUpdatingLabelsAndAnnotations)))
			}
		})
	}
}

func cleanupTime(fields []metav1.ManagedFieldsEntry) []metav1.ManagedFieldsEntry {
	for i := range fields {
		fields[i].Time = nil
	}
	return fields
}

type managedFieldEntry struct {
	Manager     string
	Operation   metav1.ManagedFieldsOperationType
	FieldsV1    string
	Subresource string
}

func toManagedFields(managedFields []managedFieldEntry) []metav1.ManagedFieldsEntry {
	res := []metav1.ManagedFieldsEntry{}
	for _, f := range managedFields {
		res = append(res, metav1.ManagedFieldsEntry{
			Manager:     f.Manager,
			Operation:   f.Operation,
			APIVersion:  bootstrapv1.GroupVersion.String(),
			FieldsType:  "FieldsV1",
			FieldsV1:    &metav1.FieldsV1{Raw: []byte(trimSpaces(f.FieldsV1))},
			Subresource: f.Subresource,
		})
	}
	return res
}

func trimSpaces(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}

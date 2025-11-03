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
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func Test_triggerInPlaceUpdate(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "in-place-trigger")
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
	desiredMachine := currentMachine.DeepCopy()
	desiredMachine.Spec.Version = "v1.31.0"

	currentKubeadmConfig := &bootstrapv1.KubeadmConfig{
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
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: bootstrapv1.ClusterConfiguration{
				Etcd: bootstrapv1.Etcd{
					Local: bootstrapv1.LocalEtcd{
						ImageTag: "3.5.0-0",
					},
				},
			},
			JoinConfiguration: bootstrapv1.JoinConfiguration{
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
	currentKubeadmConfigWithInitConfiguration := currentKubeadmConfig.DeepCopy()
	currentKubeadmConfigWithInitConfiguration.Spec.InitConfiguration.NodeRegistration = currentKubeadmConfigWithInitConfiguration.Spec.JoinConfiguration.NodeRegistration
	currentKubeadmConfigWithInitConfiguration.Spec.JoinConfiguration = bootstrapv1.JoinConfiguration{}
	desiredKubeadmConfig := currentKubeadmConfig.DeepCopy()
	desiredKubeadmConfig.Spec.ClusterConfiguration.Etcd.Local.ImageTag = "3.6.4-0"

	currentInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"kind":       builder.TestInfrastructureMachineKind,
			"metadata": map[string]interface{}{
				"name":      "machine-to-in-place-update",
				"namespace": ns.Name,
				"labels": map[string]interface{}{
					clusterv1.ClusterNameLabel: "cluster-1",
					"label-1":                  "label-value-1",
				},
				"annotations": map[string]interface{}{
					"annotation-1": "annotation-value-1",
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
	desiredInfraMachine := currentInfraMachine.DeepCopy()
	g.Expect(unstructured.SetNestedField(desiredInfraMachine.Object, "hello in-place updated world", "spec", "foo")).To(Succeed())
	desiredInfraMachine.SetAnnotations(map[string]string{
		clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template-2",
		clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate2.infrastructure.cluster.x-k8s.io",
	})

	tests := []struct {
		name                                        string
		currentMachine                              *clusterv1.Machine
		createMachineWithUpdateInProgressAnnotation bool
		currentInfraMachine                         *unstructured.Unstructured
		currentKubeadmConfig                        *bootstrapv1.KubeadmConfig
		createKubeadmConfigLikeWithCAPI1_11         bool
		desiredMachine                              *clusterv1.Machine
		desiredInfraMachine                         *unstructured.Unstructured
		desiredKubeadmConfig                        *bootstrapv1.KubeadmConfig
		wantError                                   bool
		wantErrorMessage                            string
	}{
		{
			name:                 "Return error if desiredInfraMachine is nil",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  nil,
			desiredKubeadmConfig: desiredKubeadmConfig,
			wantError:            true,
			wantErrorMessage:     fmt.Sprintf("failed to complete triggering in-place update for Machine %s/machine-to-in-place-update, could not compute desired InfraMachine", ns.Name),
		},
		{
			name:                 "Return error if desiredKubeadmConfig is nil",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: nil,
			wantError:            true,
			wantErrorMessage:     fmt.Sprintf("failed to complete triggering in-place update for Machine %s/machine-to-in-place-update, could not compute desired KubeadmConfig", ns.Name),
		},
		{
			name:                 "Trigger in-place update",
			currentMachine:       currentMachine,
			currentInfraMachine:  currentInfraMachine,
			currentKubeadmConfig: currentKubeadmConfig,
			desiredMachine:       desiredMachine,
			desiredInfraMachine:  desiredInfraMachine,
			desiredKubeadmConfig: desiredKubeadmConfig,
		},
		{
			name:           "Trigger in-place update (Machine already has UpdateInProgressAnnotation)",
			currentMachine: currentMachine,
			createMachineWithUpdateInProgressAnnotation: true,
			currentInfraMachine:                         currentInfraMachine,
			currentKubeadmConfig:                        currentKubeadmConfig,
			desiredMachine:                              desiredMachine,
			desiredInfraMachine:                         desiredInfraMachine,
			desiredKubeadmConfig:                        desiredKubeadmConfig,
		},
		{
			name:                                "Trigger in-place update: KubeadmConfig v1.11 => remove initConfiguration",
			currentMachine:                      currentMachine,
			currentInfraMachine:                 currentInfraMachine,
			currentKubeadmConfig:                currentKubeadmConfigWithInitConfiguration,
			createKubeadmConfigLikeWithCAPI1_11: true,
			desiredMachine:                      desiredMachine,
			desiredInfraMachine:                 desiredInfraMachine,
			desiredKubeadmConfig:                desiredKubeadmConfig,
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
			if tt.createMachineWithUpdateInProgressAnnotation {
				orig := currentMachineForPatch.DeepCopy()
				currentMachineForPatch.Annotations[clusterv1.UpdateInProgressAnnotation] = ""
				g.Expect(env.Client.Patch(ctx, currentMachineForPatch, client.MergeFrom(orig), client.FieldOwner("manager"))).To(Succeed())
			}

			// Create InfraMachine (same as in createInfraMachine)
			currentInfraMachineForPatch := tt.currentInfraMachine.DeepCopy()
			g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, currentInfraMachineForPatch)).To(Succeed())
			g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), currentInfraMachineForPatch, kcpManagerName)).To(Succeed())
			t.Cleanup(func() {
				g.Expect(env.CleanupAndWait(context.Background(), tt.currentInfraMachine)).To(Succeed())
			})

			// Create KubeadmConfig (same as in createKubeadmConfig)
			currentKubeadmConfigForPatch := tt.currentKubeadmConfig.DeepCopy()
			if tt.createKubeadmConfigLikeWithCAPI1_11 {
				// Note: Create the object like it was created it in CAPI <= v1.11 (with manager).
				g.Expect(env.Client.Create(ctx, currentKubeadmConfigForPatch, client.FieldOwner("manager"))).To(Succeed())
				// Note: Update labels and annotations like in CAPI <= v1.11 (with the "capi-kubeadmcontrolplane" fieldManager).
				updatedObject := &unstructured.Unstructured{}
				updatedObject.SetGroupVersionKind(bootstrapv1.GroupVersion.WithKind("KubeadmConfig"))
				updatedObject.SetNamespace(currentKubeadmConfigForPatch.GetNamespace())
				updatedObject.SetName(currentKubeadmConfigForPatch.GetName())
				updatedObject.SetUID(currentKubeadmConfigForPatch.GetUID())
				updatedObject.SetLabels(currentMachineForPatch.GetLabels())
				updatedObject.SetAnnotations(currentMachineForPatch.GetAnnotations())
				g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, updatedObject)).To(Succeed())
				// Now migrate the managedFields like CAPI >= v1.12 does.
				g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(currentKubeadmConfigForPatch), currentKubeadmConfigForPatch)).Should(Succeed())
				g.Expect(ssa.MigrateManagedFields(ctx, env.Client, currentKubeadmConfigForPatch, kcpManagerName, kcpMetadataManagerName)).To(Succeed())
				// Note: At this point spec is not owned by anyone (orphaned). This requires the code path to remove initConfiguration for CAPI v1.11 objects.
			} else {
				g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, currentKubeadmConfigForPatch)).To(Succeed())
				g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), currentKubeadmConfigForPatch, kcpManagerName)).To(Succeed())
			}
			t.Cleanup(func() {
				g.Expect(env.CleanupAndWait(context.Background(), tt.currentKubeadmConfig)).To(Succeed())
			})

			upToDateResult := internal.UpToDateResult{
				CurrentInfraMachine:  currentInfraMachineForPatch,
				CurrentKubeadmConfig: currentKubeadmConfigForPatch,
				DesiredMachine:       tt.desiredMachine.DeepCopy(),
				DesiredInfraMachine:  tt.desiredInfraMachine.DeepCopy(),
				DesiredKubeadmConfig: tt.desiredKubeadmConfig.DeepCopy(),
			}

			r := KubeadmControlPlaneReconciler{
				Client:   env.Client,
				recorder: record.NewFakeRecorder(32),
			}

			err := r.triggerInPlaceUpdate(ctx, currentMachineForPatch, upToDateResult)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			gotMachine := &clusterv1.Machine{}
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.desiredMachine), gotMachine)).To(Succeed())
			g.Expect(gotMachine.Annotations).To(Equal(map[string]string{
				"annotation-1":                       "annotation-value-1",
				clusterv1.UpdateInProgressAnnotation: "",
				runtimev1.PendingHooksAnnotation:     "UpdateMachine",
			}))
			g.Expect(gotMachine.Spec).To(BeComparableTo(tt.desiredMachine.Spec))

			gotInfraMachine := &unstructured.Unstructured{}
			gotInfraMachine.SetGroupVersionKind(tt.desiredInfraMachine.GroupVersionKind())
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.desiredInfraMachine), gotInfraMachine)).To(Succeed())
			g.Expect(gotInfraMachine.GetAnnotations()).To(Equal(map[string]string{
				"annotation-1": "annotation-value-1",
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-machine-template-2",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "TestInfrastructureMachineTemplate2.infrastructure.cluster.x-k8s.io",
				clusterv1.UpdateInProgressAnnotation:            "",
			}))
			g.Expect(gotInfraMachine.Object["spec"]).To(BeComparableTo(tt.desiredInfraMachine.Object["spec"]))

			gotKubeadmConfig := &bootstrapv1.KubeadmConfig{}
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(tt.desiredKubeadmConfig), gotKubeadmConfig)).To(Succeed())
			g.Expect(gotKubeadmConfig.Annotations).To(Equal(map[string]string{
				"annotation-1":                       "annotation-value-1",
				clusterv1.UpdateInProgressAnnotation: "",
			}))
			g.Expect(gotKubeadmConfig.Spec).To(BeComparableTo(tt.desiredKubeadmConfig.Spec))
		})
	}
}

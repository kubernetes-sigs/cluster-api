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

package upgrade

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	testt1v1beta1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t1/v1beta1"
	testt1webhook "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t1/webhook"
	testt2v1beta1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/v1beta1"
	testt2v1beta2 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/v1beta2"
	testt2webhook "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/webhook"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	omittableFieldsMustBeSet    = true
	omittableFieldsMustNotBeSet = false
	zeroMustBeSet               = true
	zeroMustNotBeSet            = false
	zeroSecondsMustBeSet        = true
	zeroSecondsMustNotBeSet     = false
)

// allObjs can be used to look at all the objects / how they change across generation while debugging.
var allObjs map[string]map[string]map[int64]client.Object

func addToAllObj(cluster, prefix string, obj client.Object) {
	ref := fmt.Sprintf("%s - %s", prefix, klog.KObj(obj))
	if allObjs == nil {
		allObjs = make(map[string]map[string]map[int64]client.Object)
	}
	if _, ok := allObjs[cluster]; !ok {
		allObjs[cluster] = make(map[string]map[int64]client.Object)
	}
	if _, ok := allObjs[cluster][ref]; !ok {
		allObjs[cluster][ref] = make(map[int64]client.Object)
	}
	allObjs[cluster][ref][obj.GetGeneration()] = obj
}

// TestAPIAndWebhookChanges validates effects of API changes and effects of removing the DropDefaulterRemoveUnknownOrOmitableFields option.
// from provider's defaulting webhooks.
// TL;DR changes in API to align to optionalrequired recommendations can be addressed with conversions.
// TL;DR if setting omittable fields in a patch, this should cause a rollout only when rebasing to a ClusterClass which uses provider objects with a new apiVersion.
// Details:
// Case 1: cluster1 created at t1 with v1beta references, rebased at t2 to v1beta2 references.
// * t1: create cluster1 with clusterClasst1 => should be stable.
// * t1 => t2: upgrade CRDs / webhooks.
// * t2: force reconcile cluster1 => should be stable.
// * t2: rebase cluster1 to clusterClasst2 => should roll out and then be stable.
// Case 2: cluster2 created at t2 with v1beta references, rebased at t2 to v1beta2 references.
// * t2: create cluster2 with clusterClasst1 => should be stable.
// * t2: rebase cluster2 to clusterClasst2 => should roll out and then be stable.
func TestAPIAndWebhookChanges(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-drop-defaulter-option")
	g.Expect(err).ToNot(HaveOccurred())

	// T1 - mimic an existing environment with a provider using the DefaulterRemoveUnknownOrOmitableFields option in its own defaulting webhook.

	// setupT1CRDAndWebHooks setups CRD and webhooks at t1.
	// At t1 we have a CRD in v1beta1, and the defaulting webhook is using the DefaulterRemoveUnknownOrOmitableFields option.
	t.Log("setupT1CRDAndWebHooks")
	ct1, t1CheckObj, t1CheckTemplate, t1webhookConfig, t1TemplateWebhookConfig := setupT1CRDAndWebHooks(g, ns)

	// create clusterClasst1 using v1beta1 references
	// Notably, this cluster class implements patches adding omittable fields.
	t.Log("create clusterClasst1")
	clusterClasst1 := createT1ClusterClass(g, ns, ct1)

	// create cluster1 using clusterClasst1
	t.Log("create cluster1 using clusterClasst1")
	cluster1 := createClusterWithClusterClass(g, ns.Name, "cluster1", clusterClasst1)

	// check cluster1 is stable (no infinite reconcile or unexpected template rotation).
	// Also, checks that the omittable fields that are added by ClusterClass patches are dropped by the DefaulterRemoveUnknownOrOmitableFields.
	t.Log("check cluster1 is stable")
	var cluster1Refs map[clusterv1.ContractVersionedObjectReference]int64
	g.Eventually(func() error {
		cluster1Refs, err = getClusterTopologyReferences(cluster1, "v1beta1",
			checkOmittableFromPatchesField(omittableFieldsMustNotBeSet), // The defaulting webhook drops the omittable field when using v1beta1.
			checkPtrTypeToType(),                          // "" should never show up in the yaml ("" is not a valid value).
			checkTypeToPtrType(),                          // zero or false should never show up due to omitempty (mutating webhook is dropping omitempty values).
			checkDurationToPtrInt32(zeroSecondsMustBeSet), // metav1.Duration is marshalled as 0s.
			checkTypeToOmitZeroType(zeroMustBeSet),        // zero value should show up (no omitzero).
		)
		if err != nil {
			return err
		}
		return nil
	}, 5*time.Second).Should(Succeed())
	assertClusterTopologyBecomesStable(g, cluster1Refs, ns.Name, "v1beta1")

	// T2 -  mimic a clusterctl upgrade for a provider dropping DefaulterRemoveUnknownOrOmitableFields from its own defaulting webhook.

	// setupT2CRDAndWebHooks setups CRD and webhooks at t2.
	// At t2 we have a CRD in v1beta1 and v1beta2 with the conversion webhook, and the defaulting webhook without the DefaulterRemoveUnknownOrOmitableFields option.
	t.Log("setupT2CRDAndWebHooks")

	err = env.Delete(ctx, t1TemplateWebhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Delete(ctx, t1webhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	ct2, t2webhookConfig, t2TemplateWebhookConfig := setupT2CRDAndWebHooks(g, ns, t1CheckObj, t1CheckTemplate)

	// create a new CC using v1beta2 types to be used for rebase later.
	t.Log("create clusterClasst2")
	clusterClasst2 := createT2ClusterClass(g, ns, ct2)

	// force reconcile on cluster 1.
	t.Log("force reconcile on cluster 1")
	cluster1New := cluster1.DeepCopy()
	if cluster1New.Annotations == nil {
		cluster1New.Annotations = map[string]string{}
	}
	cluster1New.Annotations["force-reconcile"] = ""

	err = env.Patch(ctx, cluster1New, client.MergeFrom(cluster1))
	g.Expect(err).ToNot(HaveOccurred())

	// check cluster1 (created at t1, with a cluster class using v1beta1 references) is still stable after the clusterctl upgrade.
	t.Log("check cluster1 is still stable")
	assertClusterTopologyBecomesStable(g, cluster1Refs, ns.Name, "v1beta2")

	// rebase cluster1 to a CC created at t2, using v1beta2.
	// Also, checks that the omittable fields that are added by ClusterClass is now present.
	t.Log("rebase cluster1 to clusterClasst2, check is stable and rollout happens")
	cluster1New = cluster1New.DeepCopy()
	cluster1New.Spec.Topology.ClassRef.Name = clusterClasst2.Name

	err = env.Patch(ctx, cluster1New, client.MergeFrom(cluster1))
	g.Expect(err).ToNot(HaveOccurred())

	var cluster1RefsNew map[clusterv1.ContractVersionedObjectReference]int64
	g.Eventually(func() error {
		cluster1RefsNew, err = getClusterTopologyReferences(cluster1, "v1beta2",
			checkOmittableFromPatchesField(omittableFieldsMustBeSet), // The defaulting webhook does not drop the omittable field anymore when using v1beta2.
			checkPtrTypeToType(),                             // "" should never show up in the yaml due to omitempty (we set to "" in conversion, but omitempty drops it from yaml).
			checkTypeToPtrType(),                             // zero or false should never show up (we drop zero or false on conversion, we assume implicitly set)
			checkDurationToPtrInt32(zeroSecondsMustNotBeSet), // 0 should never show up (we drop 0 on conversion, we assume implicitly set)
			checkTypeToOmitZeroType(zeroMustNotBeSet),        // zero value must not show up (omitzero through conversion).
		)
		if err != nil {
			return err
		}
		return nil
	}, 5*time.Second).Should(Succeed())
	assertRollout(g, cluster1, cluster1Refs, cluster1RefsNew) // The omittable field should trigger rollout.
	assertClusterTopologyBecomesStable(g, cluster1RefsNew, ns.Name, "v1beta2")

	// create cluster2 using clusterClasst1
	// Also, checks that the omittable fields that are added by ClusterClass patches are dropped by the conversion webhook.
	t.Log("create cluster2 using clusterClasst1")
	cluster2 := createClusterWithClusterClass(g, ns.Name, "cluster2", clusterClasst1)

	var cluster2Refs map[clusterv1.ContractVersionedObjectReference]int64
	g.Eventually(func() error {
		cluster2Refs, err = getClusterTopologyReferences(cluster2, "v1beta2",
			checkOmittableFromPatchesField(omittableFieldsMustNotBeSet), // The conversion webhook drops the omittable field when using v1beta1.
			checkPtrTypeToType(),                             // "" should never show up in the yaml ("" is not a valid value).
			checkTypeToPtrType(),                             // zero or false should never show up (we drop zero or false on conversion, we assume implicitly set)
			checkDurationToPtrInt32(zeroSecondsMustNotBeSet), // 0 should never show up (we drop 0 on conversion, we assume implicitly set)
			checkTypeToOmitZeroType(zeroMustNotBeSet),        // zero value must not show up (omitzero through conversion, also not a valid value).
		)
		if err != nil {
			return err
		}
		return nil
	}, 5*time.Second).Should(Succeed())

	t.Log("check cluster2 is stable")
	assertClusterTopologyBecomesStable(g, cluster2Refs, ns.Name, "v1beta2")

	// rebase cluster2 to a CC created at t2, using v1beta2.
	// Also, checks that the omittable fields that are added by ClusterClass is now present.
	t.Log("rebase cluster2 to clusterClasst2, check is stable and rollout happens")
	cluster2New := cluster2.DeepCopy()
	cluster2New.Spec.Topology.ClassRef.Name = clusterClasst2.Name

	err = env.Patch(ctx, cluster2New, client.MergeFrom(cluster2))
	g.Expect(err).ToNot(HaveOccurred())

	var cluster2RefsNew map[clusterv1.ContractVersionedObjectReference]int64
	g.Eventually(func() error {
		cluster2RefsNew, err = getClusterTopologyReferences(cluster2, "v1beta2",
			checkOmittableFromPatchesField(omittableFieldsMustBeSet), // The defaulting webhook do not drop anymore the omittable field when using v1beta2.
			checkPtrTypeToType(),                             // "" should never show up in the yaml due to omitempty (also not a valid value).
			checkTypeToPtrType(),                             // zero or false should never show up (it will show up if someone explicitly set zero or false)
			checkDurationToPtrInt32(zeroSecondsMustNotBeSet), // 0 should never show up (it will show up if someone explicitly sets 0)
			checkTypeToOmitZeroType(zeroMustNotBeSet),        // zero value must not show up (omitzero, also not a valid value).
		)
		if err != nil {
			return err
		}
		return nil
	}, 5*time.Second).Should(Succeed())
	assertRollout(g, cluster2, cluster2Refs, cluster2RefsNew) // The omittable field should trigger rollout.
	assertClusterTopologyBecomesStable(g, cluster2RefsNew, ns.Name, "v1beta2")

	// Cleanup
	err = env.Delete(ctx, t2TemplateWebhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Delete(ctx, t2webhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(env.Delete(ctx, ns)).To(Succeed())
}

func createClusterWithClusterClass(g *WithT, namespace, name string, clusterClass *clusterv1.ClusterClass) *clusterv1.Cluster {
	machineDeploymentTopology1 := builder.MachineDeploymentTopology("md1").
		WithClass("md-class1").
		WithReplicas(3).
		Build()

	cluster := builder.Cluster(namespace, name).
		WithTopology(
			builder.ClusterTopology().
				WithClass(clusterClass.Name).
				WithMachineDeployment(machineDeploymentTopology1).
				WithVersion("1.32.2").
				WithControlPlaneReplicas(1).
				Build()).
		Build()

	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())
	addToAllObj(cluster.Name, "cluster", cluster)
	return cluster
}

// createT1ClusterClass creates a CC with v1beta1 references and a patch adding the omittable field.
func createT1ClusterClass(g *WithT, ns *corev1.Namespace, ct1 client.Client) *clusterv1.ClusterClass {
	infrastructureClusterTemplate1 := &testt1v1beta1.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-infra-cluster-template-v1beta1-t1",
		},
		Spec: testt1v1beta1.TestResourceTemplateSpec{
			Template: testt1v1beta1.TestResourceTemplateResource{
				Spec: testt1v1beta1.TestResourceSpec{
					BoolToPtrBool:      true,
					PtrStringToString:  ptr.To("Something"),
					Int32ToPtrInt32:    int32(4),
					DurationToPtrInt32: metav1.Duration{Duration: 5 * time.Second},
					// Note: BoolRemoved tests if the removal of KubeadmConfig UseExperimentalRetryJoin triggers rollouts.
					BoolRemoved: true,
					StructWithOnlyOptionalFields: testt1v1beta1.StructWithOnlyOptionalFields{
						A: "Something",
					},
				},
			},
		},
	}
	g.Expect(ct1.Create(ctx, infrastructureClusterTemplate1)).To(Succeed())

	controlPlaneTemplate := &testt1v1beta1.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-control-plane-template-v1beta1-t1",
		},
		Spec: testt1v1beta1.TestResourceTemplateSpec{
			Template: testt1v1beta1.TestResourceTemplateResource{
				Spec: testt1v1beta1.TestResourceSpec{
					BoolToPtrBool:                false,
					PtrStringToString:            nil,
					Int32ToPtrInt32:              0,
					DurationToPtrInt32:           metav1.Duration{},
					StructWithOnlyOptionalFields: testt1v1beta1.StructWithOnlyOptionalFields{},
				},
			},
		},
	}

	g.Expect(ct1.Create(ctx, controlPlaneTemplate)).To(Succeed())

	infrastructureMachineTemplate1 := &testt1v1beta1.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-infra-machine-template-v1beta1-t1",
		},
	}

	g.Expect(ct1.Create(ctx, infrastructureMachineTemplate1)).To(Succeed())

	bootstrapTemplate := &testt1v1beta1.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-bootstrap-config-template-v1beta1-t1",
		},
	}

	g.Expect(ct1.Create(ctx, bootstrapTemplate)).To(Succeed())

	// Waits for the env client to get in cache object we created with client ct1.
	waitForObjects(g, ct1.Scheme(), infrastructureClusterTemplate1, controlPlaneTemplate, infrastructureMachineTemplate1, bootstrapTemplate)

	machineDeploymentClass1 := clusterv1.MachineDeploymentClass{
		Class: "md-class1",
		Infrastructure: clusterv1.MachineDeploymentClassInfrastructureTemplate{
			TemplateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       "TestResourceTemplate",
				Name:       infrastructureMachineTemplate1.Name,
				APIVersion: testt1v1beta1.GroupVersion.String(),
			},
		},
		Bootstrap: clusterv1.MachineDeploymentClassBootstrapTemplate{
			TemplateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       "TestResourceTemplate",
				Name:       bootstrapTemplate.Name,
				APIVersion: testt1v1beta1.GroupVersion.String(),
			},
		},
	}

	clusterClass := &clusterv1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-class-t1",
			Namespace: ns.Name,
		},
		Spec: clusterv1.ClusterClassSpec{
			Infrastructure: clusterv1.InfrastructureClass{
				TemplateRef: clusterv1.ClusterClassTemplateReference{
					Kind:       "TestResourceTemplate",
					Name:       infrastructureClusterTemplate1.Name,
					APIVersion: testt1v1beta1.GroupVersion.String(),
				},
			},
			ControlPlane: clusterv1.ControlPlaneClass{
				TemplateRef: clusterv1.ClusterClassTemplateReference{
					Kind:       "TestResourceTemplate",
					Name:       controlPlaneTemplate.Name,
					APIVersion: testt1v1beta1.GroupVersion.String(),
				},
				InfrastructureMachine: &clusterv1.ControlPlaneClassInfrastructureMachineTemplate{
					TemplateRef: clusterv1.ClusterClassTemplateReference{
						Kind:       "TestResourceTemplate",
						Name:       infrastructureMachineTemplate1.Name,
						APIVersion: testt1v1beta1.GroupVersion.String(),
					},
				},
			},
			Workers: clusterv1.WorkersClass{
				MachineDeployments: []clusterv1.MachineDeploymentClass{
					machineDeploymentClass1,
				},
			},
			Patches: []clusterv1.ClusterClassPatch{
				{
					// Add the omittable fields.
					// Note:
					// - At t1, omittable fields are then dropped by DefaulterRemoveUnknownOrOmitableFields options in the defaulter webhook
					// - At t2, omittable fields are then dropped by the conversion webhook
					Name: "patch-t1-omittable",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: testt1v1beta1.GroupVersion.String(),
								Kind:       "TestResourceTemplate",
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: ptr.To(true),
									ControlPlane:          ptr.To(true),
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{machineDeploymentClass1.Class},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/omittable",
									Value: &apiextensionsv1.JSON{Raw: []byte(`""`)},
								},
							},
						},
					},
				},
				{
					// Add an empty struct.
					// Note:
					// - At t1, empty struct fields are rendered
					// - At t2, empty struct fields are then dropped by the conversion webhook because of omitzero
					Name: "patch-t1-structWithOnlyOptionalFields",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: testt1v1beta1.GroupVersion.String(),
								Kind:       "TestResourceTemplate",
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: ptr.To(false),
									ControlPlane:          ptr.To(false),
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{machineDeploymentClass1.Class},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/structWithOnlyOptionalFields",
									Value: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
								},
							},
						},
					},
				},
			},
		},
	}

	g.Expect(env.CreateAndWait(ctx, clusterClass)).To(Succeed())
	return clusterClass
}

func waitForObjects(g *WithT, scheme *runtime.Scheme, objs ...client.Object) {
	for _, obj := range objs {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		g.Expect(err).ToNot(HaveOccurred())
		g.Eventually(func() error {
			objUnstructured := &unstructured.Unstructured{}
			objUnstructured.SetGroupVersionKind(gvk)
			return env.Get(ctx, client.ObjectKeyFromObject(obj), objUnstructured)
		}, 5*time.Second).Should(Succeed())
	}
}

// createT2ClusterClass creates a CC with v1beta2 references and a patch adding the omittable field.
func createT2ClusterClass(g *WithT, ns *corev1.Namespace, ct2 client.Client) *clusterv1.ClusterClass {
	infrastructureClusterTemplate1 := &testt2v1beta2.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-infra-cluster-template-v1beta2-t2",
		},
		Spec: testt2v1beta2.TestResourceTemplateSpec{
			Template: testt2v1beta2.TestResourceTemplateResource{
				Spec: testt2v1beta2.TestResourceSpec{
					BoolToPtrBool:      ptr.To(true),
					PtrStringToString:  "Something",
					Int32ToPtrInt32:    ptr.To[int32](4),
					DurationToPtrInt32: ptr.To[int32](5),
					StructWithOnlyOptionalFields: testt2v1beta2.StructWithOnlyOptionalFields{
						A: "Something",
					},
				},
			},
		},
	}

	g.Expect(ct2.Create(ctx, infrastructureClusterTemplate1)).To(Succeed())

	controlPlaneTemplate := &testt2v1beta2.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-control-plane-template-v1beta2-t2",
		},
		Spec: testt2v1beta2.TestResourceTemplateSpec{
			Template: testt2v1beta2.TestResourceTemplateResource{
				Spec: testt2v1beta2.TestResourceSpec{
					BoolToPtrBool:                nil,
					Int32ToPtrInt32:              nil,
					DurationToPtrInt32:           nil,
					StructWithOnlyOptionalFields: testt2v1beta2.StructWithOnlyOptionalFields{},
				},
			},
		},
	}

	g.Expect(ct2.Create(ctx, controlPlaneTemplate)).To(Succeed())

	infrastructureMachineTemplate1 := &testt2v1beta2.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-infra-machine-template-v1beta2-t2",
		},
	}

	g.Expect(ct2.Create(ctx, infrastructureMachineTemplate1)).To(Succeed())

	bootstrapTemplate := &testt2v1beta2.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-bootstrap-config-template-v1beta2-t2",
		},
	}

	g.Expect(ct2.Create(ctx, bootstrapTemplate)).To(Succeed())

	// Waits for the env client to get in cache object we created with client ct2.
	waitForObjects(g, ct2.Scheme(), infrastructureClusterTemplate1, controlPlaneTemplate, infrastructureMachineTemplate1, bootstrapTemplate)

	machineDeploymentClass1 := clusterv1.MachineDeploymentClass{
		Class: "md-class1",
		Infrastructure: clusterv1.MachineDeploymentClassInfrastructureTemplate{
			TemplateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       "TestResourceTemplate",
				Name:       infrastructureMachineTemplate1.Name,
				APIVersion: testt2v1beta2.GroupVersion.String(),
			},
		},
		Bootstrap: clusterv1.MachineDeploymentClassBootstrapTemplate{
			TemplateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       "TestResourceTemplate",
				Name:       bootstrapTemplate.Name,
				APIVersion: testt2v1beta2.GroupVersion.String(),
			},
		},
	}

	clusterClass := &clusterv1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-class-t2",
			Namespace: ns.Name,
		},
		Spec: clusterv1.ClusterClassSpec{
			Infrastructure: clusterv1.InfrastructureClass{
				TemplateRef: clusterv1.ClusterClassTemplateReference{
					Kind:       "TestResourceTemplate",
					Name:       infrastructureClusterTemplate1.Name,
					APIVersion: testt2v1beta2.GroupVersion.String(),
				},
			},
			ControlPlane: clusterv1.ControlPlaneClass{
				TemplateRef: clusterv1.ClusterClassTemplateReference{
					Kind:       "TestResourceTemplate",
					Name:       controlPlaneTemplate.Name,
					APIVersion: testt2v1beta2.GroupVersion.String(),
				},
				InfrastructureMachine: &clusterv1.ControlPlaneClassInfrastructureMachineTemplate{
					TemplateRef: clusterv1.ClusterClassTemplateReference{
						Kind:       "TestResourceTemplate",
						Name:       infrastructureMachineTemplate1.Name,
						APIVersion: testt2v1beta2.GroupVersion.String(),
					},
				},
			},
			Workers: clusterv1.WorkersClass{
				MachineDeployments: []clusterv1.MachineDeploymentClass{
					machineDeploymentClass1,
				},
			},
			Patches: []clusterv1.ClusterClassPatch{
				{
					// Add the omittable fields.
					// Note: the omittable fields are not dropped because the DefaulterRemoveUnknownOrOmitableFields options is not used anymore.
					Name: "patch-t2",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: testt2v1beta2.GroupVersion.String(),
								Kind:       "TestResourceTemplate",
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: ptr.To(true),
									ControlPlane:          ptr.To(true),
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{machineDeploymentClass1.Class},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/omittable",
									Value: &apiextensionsv1.JSON{Raw: []byte(`""`)},
								},
							},
						},
					},
				},
				// It is not possible to add patch empty struct due to minProperties.
			},
		},
	}

	g.Expect(env.CreateAndWait(ctx, clusterClass)).To(Succeed())
	return clusterClass
}

// getClusterTopologyReferences gets all the references for a test cluster + the corresponding generation.
// NOTE: it also stores all the objects / how they change across generation into allObjs for debugging.
func getClusterTopologyReferences(cluster *clusterv1.Cluster, version string, additionalChecks ...func(*unstructured.Unstructured) error) (map[clusterv1.ContractVersionedObjectReference]int64, error) {
	refs := map[clusterv1.ContractVersionedObjectReference]int64{}
	// Get all the references

	actualCluster := &clusterv1.Cluster{}
	if err := env.Get(ctx, client.ObjectKeyFromObject(cluster), actualCluster); err != nil {
		return nil, errors.Wrapf(err, "failed to get cluster %s", cluster.Name)
	}
	if c := conditions.Get(actualCluster, clusterv1.ClusterTopologyReconciledCondition); c == nil || c.Status != metav1.ConditionTrue || c.ObservedGeneration != actualCluster.Generation {
		return nil, errors.Errorf("cluster %s topology is not reconciled", cluster.Name)
	}

	addToAllObj(cluster.Name, "cluster", actualCluster)
	refs[clusterv1.ContractVersionedObjectReference{
		APIGroup: clusterv1.GroupVersion.Group,
		Kind:     "Cluster",
		Name:     actualCluster.GetName(),
	}] = actualCluster.GetGeneration()

	if actualCluster.Spec.InfrastructureRef == nil {
		return nil, errors.New("cluster actualCluster.spec.infrastructureRef is not yet set")
	}
	refObj, err := getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.InfrastructureRef, version, actualCluster.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get referenced actualCluster.spec.infrastructureRef")
	}
	addToAllObj(cluster.Name, ".spec.infrastructureRef", refObj)
	refs[clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}] = refObj.GetGeneration()
	for _, check := range additionalChecks {
		if err := check(refObj); err != nil {
			return nil, errors.Wrap(err, "failed additional checks on actualCluster.spec.infrastructureRef")
		}
	}

	if actualCluster.Spec.ControlPlaneRef == nil {
		return nil, errors.New("cluster actualCluster.spec.controlPlaneRef is not yet set")
	}
	refObj, err = getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.ControlPlaneRef, version, actualCluster.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get referenced actualCluster.spec.controlPlaneRef")
	}
	addToAllObj(cluster.Name, ".spec.controlPlaneRef", refObj)
	refs[clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}] = refObj.GetGeneration()
	for _, check := range additionalChecks {
		if err := check(refObj); err != nil {
			return nil, errors.Wrap(err, "failed additional checks on actualCluster.spec.controlPlaneRef")
		}
	}

	cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(refObj)
	if err != nil {
		return nil, errors.Wrap(err, "cluster controlPlane.spec.machineTemplate.spec.infrastructureRef is not yet set")
	}
	refObj, err = getReferencedObject(ctx, env.GetClient(), cpInfraRef, version, actualCluster.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get referenced controlPlane.spec.machineTemplate.spec.infrastructureRef")
	}
	addToAllObj(cluster.Name, "controlPlane.spec.machineTemplate.spec.infrastructureRef", refObj)
	refs[clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}] = refObj.GetGeneration()
	for _, check := range additionalChecks {
		if err := check(refObj); err != nil {
			return nil, errors.Wrap(err, "failed additional checks on controlPlane.spec.machineTemplate.spec.infrastructureRef")
		}
	}

	machineDeployments := &clusterv1.MachineDeploymentList{}
	if err = env.List(ctx, machineDeployments, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.ClusterNameLabel:          cluster.Name,
		clusterv1.ClusterTopologyOwnedLabel: "",
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list machineDeployments")
	}
	if len(machineDeployments.Items) != 1 {
		return nil, errors.Errorf("expected 1 machineDeployment, got %d", len(machineDeployments.Items))
	}

	for _, md := range machineDeployments.Items {
		refs[clusterv1.ContractVersionedObjectReference{
			APIGroup: clusterv1.GroupVersion.Group,
			Kind:     "MachineDeployment",
			Name:     md.GetName(),
		}] = md.GetGeneration()
		addToAllObj(cluster.Name, "machineDeployment "+md.Name, &md)

		refObj, err = getReferencedObject(ctx, env.GetClient(), &md.Spec.Template.Spec.InfrastructureRef, version, actualCluster.Namespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get referenced machineDeployment.spec.template.spec.infrastructureRef")
		}
		addToAllObj(cluster.Name, "machineDeployment "+md.Name+" .spec.template.spec.infrastructureRef", refObj)
		refs[clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}] = refObj.GetGeneration()
		for _, check := range additionalChecks {
			if err := check(refObj); err != nil {
				return nil, errors.Wrap(err, "failed additional checks on machineDeployment.spec.template.spec.infrastructureRef")
			}
		}

		if md.Spec.Template.Spec.Bootstrap.ConfigRef == nil {
			return nil, errors.New("cluster machineDeployment.spec.template.spec.bootstrap.configRef is not yet set")
		}
		refObj, err = getReferencedObject(ctx, env.GetClient(), md.Spec.Template.Spec.Bootstrap.ConfigRef, version, actualCluster.Namespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get referenced machineDeployment.spec.template.spec.bootstrap.configRef")
		}
		addToAllObj(cluster.Name, "machineDeployment "+md.Name+" .spec.template.spec.bootstrap.configRef", refObj)
		refs[clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}] = refObj.GetGeneration()
		for _, check := range additionalChecks {
			if err := check(refObj); err != nil {
				return nil, errors.Wrap(err, "failed additional checks on machineDeployment.spec.template.spec.bootstrap.configRef")
			}
		}
	}
	return refs, nil
}

func getReferencedObject(ctx context.Context, c client.Client, ref *clusterv1.ContractVersionedObjectReference, version, namespace string) (*unstructured.Unstructured, error) {
	refObj := &unstructured.Unstructured{}
	refObj.SetGroupVersionKind(ref.GroupKind().WithVersion(version))
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: namespace}, refObj); err != nil {
		return nil, err
	}
	return refObj, nil
}

func checkOmittableFromPatchesField(mustBeSet bool) func(obj *unstructured.Unstructured) error {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		case "TestResource":
			_, exists, err := unstructured.NestedString(obj.Object, "spec", "omittable")
			if err != nil {
				return err
			}
			if exists && !mustBeSet {
				return errors.New("expected to not contain omittable field")
			}
			if !exists && mustBeSet {
				return errors.New("expected to contain omittable field")
			}
		case "TestResourceTemplate":
			_, exists, err := unstructured.NestedString(obj.Object, "spec", "template", "spec", "omittable")
			if err != nil {
				return err
			}
			if exists && !mustBeSet {
				return errors.New("expected to not contain omittable field")
			}
			if !exists && mustBeSet {
				return errors.New("expected to contain omittable field")
			}
		}
		return nil
	}
}

func checkPtrTypeToType() func(obj *unstructured.Unstructured) error {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		case "TestResource":
			value, exists, err := unstructured.NestedString(obj.Object, "spec", "ptrStringToString")
			if err != nil {
				return err
			}
			if exists && value == "" {
				return errors.New("expected to not contain an empty ptrStringToString field")
			}
		case "TestResourceTemplate":
			value, exists, err := unstructured.NestedString(obj.Object, "spec", "template", "spec", "ptrStringToString")
			if err != nil {
				return err
			}
			if exists && value == "" {
				return errors.New("expected to not contain an empty ptrStringToString field")
			}
		}
		return nil
	}
}

func checkTypeToPtrType() func(obj *unstructured.Unstructured) error {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		case "TestResource":
			value1, exists, err := unstructured.NestedInt64(obj.Object, "spec", "int32ToPtrInt32")
			if err != nil {
				return err
			}
			if exists && value1 == 0 {
				return errors.New("expected to not contain a zero int32ToPtrInt32 field")
			}
			value2, exists, err := unstructured.NestedBool(obj.Object, "spec", "boolToPtrBool")
			if err != nil {
				return err
			}
			if exists && !value2 {
				return errors.New("expected to not contain a false boolToPtrBool field")
			}
		case "TestResourceTemplate":
			value1, exists, err := unstructured.NestedInt64(obj.Object, "spec", "template", "spec", "int32ToPtrInt32")
			if err != nil {
				return err
			}
			if exists && value1 == 0 {
				return errors.New("expected to not contain an zero int32ToPtrInt32 field")
			}
			value2, exists, err := unstructured.NestedBool(obj.Object, "spec", "template", "spec", "boolToPtrBool")
			if err != nil {
				return err
			}
			if exists && !value2 {
				return errors.New("expected to not contain a false boolToPtrBool field")
			}
		}
		return nil
	}
}

func checkDurationToPtrInt32(mustBeSet bool) func(obj *unstructured.Unstructured) error {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		case "TestResource":
			value, exists, err := unstructured.NestedFieldCopy(obj.Object, "spec", "durationToPtrInt32")
			if err != nil {
				return err
			}
			if !mustBeSet && exists && (reflect.DeepEqual(value, "0s") || reflect.DeepEqual(value, int64(0))) {
				return errors.New("expected to not contain a 0s durationToPtrInt32 field")
			}
			if mustBeSet && !exists {
				return errors.New("expected to contain a durationToPtrInt32 field")
			}
		case "TestResourceTemplate":
			value, exists, err := unstructured.NestedFieldCopy(obj.Object, "spec", "template", "spec", "durationToPtrInt32")
			if err != nil {
				return err
			}
			if !mustBeSet && exists && (reflect.DeepEqual(value, "0s") || reflect.DeepEqual(value, int64(0))) {
				return errors.New("expected to not contain a 0s durationToPtrInt32 field")
			}
			if mustBeSet && !exists {
				return errors.New("expected to contain a durationToPtrInt32 field")
			}
		}
		return nil
	}
}

func checkTypeToOmitZeroType(mustBeSet bool) func(obj *unstructured.Unstructured) error {
	return func(obj *unstructured.Unstructured) error {
		switch obj.GetKind() {
		case "TestResource":
			value, exists, err := unstructured.NestedMap(obj.Object, "spec", "structWithOnlyOptionalFields")
			if err != nil {
				return err
			}
			if exists && reflect.DeepEqual(value, map[string]interface{}{}) && !mustBeSet {
				return errors.New("expected to not contain a zero structWithOnlyOptionalFields field")
			}
			if !exists && mustBeSet {
				return errors.New("expected to contain a zero structWithOnlyOptionalFields field")
			}
		case "TestResourceTemplate":
			value, exists, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec", "structWithOnlyOptionalFields")
			if err != nil {
				return err
			}
			if exists && reflect.DeepEqual(value, map[string]interface{}{}) && !mustBeSet {
				return errors.New("expected to not contain a zero structWithOnlyOptionalFields field")
			}
			if !exists && mustBeSet {
				return errors.New("expected to contain a zero structWithOnlyOptionalFields field")
			}
		}
		return nil
	}
}

// assertRollout assert a rollout happened by checking referenced template are changed or referencedObjects increased generation.
func assertRollout(g *WithT, cluster *clusterv1.Cluster, refsBefore, refsAfter map[clusterv1.ContractVersionedObjectReference]int64) {
	actualCluster := &clusterv1.Cluster{}
	err := env.Get(ctx, client.ObjectKeyFromObject(cluster), actualCluster)
	g.Expect(err).To(Succeed())

	clusterRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: clusterv1.GroupVersion.Group,
		Kind:     "Cluster",
		Name:     actualCluster.GetName(),
	}
	g.Expect(refsAfter[clusterRef]).To(BeNumerically(">", refsBefore[clusterRef]), "cluster unexpected change") // Cluster is expected to have an additional generation due to the rebase

	refObj, err := getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.InfrastructureRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	infraClusterRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsAfter[infraClusterRef]).To(BeNumerically(">", refsBefore[infraClusterRef]), "cluster.spec.infrastructureRef has unexpected generation") // InfrastructureRef is expected to have an additional generation due to the rebase.

	refObj, err = getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.ControlPlaneRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	controlPlaneRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsAfter[controlPlaneRef]).To(BeNumerically(">", refsBefore[controlPlaneRef]), "cluster.spec.controlPlaneRef has unexpected generation") // controlPlaneRef is expected to have an additional generation due to the rebase.

	cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(refObj)
	g.Expect(err).To(Succeed())
	refObj, err = getReferencedObject(ctx, env.GetClient(), cpInfraRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	controlPlaneInfraRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsBefore).ToNot(HaveKey(controlPlaneInfraRef), "controlPlane.spec.machineTemplate.spec.infrastructureRef did not rollout")
	g.Expect(refsAfter[controlPlaneInfraRef]).To(Equal(int64(1)), "controlPlane.spec.machineTemplate.spec.infrastructureRef has unexpected generation")

	machineDeployments := &clusterv1.MachineDeploymentList{}
	err = env.List(ctx, machineDeployments, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.ClusterNameLabel:          cluster.Name,
		clusterv1.ClusterTopologyOwnedLabel: "",
	})
	g.Expect(err).To(Succeed())
	g.Expect(machineDeployments.Items).To(HaveLen(1))

	for _, md := range machineDeployments.Items {
		mdRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: clusterv1.GroupVersion.Group,
			Kind:     "MachineDeployment",
			Name:     md.GetName(),
		}
		g.Expect(refsAfter[mdRef]).To(Equal(refsBefore[mdRef]+1), "machineDeployment "+md.Name+" unexpected change") // MachineDeployment is expected to have an additional generation due to rollout

		refObj, err = getReferencedObject(ctx, env.GetClient(), &md.Spec.Template.Spec.InfrastructureRef, "v1beta2", cluster.Namespace)
		g.Expect(err).To(Succeed())
		mdInfraRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}
		g.Expect(refsBefore).ToNot(HaveKey(mdInfraRef), "machineDeployment "+md.Name+" .spec.template.spec.infrastructureRef did not rollout")
		g.Expect(refsAfter[mdInfraRef]).To(Equal(int64(1)), "machineDeployment "+md.Name+" .spec.template.spec.infrastructureRef has unexpected generation")

		refObj, err = getReferencedObject(ctx, env.GetClient(), md.Spec.Template.Spec.Bootstrap.ConfigRef, "v1beta2", cluster.Namespace)
		g.Expect(err).To(Succeed())
		mdBootstrapRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}
		g.Expect(refsBefore).ToNot(HaveKey(mdBootstrapRef), "machineDeployment "+md.Name+" .spec.template.spec.bootstrap.configRef did not rollout")
		g.Expect(refsAfter[mdBootstrapRef]).To(Equal(int64(1)), "machineDeployment "+md.Name+" .spec.template.spec.bootstrap.configRef has unexpected generation")
	}
}

// assertNoRollout assert a rollout did not happened by checking referenced template or referencedObjects did not changed/increased generation.
//
//nolint:unused
func assertNoRollout(g *WithT, cluster *clusterv1.Cluster, refsBefore, refsAfter map[clusterv1.ContractVersionedObjectReference]int64) {
	actualCluster := &clusterv1.Cluster{}
	err := env.Get(ctx, client.ObjectKeyFromObject(cluster), actualCluster)
	g.Expect(err).To(Succeed())

	clusterRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: clusterv1.GroupVersion.Group,
		Kind:     "Cluster",
		Name:     actualCluster.GetName(),
	}
	g.Expect(refsAfter[clusterRef]).To(Equal(refsBefore[clusterRef]+1), "cluster unexpected change") // Cluster is expected to have an additional generation due to the rebase

	refObj, err := getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.InfrastructureRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	infraClusterRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsAfter[infraClusterRef]).To(Equal(refsBefore[infraClusterRef]), "cluster.spec.infrastructureRef unexpected change")

	refObj, err = getReferencedObject(ctx, env.GetClient(), actualCluster.Spec.ControlPlaneRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	controlPlaneRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsAfter[controlPlaneRef]).To(Equal(refsBefore[controlPlaneRef]), "cluster.spec.controlPlaneRef unexpected change")

	cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(refObj)
	g.Expect(err).To(Succeed())
	refObj, err = getReferencedObject(ctx, env.GetClient(), cpInfraRef, "v1beta2", cluster.Namespace)
	g.Expect(err).To(Succeed())
	controlPlaneInfraRef := clusterv1.ContractVersionedObjectReference{
		APIGroup: refObj.GroupVersionKind().Group,
		Kind:     refObj.GetKind(),
		Name:     refObj.GetName(),
	}
	g.Expect(refsAfter[controlPlaneInfraRef]).To(Equal(refsBefore[controlPlaneInfraRef]), "controlPlane.spec.machineTemplate.spec.infrastructureRef unexpected change triggering a rollout")

	machineDeployments := &clusterv1.MachineDeploymentList{}
	err = env.List(ctx, machineDeployments, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.ClusterNameLabel:          cluster.Name,
		clusterv1.ClusterTopologyOwnedLabel: "",
	})
	g.Expect(err).To(Succeed())
	g.Expect(machineDeployments.Items).To(HaveLen(1))

	for _, md := range machineDeployments.Items {
		mdRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: clusterv1.GroupVersion.Group,
			Kind:     "MachineDeployment",
			Name:     md.GetName(),
		}
		g.Expect(refsAfter[mdRef]).To(Equal(refsBefore[mdRef]), "machineDeployment "+md.Name+" unexpected change")

		refObj, err = getReferencedObject(ctx, env.GetClient(), &md.Spec.Template.Spec.InfrastructureRef, "v1beta2", cluster.Namespace)
		g.Expect(err).To(Succeed())
		mdInfraRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}
		g.Expect(refsAfter[mdInfraRef]).To(Equal(refsBefore[mdInfraRef]), "machineDeployment "+md.Name+" .spec.template.spec.infrastructureRef unexpected change triggering a rollout")

		refObj, err = getReferencedObject(ctx, env.GetClient(), md.Spec.Template.Spec.Bootstrap.ConfigRef, "v1beta2", cluster.Namespace)
		g.Expect(err).To(Succeed())
		mdBootstrapRef := clusterv1.ContractVersionedObjectReference{
			APIGroup: refObj.GroupVersionKind().Group,
			Kind:     refObj.GetKind(),
			Name:     refObj.GetName(),
		}
		g.Expect(refsAfter[mdBootstrapRef]).To(Equal(refsBefore[mdBootstrapRef]), "machineDeployment "+md.Name+" .spec.template.spec.bootstrap.configRef unexpected change triggering a rollout")
	}
}

// assertClusterTopologyBecomesStable checks a cluster topology becomes stable ensuring all the objects included cluster, md and referenced template or referencedObjects do not changed/increased generation.
func assertClusterTopologyBecomesStable(g *WithT, refs map[clusterv1.ContractVersionedObjectReference]int64, namespace, version string) {
	g.Consistently(func(g Gomega) {
		for r, generation := range refs {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(r.GroupKind().WithVersion(version))
			err := env.GetClient().Get(ctx, client.ObjectKey{Name: r.Name, Namespace: namespace}, obj)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(obj.GetGeneration()).To(Equal(generation), "generation is not remaining stable for %s/%s, %s", r.Kind, r.GroupKind().WithVersion(version).GroupVersion().String(), r.Name)
		}
	}, 10*time.Second, 2*time.Second).Should(Succeed(), "Resource versions didn't stay stable")
}

// setupT1CRDAndWebHooks setups CRD and webhooks at t1.
// At t1 we have a CRD in v1beta1, and the defaulting webhook is using the DefaulterRemoveUnknownOrOmitableFields option.
func setupT1CRDAndWebHooks(g *WithT, ns *corev1.Namespace) (client.Client, *testt1v1beta1.TestResource, *testt1v1beta1.TestResourceTemplate, *admissionv1.MutatingWebhookConfiguration, *admissionv1.MutatingWebhookConfiguration) {
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled

	// Create a scheme
	t1Scheme := runtime.NewScheme()
	_ = testt1v1beta1.AddToScheme(t1Scheme)

	// Install CRD
	err := env.ApplyCRDs(ctx, filepath.Join(path.Dir(filename), "test", "t1", "crd"))
	g.Expect(err).ToNot(HaveOccurred())

	// Set contract label on CRDs
	// NOTE: for sake of simplicity we are setting v1beta2 contract for v1beta1 API version.
	err = addOrReplaceContractLabels(ctx, env, "testresourcetemplates.test.cluster.x-k8s.io", testt1v1beta1.GroupVersion.Version)
	g.Expect(err).ToNot(HaveOccurred())

	err = addOrReplaceContractLabels(ctx, env, "testresources.test.cluster.x-k8s.io", testt1v1beta1.GroupVersion.Version)
	g.Expect(err).ToNot(HaveOccurred())

	// Install the defaulter webhook for v1beta1 and test it works
	t1TemplateDefaulter := admission.WithCustomDefaulter(t1Scheme, &testt1v1beta1.TestResourceTemplate{}, &testt1webhook.TestResourceTemplate{}, admission.DefaulterRemoveUnknownOrOmitableFields)
	t1TemplateWebhookConfig, err := newMutatingWebhookConfigurationForCustomDefaulter(ns, testt1v1beta1.GroupVersion.WithResource("testresourcetemplates"), t1TemplateDefaulter)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Create(ctx, t1TemplateWebhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	t1Defaulter := admission.WithCustomDefaulter(t1Scheme, &testt1v1beta1.TestResource{}, &testt1webhook.TestResource{}, admission.DefaulterRemoveUnknownOrOmitableFields)
	t1webhookConfig, err := newMutatingWebhookConfigurationForCustomDefaulter(ns, testt1v1beta1.GroupVersion.WithResource("testresources"), t1Defaulter)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Create(ctx, t1webhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	ct1, err := client.New(env.Config, client.Options{Scheme: t1Scheme})
	g.Expect(err).ToNot(HaveOccurred())

	// Test if webhook is working
	g.Eventually(ctx, func(g Gomega) {
		t1TemplateObj := &testt1v1beta1.TestResourceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-setup-dryrun",
				Namespace: ns.Name,
			},
		}
		g.Expect(ct1.Create(ctx, t1TemplateObj, client.DryRunAll)).To(Succeed())
		g.Expect(t1TemplateObj.Annotations).To(HaveKey("default-t1"))
	}, 5*time.Second).Should(Succeed())

	g.Eventually(ctx, func(g Gomega) {
		t1Obj := &testt1v1beta1.TestResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-setup-dryrun",
				Namespace: ns.Name,
			},
		}
		g.Expect(ct1.Create(ctx, t1Obj, client.DryRunAll)).To(Succeed())
		g.Expect(t1Obj.Annotations).To(HaveKey("default-t1"))
	}, 5*time.Second).Should(Succeed())

	// Create an object
	t1CheckTemplate := &testt1v1beta1.TestResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-setup",
			Namespace: ns.Name,
		},
	}

	err = ct1.Create(ctx, t1CheckTemplate)
	g.Expect(err).ToNot(HaveOccurred())

	t1CheckObj := &testt1v1beta1.TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-setup",
			Namespace: ns.Name,
		},
	}

	err = ct1.Create(ctx, t1CheckObj)
	g.Expect(err).ToNot(HaveOccurred())

	return ct1, t1CheckObj, t1CheckTemplate, t1webhookConfig, t1TemplateWebhookConfig
}

// setupT2CRDAndWebHooks setups CRD and webhooks at t2.
// At t2 we have a CRD in v1beta1 and v1beta2 with the conversion webhook, and the defaulting webhook without the DefaulterRemoveUnknownOrOmitableFields option.
func setupT2CRDAndWebHooks(g *WithT, ns *corev1.Namespace, t1CheckObj *testt1v1beta1.TestResource, t1CheckTemplate *testt1v1beta1.TestResourceTemplate) (client.Client, *admissionv1.MutatingWebhookConfiguration, *admissionv1.MutatingWebhookConfiguration) {
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled

	// Create a scheme
	t2Scheme := runtime.NewScheme()
	_ = testt2v1beta1.AddToScheme(t2Scheme)
	_ = testt2v1beta2.AddToScheme(t2Scheme)

	// Install CRD
	err := env.ApplyCRDs(ctx, filepath.Join(path.Dir(filename), "test", "t2", "crd"))
	g.Expect(err).ToNot(HaveOccurred())

	// Set contract label on CRDs
	// NOTE: for sake of simplicity we are setting v1beta2 contract both for v1beta1 API version and v1beta2 API version.
	err = addOrReplaceContractLabels(ctx, env, "testresourcetemplates.test.cluster.x-k8s.io", testt2v1beta1.GroupVersion.Version+"_"+testt2v1beta2.GroupVersion.Version)
	g.Expect(err).ToNot(HaveOccurred())

	err = addOrReplaceContractLabels(ctx, env, "testresources.test.cluster.x-k8s.io", testt2v1beta1.GroupVersion.Version+"_"+testt2v1beta2.GroupVersion.Version)
	g.Expect(err).ToNot(HaveOccurred())

	// Install the defaulter webhook for v1beta2
	t2TemplateDefaulter := admission.WithCustomDefaulter(t2Scheme, &testt2v1beta2.TestResourceTemplate{}, &testt2webhook.TestResourceTemplate{})
	t2TemplateWebhookConfig, err := newMutatingWebhookConfigurationForCustomDefaulter(ns, testt2v1beta2.GroupVersion.WithResource("testresourcetemplates"), t2TemplateDefaulter)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Create(ctx, t2TemplateWebhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	t2Defaulter := admission.WithCustomDefaulter(t2Scheme, &testt2v1beta2.TestResource{}, &testt2webhook.TestResource{})
	t2webhookConfig, err := newMutatingWebhookConfigurationForCustomDefaulter(ns, testt2v1beta2.GroupVersion.WithResource("testresources"), t2Defaulter)
	g.Expect(err).ToNot(HaveOccurred())

	err = env.Create(ctx, t2webhookConfig)
	g.Expect(err).ToNot(HaveOccurred())

	// Install the conversion webhook
	err = addOrReplaceConversionWebhookHandler(ctx, env.GetClient(), "testresourcetemplates.test.cluster.x-k8s.io", conversion.NewWebhookHandler(t2Scheme))
	g.Expect(err).ToNot(HaveOccurred())

	err = addOrReplaceConversionWebhookHandler(ctx, env.GetClient(), "testresources.test.cluster.x-k8s.io", conversion.NewWebhookHandler(t2Scheme))
	g.Expect(err).ToNot(HaveOccurred())

	ct2, err := client.New(env.Config, client.Options{Scheme: t2Scheme})
	g.Expect(err).ToNot(HaveOccurred())

	// Test the defaulter works
	g.Eventually(ctx, func(g Gomega) {
		t2TemplateObj := &testt2v1beta2.TestResourceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-setup-dryrun",
				Namespace: ns.Name,
			},
		}
		g.Expect(ct2.Create(ctx, t2TemplateObj, client.DryRunAll)).To(Succeed())
		g.Expect(t2TemplateObj.Annotations).ToNot(HaveKey("default-t1"))
		g.Expect(t2TemplateObj.Annotations).To(HaveKey("default-t2"))
		g.Expect(t2TemplateObj.Annotations).ToNot(HaveKey("conversionTo"))
	}, 5*time.Second).Should(Succeed())

	g.Eventually(ctx, func(g Gomega) {
		t2Obj := &testt2v1beta2.TestResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-setup-dryrun",
				Namespace: ns.Name,
			},
		}
		g.Expect(ct2.Create(ctx, t2Obj, client.DryRunAll)).To(Succeed())
		g.Expect(t2Obj.Annotations).ToNot(HaveKey("default-t1"))
		g.Expect(t2Obj.Annotations).To(HaveKey("default-t2"))
		g.Expect(t2Obj.Annotations).ToNot(HaveKey("conversionTo"))
	}, 5*time.Second).Should(Succeed())

	// Test the conversion works
	g.Eventually(ctx, func(g Gomega) {
		t2TemplateObj := &testt2v1beta2.TestResourceTemplate{}
		err = ct2.Get(ctx, client.ObjectKeyFromObject(t1CheckTemplate), t2TemplateObj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(t2TemplateObj.Annotations).To(HaveKey("default-t1"))
		g.Expect(t2TemplateObj.Annotations).ToNot(HaveKey("default-t2"))
		g.Expect(t2TemplateObj.Annotations).To(HaveKey("conversionTo"))
	}, 5*time.Second).Should(Succeed())

	g.Eventually(ctx, func(g Gomega) {
		t2Obj := &testt2v1beta2.TestResource{}
		err = ct2.Get(ctx, client.ObjectKeyFromObject(t1CheckObj), t2Obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(t2Obj.Annotations).To(HaveKey("default-t1"))
		g.Expect(t2Obj.Annotations).ToNot(HaveKey("default-t2"))
		g.Expect(t2Obj.Annotations).To(HaveKey("conversionTo"))
	}, 5*time.Second).Should(Succeed())

	return ct2, t2webhookConfig, t2TemplateWebhookConfig
}

func newMutatingWebhookConfigurationForCustomDefaulter(ns *corev1.Namespace, gvr schema.GroupVersionResource, handler http.Handler) (*admissionv1.MutatingWebhookConfiguration, error) {
	webhookServer := env.GetWebhookServer().(*webhook.DefaultServer)

	// Calculate webhook host and path.
	// Note: This is done the same way as in our envtest package, but the webhook is set up to work only for objects in the test namespace.
	webhookPath := fmt.Sprintf("/%s/%s-%s-defaulting-webhook", ns.Name, gvr.Resource, gvr.Version)
	webhookHost := "127.0.0.1"

	// Register the webhook.
	webhookServer.Register(webhookPath, handler)

	// Calculate the MutatingWebhookConfiguration
	caBundle, err := os.ReadFile(filepath.Join(webhookServer.Options.CertDir, webhookServer.Options.CertName))
	if err != nil {
		return nil, err
	}

	sideEffectNone := admissionv1.SideEffectClassNone
	webhookConfig := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-webhook-config", ns.Name, gvr.Resource),
		},
		Webhooks: []admissionv1.MutatingWebhook{
			{
				Name: fmt.Sprintf("%s.%s.test.cluster.x-k8s.io", ns.Name, gvr.Resource),
				ClientConfig: admissionv1.WebhookClientConfig{
					URL:      ptr.To(fmt.Sprintf("https://%s%s", net.JoinHostPort(webhookHost, strconv.Itoa(webhookServer.Options.Port)), webhookPath)),
					CABundle: caBundle,
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Update,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{gvr.Group},
							APIVersions: []string{gvr.Version},
							Resources:   []string{gvr.Resource},
						},
					},
				},
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						corev1.LabelMetadataName: ns.Name,
					},
				},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffectNone,
			},
		},
	}
	return webhookConfig, nil
}

func addOrReplaceConversionWebhookHandler(ctx context.Context, c client.Client, crdName string, handler http.Handler) error {
	webhookServer := env.GetWebhookServer().(*webhook.DefaultServer)

	// Calculate webhook host and path.
	// Note: This is done the same way as in our envtest package.
	webhookPath := fmt.Sprintf("/%s/conversion", crdName)
	webhookHost := "127.0.0.1"

	// Register the webhook.
	webhookServer.Register(webhookPath, handler)

	// Get the CA bundle generated by test env.
	caBundle, err := os.ReadFile(filepath.Join(webhookServer.Options.CertDir, webhookServer.Options.CertName))
	if err != nil {
		return err
	}

	// Get the CRD
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
		return err
	}

	// Update the CRD with the webhook configuration
	crdNew := crd.DeepCopy()
	crdNew.Spec.Conversion = &apiextensionsv1.CustomResourceConversion{
		Strategy: "Webhook",
		Webhook: &apiextensionsv1.WebhookConversion{
			ClientConfig: &apiextensionsv1.WebhookClientConfig{
				URL:      ptr.To(fmt.Sprintf("https://%s%s", net.JoinHostPort(webhookHost, strconv.Itoa(webhookServer.Options.Port)), webhookPath)),
				CABundle: caBundle,
			},
			ConversionReviewVersions: []string{"v1", "v1beta1"},
		},
	}
	return c.Patch(ctx, crdNew, client.MergeFrom(crd))
}

func addOrReplaceContractLabels(ctx context.Context, c client.Client, crdName string, contractLabelValue string) error {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: crdName},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
		return err
	}

	// Update the CRD with the contract label
	crdNew := crd.DeepCopy()
	if crdNew.Labels == nil {
		crdNew.Labels = map[string]string{}
	}
	crdNew.Labels[clusterv1.GroupVersion.String()] = contractLabelValue
	return c.Patch(ctx, crdNew, client.MergeFrom(crd))
}

/*
Copyright 2026 The Kubernetes Authors.

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

package patches

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/api/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

// TestApply_UnrelatedMetadataPatchDropsTopologyDeclaredLabel reproduces a regression introduced by
// #13924 ("Allow patching of metadata.{labels,annotations} through ClusterClass patches").
//
// Cluster API already has a documented, independent mechanism for declaring labels on generated
// objects: Cluster.spec.topology.controlPlane.metadata.labels / ClusterClass.spec.controlPlane.metadata.labels
// (merged into `controlPlaneLabels` and set on the desired ControlPlane object in
// exp/topology/desiredstate/desired_state.go's computeControlPlane, *before* patches.Engine.Apply runs).
//
// #13924 made patchObject/patchTemplate copy metadata.labels/annotations wholesale from the patched
// template onto the desired object (see internal/util/patch/patch.go CopyFields: dest.metadata.labels
// is fully replaced by src's field, not merged), only re-inserting a small hardcoded allowlist of
// system labels (ClusterNameLabel, ClusterTopologyOwnedLabel, ClusterTopologyMachineDeploymentNameLabel,
// ClusterTopologyMachinePoolNameLabel) via FieldsToPreserve in updateDesiredState.
//
// As a result, any ClusterClass patch that sets *any* key under spec.template.metadata.labels on a
// selected template -- even for a completely unrelated label, using the exact pattern documented in
// docs/book/src/tasks/experimental-features/cluster-class/write-clusterclass.md -- silently wipes out
// every topology/ClusterClass-declared label on that object that isn't in the hardcoded allowlist.
// No error, warning or event is produced; the labels just disappear from the live object next reconcile.
func TestApply_UnrelatedMetadataPatchDropsTopologyDeclaredLabel(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)
	g := NewWithT(t)
	ctx := context.Background()

	blueprint, desired := setupTestObjects()

	// Simulate what computeControlPlane (exp/topology/desiredstate/desired_state.go) does *before*
	// patches.Engine.Apply is called: it sets a topology/ClusterClass-declared custom label on the
	// desired ControlPlane object, in addition to the always-preserved system labels already set by
	// addStandardLabelsAndAnnotations in setupTestObjects().
	g.Expect(unstructured.SetNestedField(desired.ControlPlane.Object.Object, "platform", "metadata", "labels", "team")).To(Succeed())

	// An unrelated ClusterClass patch that only intends to add a cost-allocation label to the
	// ControlPlane, using exactly the pattern documented for the new metadata-patching feature.
	blueprint.ClusterClass.Spec.Patches = []clusterv1.ClusterClassPatch{
		{
			Name: "add-cost-center-label",
			Definitions: []clusterv1.PatchDefinition{
				{
					Selector: clusterv1.PatchSelector{
						APIVersion: builder.ControlPlaneGroupVersion.String(),
						Kind:       builder.GenericControlPlaneTemplateKind,
						MatchResources: clusterv1.PatchSelectorMatch{
							ControlPlane: ptr.To(true),
						},
					},
					JSONPatches: []clusterv1.JSONPatch{
						{
							Op:    "add",
							Path:  "/spec/template/metadata",
							Value: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
						},
						{
							Op:    "add",
							Path:  "/spec/template/metadata/labels",
							Value: &apiextensionsv1.JSON{Raw: []byte(`{"cost-center": "eng-42"}`)},
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	crd := builder.GenericControlPlaneCRD.DeepCopy()
	crd.Labels = map[string]string{
		clusterv1.GroupVersion.Group + "/v1beta2": clusterv1.GroupVersionControlPlane.Version,
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()

	cat := runtimecatalog.New()
	g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
	runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().WithCatalog(cat).Build()

	patchEngine := NewEngine(client, runtimeClient)
	g.Expect(patchEngine.Apply(ctx, blueprint, desired)).To(Succeed())

	gotLabels := desired.ControlPlane.Object.GetLabels()

	// The unrelated patch was applied as intended.
	g.Expect(gotLabels).To(HaveKeyWithValue("cost-center", "eng-42"))

	// System labels the engine explicitly preserves survive, as expected.
	g.Expect(gotLabels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, "cluster1"))
	g.Expect(gotLabels).To(HaveKeyWithValue(clusterv1.ClusterTopologyOwnedLabel, ""))

	// BUG: the topology/ClusterClass-declared "team" label was never touched by the patch above --
	// it targets a different key entirely -- yet it silently disappears because CopyFields overwrites
	// the whole metadata.labels map instead of merging into it, and "team" isn't in the small hardcoded
	// FieldsToPreserve allowlist in updateDesiredState.
	g.Expect(gotLabels).To(HaveKeyWithValue("team", "platform"),
		"topology/ClusterClass-declared label 'team' should survive an unrelated ClusterClass patch, but was silently dropped")
}

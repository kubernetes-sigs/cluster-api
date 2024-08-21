/*
Copyright 2021 The Kubernetes Authors.

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

// Package patches implement the patch engine.
package patches

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/api"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/external"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/inline"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
)

// Engine is a patch engine which applies patches defined in a ClusterBlueprint to a ClusterState.
type Engine interface {
	Apply(ctx context.Context, blueprint *scope.ClusterBlueprint, desired *scope.ClusterState) error
}

// NewEngine creates a new patch engine.
func NewEngine(runtimeClient runtimeclient.Client) Engine {
	return &engine{
		runtimeClient: runtimeClient,
	}
}

// engine implements the Engine interface.
type engine struct {
	runtimeClient runtimeclient.Client
}

// Apply applies patches to the desired state according to the patches from the ClusterClass, variables from the Cluster
// and builtin variables.
//   - A GeneratePatchesRequest with all templates and global and template-specific variables is created.
//   - Then for all ClusterClassPatches of a ClusterClass, JSON or JSON merge patches are generated
//     and successively applied to the templates in the GeneratePatchesRequest.
//   - Eventually the patched templates are used to update the specs of the desired objects.
func (e *engine) Apply(ctx context.Context, blueprint *scope.ClusterBlueprint, desired *scope.ClusterState) error {
	// Return if there are no patches.
	if len(blueprint.ClusterClass.Spec.Patches) == 0 {
		return nil
	}

	log := tlog.LoggerFrom(ctx)

	// Create a patch generation request.
	req, err := createRequest(blueprint, desired)
	if err != nil {
		return errors.Wrapf(err, "failed to generate patch request")
	}

	// Loop over patches in ClusterClass, generate patches and apply them to the request,
	// respecting the order in which they are defined.
	for i := range blueprint.ClusterClass.Spec.Patches {
		clusterClassPatch := blueprint.ClusterClass.Spec.Patches[i]
		ctx, log := log.WithValues("patch", clusterClassPatch.Name).Into(ctx)

		definitionFrom := clusterClassPatch.Name
		// If this isn't an external patch, use the inline patch name.
		if clusterClassPatch.External == nil {
			definitionFrom = clusterv1.VariableDefinitionFromInline
		}
		if err := addVariablesForPatch(blueprint, desired, req, definitionFrom); err != nil {
			return errors.Wrapf(err, "failed to calculate variables for patch %q", clusterClassPatch.Name)
		}
		log.V(5).Infof("Applying patch to templates")

		// Create patch generator for the current patch.
		generator, err := createPatchGenerator(e.runtimeClient, &clusterClassPatch)
		if err != nil {
			return err
		}

		// Generate patches.
		// NOTE: All the partial patches accumulate on top of the request, so the
		// patch generator in the next iteration of the loop will get the modified
		// version of the request (including the patched version of the templates).
		resp, err := generator.Generate(ctx, desired.Cluster, req)
		if err != nil {
			return errors.Wrapf(err, "failed to generate patches for patch %q", clusterClassPatch.Name)
		}

		// Apply patches to the request.
		if err := applyPatchesToRequest(ctx, req, resp); err != nil {
			return errors.Wrapf(err, "failed to apply patches for patch %q", clusterClassPatch.Name)
		}
	}

	// Convert request to validation request.
	validationRequest := convertToValidationRequest(req)

	// Loop over patches in ClusterClass and validate topology,
	// respecting the order in which they are defined.
	for i := range blueprint.ClusterClass.Spec.Patches {
		clusterClassPatch := blueprint.ClusterClass.Spec.Patches[i]

		if clusterClassPatch.External == nil || clusterClassPatch.External.ValidateExtension == nil {
			continue
		}

		ctx, log := log.WithValues("patch", clusterClassPatch.Name).Into(ctx)

		log.V(5).Infof("Validating topology")

		validator := external.NewValidator(e.runtimeClient, &clusterClassPatch)

		_, err := validator.Validate(ctx, desired.Cluster, validationRequest)
		if err != nil {
			return errors.Wrapf(err, "validation of patch %q failed", clusterClassPatch.Name)
		}
	}

	// Use patched templates to update the desired state objects.
	log.V(5).Infof("Applying patched templates to desired state")
	if err := updateDesiredState(ctx, req, blueprint, desired); err != nil {
		return errors.Wrapf(err, "failed to apply patches to desired state")
	}

	return nil
}

// addVariablesForPatch adds variables for a given ClusterClassPatch to the items in the PatchRequest.
func addVariablesForPatch(blueprint *scope.ClusterBlueprint, desired *scope.ClusterState, req *runtimehooksv1.GeneratePatchesRequest, definitionFrom string) error {
	// If there is no definitionFrom return an error.
	if definitionFrom == "" {
		return errors.New("failed to calculate variables: no patch name provided")
	}

	patchVariableDefinitions := definitionsForPatch(blueprint, definitionFrom)
	// Calculate global variables.
	globalVariables, err := variables.Global(blueprint.Topology, desired.Cluster, patchVariableDefinitions)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate global variables")
	}
	req.Variables = globalVariables

	// Calculate the Control Plane variables.
	controlPlaneVariables, err := variables.ControlPlane(&blueprint.Topology.ControlPlane, desired.ControlPlane.Object, desired.ControlPlane.InfrastructureMachineTemplate, patchVariableDefinitions)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate ControlPlane variables")
	}

	mdStateIndex := map[string]*scope.MachineDeploymentState{}
	for _, md := range desired.MachineDeployments {
		mdStateIndex[md.Object.Name] = md
	}
	mpStateIndex := map[string]*scope.MachinePoolState{}
	for _, mp := range desired.MachinePools {
		mpStateIndex[mp.Object.Name] = mp
	}
	for i, item := range req.Items {
		// If the item is a Control Plane add the Control Plane variables.
		if item.HolderReference.FieldPath == "spec.controlPlaneRef" {
			item.Variables = controlPlaneVariables
		}
		// If the item holder reference is a Control Plane machine add the Control Plane variables.
		if blueprint.HasControlPlaneInfrastructureMachine() &&
			item.HolderReference.FieldPath == strings.Join(contract.ControlPlane().MachineTemplate().InfrastructureRef().Path(), ".") {
			item.Variables = controlPlaneVariables
		}
		// If the item holder reference is a MachineDeployment calculate the variables for each MachineDeploymentTopology
		// and add them to the variables for the MachineDeployment.
		if item.HolderReference.Kind == "MachineDeployment" {
			md, ok := mdStateIndex[item.HolderReference.Name]
			if !ok {
				return errors.Errorf("could not find desired state for MachineDeployment %s", klog.KRef(item.HolderReference.Namespace, item.HolderReference.Name))
			}
			mdTopology, err := getMDTopologyFromMD(blueprint, md.Object)
			if err != nil {
				return err
			}

			// Calculate MachineDeployment variables.
			mdVariables, err := variables.MachineDeployment(mdTopology, md.Object, md.BootstrapTemplate, md.InfrastructureMachineTemplate, patchVariableDefinitions)
			if err != nil {
				return errors.Wrapf(err, "failed to calculate variables for %s", klog.KObj(md.Object))
			}
			item.Variables = mdVariables
		} else if item.HolderReference.Kind == "MachinePool" {
			mp, ok := mpStateIndex[item.HolderReference.Name]
			if !ok {
				return errors.Errorf("could not find desired state for MachinePool %s", klog.KRef(item.HolderReference.Namespace, item.HolderReference.Name))
			}
			mpTopology, err := getMPTopologyFromMP(blueprint, mp.Object)
			if err != nil {
				return err
			}

			// Calculate MachinePool variables.
			mpVariables, err := variables.MachinePool(mpTopology, mp.Object, mp.BootstrapObject, mp.InfrastructureMachinePoolObject, patchVariableDefinitions)
			if err != nil {
				return errors.Wrapf(err, "failed to calculate variables for %s", klog.KObj(mp.Object))
			}
			item.Variables = mpVariables
		}
		req.Items[i] = item
	}
	return nil
}

func getMDTopologyFromMD(blueprint *scope.ClusterBlueprint, md *clusterv1.MachineDeployment) (*clusterv1.MachineDeploymentTopology, error) {
	topologyName, ok := md.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
	if !ok {
		return nil, errors.Errorf("failed to get topology name for %s", klog.KObj(md))
	}
	mdTopology, err := lookupMDTopology(blueprint.Topology, topologyName)
	if err != nil {
		return nil, err
	}
	return mdTopology, nil
}

func getMPTopologyFromMP(blueprint *scope.ClusterBlueprint, mp *expv1.MachinePool) (*clusterv1.MachinePoolTopology, error) {
	topologyName, ok := mp.Labels[clusterv1.ClusterTopologyMachinePoolNameLabel]
	if !ok {
		return nil, errors.Errorf("failed to get topology name for %s", klog.KObj(mp))
	}
	mpTopology, err := lookupMPTopology(blueprint.Topology, topologyName)
	if err != nil {
		return nil, err
	}
	return mpTopology, nil
}

// createRequest creates a GeneratePatchesRequest based on the ClusterBlueprint and the desired state.
// NOTE: GenerateRequestTemplates are created for the templates of each individual MachineDeployment in the desired
// state. This is necessary because some builtin variables are MachineDeployment specific. For example version and
// replicas of a MachineDeployment.
// NOTE: A single GeneratePatchesRequest object is used to carry templates state across subsequent Generate calls.
// NOTE: This function does not add variables to items for the request, as the variables depend on the specific patch.
func createRequest(blueprint *scope.ClusterBlueprint, desired *scope.ClusterState) (*runtimehooksv1.GeneratePatchesRequest, error) {
	req := &runtimehooksv1.GeneratePatchesRequest{}

	// Add the InfrastructureClusterTemplate.
	t, err := newRequestItemBuilder(blueprint.InfrastructureClusterTemplate).
		WithHolder(desired.Cluster, clusterv1.GroupVersion.WithKind("Cluster"), "spec.infrastructureRef").
		Build()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare InfrastructureCluster template %s for patching",
			tlog.KObj{Obj: blueprint.InfrastructureClusterTemplate})
	}
	req.Items = append(req.Items, *t)

	// Add the ControlPlaneTemplate.
	t, err = newRequestItemBuilder(blueprint.ControlPlane.Template).
		WithHolder(desired.Cluster, clusterv1.GroupVersion.WithKind("Cluster"), "spec.controlPlaneRef").
		Build()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare ControlPlane template %s for patching",
			tlog.KObj{Obj: blueprint.ControlPlane.Template})
	}
	req.Items = append(req.Items, *t)

	// If the clusterClass mandates the controlPlane has infrastructureMachines,
	// add the InfrastructureMachineTemplate for control plane machines.
	if blueprint.HasControlPlaneInfrastructureMachine() {
		t, err := newRequestItemBuilder(blueprint.ControlPlane.InfrastructureMachineTemplate).
			WithHolder(desired.ControlPlane.Object, desired.ControlPlane.Object.GroupVersionKind(), strings.Join(contract.ControlPlane().MachineTemplate().InfrastructureRef().Path(), ".")).
			Build()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to prepare ControlPlane's machine template %s for patching",
				tlog.KObj{Obj: blueprint.ControlPlane.InfrastructureMachineTemplate})
		}
		req.Items = append(req.Items, *t)
	}

	// Add BootstrapConfigTemplate and InfrastructureMachine template for all MachineDeploymentTopologies
	// in the Cluster.
	// NOTE: We intentionally iterate over MachineDeployment in the Cluster instead of over
	// MachineDeploymentClasses in the ClusterClass because each MachineDeployment in a topology
	// has its own state, e.g. version or replicas. This state is used to calculate builtin variables,
	// which can then be used e.g. to compute the machine image for a specific Kubernetes version.
	for mdTopologyName, md := range desired.MachineDeployments {
		// Lookup MachineDeploymentTopology definition from cluster.spec.topology.
		mdTopology, err := lookupMDTopology(blueprint.Topology, mdTopologyName)
		if err != nil {
			return nil, err
		}

		// Get corresponding MachineDeploymentClass from the ClusterClass.
		mdClass, ok := blueprint.MachineDeployments[mdTopology.Class]
		if !ok {
			return nil, errors.Errorf("failed to lookup MachineDeployment class %q in ClusterClass", mdTopology.Class)
		}

		// Add the BootstrapTemplate.
		t, err := newRequestItemBuilder(mdClass.BootstrapTemplate).
			WithHolder(md.Object, clusterv1.GroupVersion.WithKind("MachineDeployment"), "spec.template.spec.bootstrap.configRef").
			Build()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to prepare BootstrapConfig template %s for MachineDeployment topology %s for patching",
				tlog.KObj{Obj: mdClass.BootstrapTemplate}, mdTopologyName)
		}
		req.Items = append(req.Items, *t)

		// Add the InfrastructureMachineTemplate.
		t, err = newRequestItemBuilder(mdClass.InfrastructureMachineTemplate).
			WithHolder(md.Object, clusterv1.GroupVersion.WithKind("MachineDeployment"), "spec.template.spec.infrastructureRef").
			Build()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to prepare InfrastructureMachine template %s for MachineDeployment topology %s for patching",
				tlog.KObj{Obj: mdClass.InfrastructureMachineTemplate}, mdTopologyName)
		}
		req.Items = append(req.Items, *t)
	}

	// Add BootstrapConfigTemplate and InfrastructureMachinePoolTemplate for all MachinePoolTopologies
	// in the Cluster.
	// NOTE: We intentionally iterate over MachinePool in the Cluster instead of over
	// MachinePoolClasses in the ClusterClass because each MachinePool in a topology
	// has its own state, e.g. version or replicas. This state is used to calculate builtin variables,
	// which can then be used e.g. to compute the machine image for a specific Kubernetes version.
	for mpTopologyName, mp := range desired.MachinePools {
		// Lookup MachinePoolTopology definition from cluster.spec.topology.
		mpTopology, err := lookupMPTopology(blueprint.Topology, mpTopologyName)
		if err != nil {
			return nil, err
		}

		// Get corresponding MachinePoolClass from the ClusterClass.
		mpClass, ok := blueprint.MachinePools[mpTopology.Class]
		if !ok {
			return nil, errors.Errorf("failed to lookup MachinePool class %q in ClusterClass", mpTopology.Class)
		}

		// Add the BootstrapTemplate.
		t, err := newRequestItemBuilder(mpClass.BootstrapTemplate).
			WithHolder(mp.Object, expv1.GroupVersion.WithKind("MachinePool"), "spec.template.spec.bootstrap.configRef").
			Build()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to prepare BootstrapConfig template %s for MachinePool topology %s for patching",
				tlog.KObj{Obj: mpClass.BootstrapTemplate}, mpTopologyName)
		}
		req.Items = append(req.Items, *t)

		// Add the InfrastructureMachineTemplate.
		t, err = newRequestItemBuilder(mpClass.InfrastructureMachinePoolTemplate).
			WithHolder(mp.Object, expv1.GroupVersion.WithKind("MachinePool"), "spec.template.spec.infrastructureRef").
			Build()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to prepare InfrastructureMachinePoolTemplate %s for MachinePool topology %s for patching",
				tlog.KObj{Obj: mpClass.InfrastructureMachinePoolTemplate}, mpTopologyName)
		}
		req.Items = append(req.Items, *t)
	}

	return req, nil
}

// lookupMDTopology looks up the MachineDeploymentTopology based on a mdTopologyName in a topology.
func lookupMDTopology(topology *clusterv1.Topology, mdTopologyName string) (*clusterv1.MachineDeploymentTopology, error) {
	for _, mdTopology := range topology.Workers.MachineDeployments {
		if mdTopology.Name == mdTopologyName {
			return &mdTopology, nil
		}
	}
	return nil, errors.Errorf("failed to lookup MachineDeployment topology %q in Cluster.spec.topology.workers.machineDeployments", mdTopologyName)
}

// lookupMPTopology looks up the MachinePoolTopology based on a mpTopologyName in a topology.
func lookupMPTopology(topology *clusterv1.Topology, mpTopologyName string) (*clusterv1.MachinePoolTopology, error) {
	for _, mpTopology := range topology.Workers.MachinePools {
		if mpTopology.Name == mpTopologyName {
			return &mpTopology, nil
		}
	}
	return nil, errors.Errorf("failed to lookup MachinePool topology %q in Cluster.spec.topology.workers.machinePools", mpTopologyName)
}

// createPatchGenerator creates a patch generator for the given patch.
// NOTE: Currently only inline JSON patches are supported; in the future we will add
// external patches as well.
func createPatchGenerator(runtimeClient runtimeclient.Client, patch *clusterv1.ClusterClassPatch) (api.Generator, error) {
	// Return a jsonPatchGenerator if there are PatchDefinitions in the patch.
	if len(patch.Definitions) > 0 {
		return inline.NewGenerator(patch), nil
	}
	// Return an externalPatchGenerator if there is an external configuration in the patch.
	if patch.External != nil && patch.External.GenerateExtension != nil {
		if !feature.Gates.Enabled(feature.RuntimeSDK) {
			return nil, errors.Errorf("can not use external patch %q if RuntimeSDK feature flag is disabled", patch.Name)
		}
		if runtimeClient == nil {
			return nil, errors.Errorf("failed to create patch generator for patch %q: runtimeClient is not set up", patch.Name)
		}
		return external.NewGenerator(runtimeClient, patch), nil
	}

	return nil, errors.Errorf("failed to create patch generator for patch %q", patch.Name)
}

// applyPatchesToRequest updates the templates of a GeneratePatchesRequest by applying the patches
// of a GeneratePatchesResponse.
func applyPatchesToRequest(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) error {
	for _, patch := range resp.Items {
		if err := applyPatchToRequest(ctx, req, patch); err != nil {
			return err
		}
	}
	return nil
}

func applyPatchToRequest(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, patch runtimehooksv1.GeneratePatchesResponseItem) (reterr error) {
	log := tlog.LoggerFrom(ctx).WithValues("uid", patch.UID)

	defer func() {
		if r := recover(); r != nil {
			log.Infof("Observed a panic when applying patch: %v\n%s", r, string(debug.Stack()))
			reterr = kerrors.NewAggregate([]error{reterr, fmt.Errorf("observed a panic when applying patch: %v", r)})
		}
	}()

	// Get the request item the patch belongs to.
	requestItem := getRequestItemByUID(req, patch.UID)

	// If a patch doesn't have a corresponding request item, the patch is invalid.
	if requestItem == nil {
		log.Infof("Unable to find corresponding request item with uid %q for the patch", patch.UID)
		return errors.Errorf("unable to find corresponding request item for patch")
	}

	// Use the patch to create a patched copy of the template.
	var patchedTemplate []byte
	var err error

	switch patch.PatchType {
	case runtimehooksv1.JSONPatchType:
		log.V(5).Infof("Accumulating JSON patch: %s", string(patch.Patch))
		jsonPatch, err := jsonpatch.DecodePatch(patch.Patch)
		if err != nil {
			log.Infof("Failed to apply patch with uid %q: error decoding json patch (RFC6902): %s: %s", requestItem.UID, string(patch.Patch), err)
			return errors.Wrap(err, "failed to apply patch: error decoding json patch (RFC6902)")
		}

		patchedTemplate, err = jsonPatch.Apply(requestItem.Object.Raw)
		if err != nil {
			log.Infof("Failed to apply patch with uid %q: error applying json patch (RFC6902): %s: %s", requestItem.UID, string(patch.Patch), err)
			return errors.Wrap(err, "failed to apply patch: error applying json patch (RFC6902)")
		}
	case runtimehooksv1.JSONMergePatchType:
		log.V(5).Infof("Accumulating JSON merge patch: %s", string(patch.Patch))
		patchedTemplate, err = jsonpatch.MergePatch(requestItem.Object.Raw, patch.Patch)
		if err != nil {
			log.Infof("Failed to apply patch with uid %q: error applying json merge patch (RFC7386): %s: %s", requestItem.UID, string(patch.Patch), err)
			return errors.Wrap(err, "failed to apply patch: error applying json merge patch (RFC7386)")
		}
	}

	// Overwrite the spec of template.Template with the spec of the patchedTemplate,
	// to ensure that we only pick up changes to the spec.
	if err := patchTemplateSpec(&requestItem.Object, patchedTemplate); err != nil {
		log.Infof("Failed to apply patch to template %s: %s", requestItem.UID, err)
		return errors.Wrap(err, "failed to apply patch to template")
	}

	return nil
}

// convertToValidationRequest converts a GeneratePatchesRequest to a ValidateTopologyRequest.
func convertToValidationRequest(generateRequest *runtimehooksv1.GeneratePatchesRequest) *runtimehooksv1.ValidateTopologyRequest {
	validationRequest := &runtimehooksv1.ValidateTopologyRequest{}
	validationRequest.Variables = generateRequest.Variables

	for i := range generateRequest.Items {
		item := generateRequest.Items[i]

		validationRequest.Items = append(validationRequest.Items, &runtimehooksv1.ValidateTopologyRequestItem{
			HolderReference: item.HolderReference,
			Object:          item.Object,
			Variables:       item.Variables,
		})
	}

	return validationRequest
}

// updateDesiredState uses the patched templates of a GeneratePatchesRequest to update the desired state.
// NOTE: This func should be called after all the patches have been applied to the GeneratePatchesRequest.
func updateDesiredState(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, blueprint *scope.ClusterBlueprint, desired *scope.ClusterState) error {
	var err error

	// Update the InfrastructureCluster.
	infrastructureClusterTemplate, err := getTemplateAsUnstructured(req, "Cluster", "spec.infrastructureRef", requestTopologyName{})
	if err != nil {
		return err
	}
	if err := patchObject(ctx, desired.InfrastructureCluster, infrastructureClusterTemplate); err != nil {
		return err
	}

	// Update the ControlPlane.
	controlPlaneTemplate, err := getTemplateAsUnstructured(req, "Cluster", "spec.controlPlaneRef", requestTopologyName{})
	if err != nil {
		return err
	}
	if err := patchObject(ctx, desired.ControlPlane.Object, controlPlaneTemplate, PreserveFields{
		contract.ControlPlane().MachineTemplate().Metadata().Path(),
		contract.ControlPlane().MachineTemplate().InfrastructureRef().Path(),
		contract.ControlPlane().MachineTemplate().NodeDrainTimeout().Path(),
		contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Path(),
		contract.ControlPlane().MachineTemplate().NodeDeletionTimeout().Path(),
		contract.ControlPlane().Replicas().Path(),
		contract.ControlPlane().Version().Path(),
	}); err != nil {
		return err
	}

	// If the ClusterClass mandates the ControlPlane has InfrastructureMachines,
	// update the InfrastructureMachineTemplate for ControlPlane machines.
	if blueprint.HasControlPlaneInfrastructureMachine() {
		infrastructureMachineTemplate, err := getTemplateAsUnstructured(req, desired.ControlPlane.Object.GetKind(), strings.Join(contract.ControlPlane().MachineTemplate().InfrastructureRef().Path(), "."), requestTopologyName{})
		if err != nil {
			return err
		}
		if err := patchTemplate(ctx, desired.ControlPlane.InfrastructureMachineTemplate, infrastructureMachineTemplate); err != nil {
			return err
		}
	}

	// Update the templates for all MachineDeployments.
	for mdTopologyName, md := range desired.MachineDeployments {
		topologyName := requestTopologyName{mdTopologyName: mdTopologyName}
		// Update the BootstrapConfigTemplate.
		bootstrapTemplate, err := getTemplateAsUnstructured(req, "MachineDeployment", "spec.template.spec.bootstrap.configRef", topologyName)
		if err != nil {
			return err
		}
		if err := patchTemplate(ctx, md.BootstrapTemplate, bootstrapTemplate); err != nil {
			return err
		}

		// Update the InfrastructureMachineTemplate.
		infrastructureMachineTemplate, err := getTemplateAsUnstructured(req, "MachineDeployment", "spec.template.spec.infrastructureRef", topologyName)
		if err != nil {
			return err
		}
		if err := patchTemplate(ctx, md.InfrastructureMachineTemplate, infrastructureMachineTemplate); err != nil {
			return err
		}
	}

	// Update the templates for all MachinePools.
	for mpTopologyName, mp := range desired.MachinePools {
		topologyName := requestTopologyName{mpTopologyName: mpTopologyName}
		// Update the BootstrapConfig.
		bootstrapTemplate, err := getTemplateAsUnstructured(req, "MachinePool", "spec.template.spec.bootstrap.configRef", topologyName)
		if err != nil {
			return err
		}
		if err := patchObject(ctx, mp.BootstrapObject, bootstrapTemplate); err != nil {
			return err
		}

		// Update the InfrastructureMachinePool.
		infrastructureMachinePoolTemplate, err := getTemplateAsUnstructured(req, "MachinePool", "spec.template.spec.infrastructureRef", topologyName)
		if err != nil {
			return err
		}
		if err := patchObject(ctx, mp.InfrastructureMachinePoolObject, infrastructureMachinePoolTemplate); err != nil {
			return err
		}
	}

	return nil
}

// definitionsForPatch returns a set which of variables in a ClusterClass defined by "definitionFrom".
func definitionsForPatch(blueprint *scope.ClusterBlueprint, definitionFrom string) map[string]bool {
	variableDefinitionsForPatch := make(map[string]bool)
	for _, definitionsWithName := range blueprint.ClusterClass.Status.Variables {
		for _, definition := range definitionsWithName.Definitions {
			if definition.From == definitionFrom {
				variableDefinitionsForPatch[definitionsWithName.Name] = true
			}
		}
	}
	return variableDefinitionsForPatch
}

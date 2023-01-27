/*
Copyright 2022 The Kubernetes Authors.

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

// Package topologymutation contains the handlers for the topologymutation webhook.
//
// The implementation of the handlers is specifically designed for Cluster API E2E tests use cases.
// When implementing custom RuntimeExtension, it is only required to expose HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
package topologymutation

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/topologymutation"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
	"sigs.k8s.io/cluster-api/internal/topology/variables"
	"sigs.k8s.io/cluster-api/test/extension/api"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/version"
)

var (
	cgroupDriverCgroupfs            = "cgroupfs"
	cgroupDriverPatchVersionCeiling = semver.Version{Major: 1, Minor: 24}
)

// ExtensionHandlers provides a common struct shared across the topology mutation hooks handlers;
// this is convenient because in Cluster API's E2E tests all of them are using a decoder for working with typed
// API objects, which makes code easier to read and less error prone than using unstructured or working with raw json/yaml.
// NOTE: it is not mandatory to use a ExtensionHandlers in custom RuntimeExtension, what is important
// is to expose HandlerFunc with the signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
type ExtensionHandlers struct {
	decoder runtime.Decoder
}

// NewExtensionHandlers returns a new ExtensionHandlers for the topology mutation hook handlers.
func NewExtensionHandlers(scheme *runtime.Scheme) *ExtensionHandlers {
	return &ExtensionHandlers{
		// Add the apiGroups being handled to the decoder
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			infrav1.GroupVersion,
			controlplanev1.GroupVersion,
			bootstrapv1.GroupVersion,
		),
	}
}

// GeneratePatches implements the HandlerFunc for the GeneratePatches hook.
// The hook adds to the response the patches we are using in Cluster API E2E tests.
// NOTE: custom RuntimeExtension must implement the body of this func according to the specific use case.
func (h *ExtensionHandlers) GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("GeneratePatches is called")

	// FIXME: validate variable values against variable schemas (just like in the webhook/reconciler today).
	var userVariableValues []clusterv1.ClusterVariable
	for _, v := range req.Variables {
		if v.Name == patchvariables.BuiltinsName {
			continue
		}
		userVariableValues = append(userVariableValues, clusterv1.ClusterVariable{
			Name:  v.Name,
			Value: v.Value,
		})
	}
	errs := variables.ValidateClusterVariables(userVariableValues, api.VariableDefinitions, field.NewPath(""))
	if len(errs) > 0 {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = fmt.Sprintf("Error validing variables: %s", errs.ToAggregate().Error())
	}

	// By using WalkTemplates it is possible to implement patches using typed API objects, which makes code
	// easier to read and less error prone than using unstructured or working with raw json/yaml.
	// IMPORTANT: by unit testing this func/nested func properly, it is possible to prevent unexpected rollouts when patches are modified.
	topologymutation.WalkTemplates(ctx, h.decoder, req, resp, func(ctx context.Context, obj runtime.Object, builtinVariable patchvariables.Builtins, variables interface{}, holderRef runtimehooksv1.HolderReference) error {
		log := ctrl.LoggerFrom(ctx)

		vars, ok := variables.(api.Variables)
		if !ok {
			return errors.Errorf("wrong variable type")
		}

		switch obj := obj.(type) {
		case *infrav1.DockerClusterTemplate:
			if err := patchDockerClusterTemplate(ctx, obj, builtinVariable, vars); err != nil {
				log.Error(err, "error patching DockerClusterTemplate")
				return errors.Wrap(err, "error patching DockerClusterTemplate")
			}
		case *controlplanev1.KubeadmControlPlaneTemplate:
			err := patchKubeadmControlPlaneTemplate(ctx, obj, builtinVariable, vars)
			if err != nil {
				log.Error(err, "error patching KubeadmControlPlaneTemplate")
				return errors.Wrapf(err, "error patching KubeadmControlPlaneTemplate")
			}
		case *bootstrapv1.KubeadmConfigTemplate:
			// NOTE: KubeadmConfigTemplate could be linked to one or more of the existing MachineDeployment class;
			// the patchKubeadmConfigTemplate func shows how to implement patches only for KubeadmConfigTemplates
			// linked to a specific MachineDeployment class; another option is to check the holderRef value and call
			// this func or more specialized func conditionally.
			if err := patchKubeadmConfigTemplate(ctx, obj, builtinVariable, vars); err != nil {
				log.Error(err, "error patching KubeadmConfigTemplate")
				return errors.Wrap(err, "error patching KubeadmConfigTemplate")
			}
		case *infrav1.DockerMachineTemplate:
			// NOTE: DockerMachineTemplate could be linked to the ControlPlane or one or more of the existing MachineDeployment class;
			// the patchDockerMachineTemplate func shows how to implement different patches for DockerMachineTemplate
			// linked to ControlPlane or for DockerMachineTemplate linked to MachineDeployment classes; another option
			// is to check the holderRef value and call this func or more specialized func conditionally.
			if err := patchDockerMachineTemplate(ctx, obj, builtinVariable, vars); err != nil {
				log.Error(err, "error patching DockerMachineTemplate")
				return errors.Wrap(err, "error patching DockerMachineTemplate")
			}
		}
		return nil
	})
}

// patchDockerClusterTemplate patches the DockerClusterTemplate.
// It sets the LoadBalancer.ImageRepository if the lbImageRepository variable is provided.
// NOTE: this patch is not required for any special reason, it is used for testing the patch machinery itself.
func patchDockerClusterTemplate(_ context.Context, dockerClusterTemplate *infrav1.DockerClusterTemplate, builtinVariable patchvariables.Builtins, variables api.Variables) error {
	if variables.LBImageRepository != "" {
		dockerClusterTemplate.Spec.Template.Spec.LoadBalancer.ImageRepository = variables.LBImageRepository
	}
	return nil
}

// patchKubeadmControlPlaneTemplate patches the ControlPlaneTemplate.
// It sets KubeletExtraArgs["cgroup-driver"] to cgroupfs for Kubernetes < 1.24; this patch is required for tests
// to work with older kind images.
// It also sets the RolloutStrategy.RollingUpdate.MaxSurge if the kubeadmControlPlaneMaxSurge is provided.
// NOTE: RolloutStrategy.RollingUpdate.MaxSurge patch is not required for any special reason, it is used for testing the patch machinery itself.
func patchKubeadmControlPlaneTemplate(ctx context.Context, kcpTemplate *controlplanev1.KubeadmControlPlaneTemplate, builtinVariable patchvariables.Builtins, variables api.Variables) error {
	log := ctrl.LoggerFrom(ctx)

	// 1) If the Kubernetes version from builtin.controlPlane.version is below 1.24.0 set "cgroup-driver": "cgroupfs" to
	//    - kubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs
	//    - kubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs

	// This is a required variable. Return an error if it's not found.
	// NOTE: this should never happen because it is enforced by the patch engine.
	// FIXME: if we want to validate this, we should do it while parsing the builtin variable
	if builtinVariable.ControlPlane == nil || builtinVariable.ControlPlane.Version == "" {
		return errors.New("could not set cgroup-driver to control plane template kubeletExtraArgs: variable \"builtin.controlPlane.version\" not found")
	}

	controlPlaneVersion, err := version.ParseMajorMinorPatchTolerant(builtinVariable.ControlPlane.Version)
	if err != nil {
		return err
	}
	if version.Compare(controlPlaneVersion, cgroupDriverPatchVersionCeiling) == -1 {
		log.Info(fmt.Sprintf("Setting KubeadmControlPlaneTemplate cgroup-driver to %q", cgroupDriverCgroupfs))
		// Set the cgroupDriver in the InitConfiguration.
		if kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration == nil {
			kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration = &bootstrapv1.InitConfiguration{}
		}
		if kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs == nil {
			kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs = map[string]string{}
		}
		kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs["cgroup-driver"] = cgroupDriverCgroupfs

		// Set the cgroupDriver in the JoinConfiguration.
		if kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration == nil {
			kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		if kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs == nil {
			kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs = map[string]string{}
		}
		kcpTemplate.Spec.Template.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs["cgroup-driver"] = cgroupDriverCgroupfs
	}

	// 2) Patch RolloutStrategy RollingUpdate MaxSurge with the value from the Cluster Topology variable.
	//    If this is unset continue as this variable is not required.
	if variables.KubeadmControlPlaneMaxSurge != "" {
		// This has to be converted to IntOrString type.
		kubeadmControlPlaneMaxSurgeIntOrString := intstrutil.Parse(variables.KubeadmControlPlaneMaxSurge)
		log.Info(fmt.Sprintf("Setting KubeadmControlPlaneMaxSurge to %q", kubeadmControlPlaneMaxSurgeIntOrString.String()))
		if kcpTemplate.Spec.Template.Spec.RolloutStrategy == nil {
			kcpTemplate.Spec.Template.Spec.RolloutStrategy = &controlplanev1.RolloutStrategy{}
		}
		if kcpTemplate.Spec.Template.Spec.RolloutStrategy.RollingUpdate == nil {
			kcpTemplate.Spec.Template.Spec.RolloutStrategy.RollingUpdate = &controlplanev1.RollingUpdate{}
		}
		kcpTemplate.Spec.Template.Spec.RolloutStrategy.RollingUpdate.MaxSurge = &kubeadmControlPlaneMaxSurgeIntOrString
	}
	return nil
}

// patchKubeadmConfigTemplate patches the ControlPlaneTemplate.
// Only for the templates linked to the default-worker MachineDeployment class, It sets KubeletExtraArgs["cgroup-driver"]
// to cgroupfs for Kubernetes < 1.24; this patch is required for tests to work with older kind images.
func patchKubeadmConfigTemplate(ctx context.Context, k *bootstrapv1.KubeadmConfigTemplate, builtinVariable patchvariables.Builtins, variables api.Variables) error {
	log := ctrl.LoggerFrom(ctx)

	// This is a required variable. Return an error if it's not found.
	// NOTE: this should never happen because it is enforced by the patch engine.
	if builtinVariable.MachineDeployment == nil || builtinVariable.MachineDeployment.Class == "" {
		return errors.New("could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs: variable \"builtin.machineDeployment.class\" not found")
	}

	if builtinVariable.MachineDeployment.Class == "default-worker" {
		// If the Kubernetes version from builtin.machineDeployment.version is below 1.24.0 set "cgroup-driver": "cgroupDriverCgroupfs" to
		//    - InitConfiguration.KubeletExtraArgs
		//    - JoinConfiguration.KubeletExtraArgs
		// NOTE: MachineDeployment version might be different than Cluster.version or other MachineDeployment's versions;
		// the builtin variables provides the right version to use.
		// This is a required variable. Return an error if it's not found.
		if builtinVariable.MachineDeployment == nil || builtinVariable.MachineDeployment.Version == "" {
			return errors.New("could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs: variable \"builtin.machineDeployment.version\" not found")
		}
		machineDeploymentVersion, err := version.ParseMajorMinorPatchTolerant(builtinVariable.MachineDeployment.Version)
		if err != nil {
			return errors.Wrap(err, "could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs")
		}
		if version.Compare(machineDeploymentVersion, cgroupDriverPatchVersionCeiling) == -1 {
			log.Info(fmt.Sprintf("Setting KubeadmConfigTemplate cgroup-driver to %q", cgroupDriverCgroupfs))

			// Set the cgroupDriver in the JoinConfiguration.
			if k.Spec.Template.Spec.JoinConfiguration == nil {
				k.Spec.Template.Spec.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
			}
			if k.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs == nil {
				k.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs = map[string]string{}
			}

			k.Spec.Template.Spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs["cgroup-driver"] = cgroupDriverCgroupfs
		}
	}
	return nil
}

// patchDockerMachineTemplate patches the DockerMachineTemplate.
// It sets the CustomImage to an image for the version in use by the controlPlane or by the MachineDeployment
// the DockerMachineTemplate belongs to. This patch is required to pick up the kind image with the required Kubernetes version.
func patchDockerMachineTemplate(ctx context.Context, dockerMachineTemplate *infrav1.DockerMachineTemplate, builtinVariable patchvariables.Builtins, variables api.Variables) error {
	log := ctrl.LoggerFrom(ctx)

	// If the DockerMachineTemplate belongs to the ControlPlane, set the images using the ControlPlane version.
	// NOTE: ControlPlane version might be different than Cluster.version or MachineDeployment's versions;
	// the builtin variables provides the right version to use.
	// NOTE: This works by checking the existence of a builtin variable that exists only for templates liked to the ControlPlane.
	if builtinVariable.ControlPlane != nil && builtinVariable.ControlPlane.Version != "" {
		_, err := version.ParseMajorMinorPatchTolerant(builtinVariable.ControlPlane.Version)
		if err != nil {
			return errors.Wrap(err, "could not parse control plane version")
		}
		customImage := fmt.Sprintf("kindest/node:%s", strings.ReplaceAll(builtinVariable.ControlPlane.Version, "+", "_"))
		log.Info(fmt.Sprintf("Setting MachineDeployment custom image to %q", customImage))
		dockerMachineTemplate.Spec.Template.Spec.CustomImage = customImage
		// return early if we have successfully patched a control plane dockerMachineTemplate
		return nil
	}

	// If the DockerMachineTemplate belongs to a MachineDeployment, set the images the MachineDeployment version.
	// NOTE: MachineDeployment version might be different than Cluster.version or other MachineDeployment's versions;
	// the builtin variables provides the right version to use.
	// NOTE: This works by checking the existence of a built in variable that exists only for templates liked to MachineDeployments.
	if builtinVariable.MachineDeployment != nil && builtinVariable.MachineDeployment.Version != "" {
		_, err := version.ParseMajorMinorPatchTolerant(builtinVariable.MachineDeployment.Version)
		if err != nil {
			return errors.Wrap(err, "could not parse MachineDeployment version")
		}
		customImage := fmt.Sprintf("kindest/node:%s", strings.ReplaceAll(builtinVariable.MachineDeployment.Version, "+", "_"))
		log.Info(fmt.Sprintf("Setting MachineDeployment customImage to %q", customImage))
		dockerMachineTemplate.Spec.Template.Spec.CustomImage = customImage
		return nil
	}

	// If the Docker Machine didn't have variables for either a control plane or a machineDeployment return an error.
	// NOTE: this should never happen because it is enforced by the patch engine.
	return errors.New("no version variables found for DockerMachineTemplate patch")
}

// ValidateTopology implements the HandlerFunc for the ValidateTopology hook.
// Cluster API E2E currently are just validating the hook gets called.
// NOTE: custom RuntimeExtension must implement the body of this func according to the specific use case.
func (h *ExtensionHandlers) ValidateTopology(ctx context.Context, _ *runtimehooksv1.ValidateTopologyRequest, resp *runtimehooksv1.ValidateTopologyResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("ValidateTopology called")

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DiscoverVariables implements the HandlerFunc for the DiscoverVariables hook.
// # Test via Tilt:
// ## First terminal: kubectl proxy
// ## Second terminal: curl -X 'POST' 'http://127.0.0.1:8001/api/v1/namespaces/test-extension-system/services/https:test-extension-webhook-service:443/proxy/hooks.runtime.cluster.x-k8s.io/v1alpha1/discovervariables/discover-variables' -d '{"apiVersion":"hooks.runtime.cluster.x-k8s.io/v1alpha1","kind":"DiscoverVariablesRequest"}' | jq
func (h *ExtensionHandlers) DiscoverVariables(ctx context.Context, _ *runtimehooksv1.DiscoverVariablesRequest, resp *runtimehooksv1.DiscoverVariablesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("DiscoverVariables called")

	resp.Variables = api.VariableDefinitions
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

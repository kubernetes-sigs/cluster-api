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
// signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
package topologymutation

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/topologymutation"
	infrav1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	infraexpv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
)

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;patch;update;create

// ExtensionHandlers provides a common struct shared across the topology mutation hooks handlers;
// this is convenient because in Cluster API's E2E tests all of them are using a decoder for working with typed
// API objects, which makes code easier to read and less error prone than using unstructured or working with raw json/yaml.
// NOTE: it is not mandatory to use a ExtensionHandlers in custom RuntimeExtension, what is important
// is to expose HandlerFunc with the signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
type ExtensionHandlers struct {
	decoder runtime.Decoder
}

// NewExtensionHandlers returns a new ExtensionHandlers for the topology mutation hook handlers.
func NewExtensionHandlers() *ExtensionHandlers {
	scheme := runtime.NewScheme()
	_ = infrav1beta1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = infraexpv1beta1.AddToScheme(scheme)
	_ = infraexpv1.AddToScheme(scheme)
	_ = bootstrapv1beta1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1beta1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	return &ExtensionHandlers{
		// Add the apiGroups being handled to the decoder
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			infrav1beta1.GroupVersion,
			infrav1.GroupVersion,
			bootstrapv1.GroupVersion,
			bootstrapv1beta1.GroupVersion,
			controlplanev1beta1.GroupVersion,
			controlplanev1.GroupVersion,
		),
	}
}

// GeneratePatches implements the HandlerFunc for the GeneratePatches hook.
// The hook adds to the response the patches we are using in Cluster API E2E tests.
// NOTE: custom RuntimeExtension must implement the body of this func according to the specific use case.
func (h *ExtensionHandlers) GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("GeneratePatches is called")

	// TODO: validate variables.

	// By using WalkTemplates it is possible to implement patches using typed API objects, which makes code
	// easier to read and less error prone than using unstructured or working with raw json/yaml.
	// IMPORTANT: by unit testing this func/nested func properly, it is possible to prevent unexpected rollouts when patches are modified.
	topologymutation.WalkTemplates(ctx, h.decoder, req, resp, func(ctx context.Context, obj runtime.Object, variables map[string]apiextensionsv1.JSON, _ runtimehooksv1.HolderReference) error {
		log := ctrl.LoggerFrom(ctx)

		switch obj.(type) {
		case *infrav1beta1.DockerClusterTemplate, *infrav1.DockerClusterTemplate:
			if err := patchDockerClusterTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "Error patching DockerClusterTemplate")
				return errors.Wrap(err, "error patching DockerClusterTemplate")
			}
		case *controlplanev1beta1.KubeadmControlPlaneTemplate, *controlplanev1.KubeadmControlPlaneTemplate:
			err := patchKubeadmControlPlaneTemplate(ctx, obj, variables)
			if err != nil {
				log.Error(err, "Error patching KubeadmControlPlaneTemplate")
				return errors.Wrapf(err, "error patching KubeadmControlPlaneTemplate")
			}
		case *bootstrapv1beta1.KubeadmConfigTemplate, *bootstrapv1.KubeadmConfigTemplate:
			// NOTE: KubeadmConfigTemplate could be linked to one or more of the existing MachineDeployment class;
			// the patchKubeadmConfigTemplate func shows how to implement patches only for KubeadmConfigTemplates
			// linked to a specific MachineDeployment class; another option is to check the holderRef value and call
			// this func or more specialized func conditionally.
			if err := patchKubeadmConfigTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "Error patching KubeadmConfigTemplate")
				return errors.Wrap(err, "error patching KubeadmConfigTemplate")
			}
		case *infrav1beta1.DockerMachineTemplate, *infrav1.DockerMachineTemplate:
			// NOTE: DockerMachineTemplate could be linked to the ControlPlane or one or more of the existing MachineDeployment class;
			// the patchDockerMachineTemplate func shows how to implement different patches for DockerMachineTemplate
			// linked to ControlPlane or for DockerMachineTemplate linked to MachineDeployment classes; another option
			// is to check the holderRef value and call this func or more specialized func conditionally.
			if err := patchDockerMachineTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "Error patching DockerMachineTemplate")
				return errors.Wrap(err, "error patching DockerMachineTemplate")
			}
		case *infraexpv1beta1.DockerMachinePoolTemplate, *infraexpv1.DockerMachinePoolTemplate:
			if err := patchDockerMachinePoolTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "Error patching DockerMachinePoolTemplate")
				return errors.Wrap(err, "error patching DockerMachinePoolTemplate")
			}
		}
		return nil
	}, topologymutation.FailForUnknownTypes{})
}

// patchDockerClusterTemplate patches the DockerClusterTemplate.
// It sets the LoadBalancer.ImageRepository if the imageRepository variable is provided.
// NOTE: this patch is not required for any special reason, it is used for testing the patch machinery itself.
func patchDockerClusterTemplate(_ context.Context, obj runtime.Object, templateVariables map[string]apiextensionsv1.JSON) error {
	imageRepo, err := topologymutation.GetStringVariable(templateVariables, "imageRepository")
	if err != nil {
		if topologymutation.IsNotFoundError(err) {
			return nil
		}
		return errors.Wrap(err, "could not set DockerClusterTemplate loadBalancer imageRepository")
	}

	dockerClusterTemplateV1Beta1, ok := obj.(*infrav1beta1.DockerClusterTemplate)
	if ok {
		dockerClusterTemplateV1Beta1.Spec.Template.Spec.LoadBalancer.ImageRepository = imageRepo
		return nil
	}

	dockerClusterTemplate, ok := obj.(*infrav1.DockerClusterTemplate)
	if ok {
		dockerClusterTemplate.Spec.Template.Spec.LoadBalancer.ImageRepository = imageRepo
		return nil
	}

	return nil
}

// patchKubeadmControlPlaneTemplate patches the ControlPlaneTemplate.
// It sets the RolloutStrategy.RollingUpdate.MaxSurge if the kubeadmControlPlaneMaxSurge is provided.
// NOTE: RolloutStrategy.RollingUpdate.MaxSurge patch is not required for any special reason, it is used for testing the patch machinery itself.
func patchKubeadmControlPlaneTemplate(ctx context.Context, obj runtime.Object, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)

	// 1) Patch RolloutStrategy RollingUpdate MaxSurge with the value from the Cluster Topology variable.
	//    If this is unset continue as this variable is not required.
	kcpControlPlaneMaxSurge, err := topologymutation.GetStringVariable(templateVariables, "kubeadmControlPlaneMaxSurge")
	if err != nil {
		if topologymutation.IsNotFoundError(err) {
			return nil
		}
		return errors.Wrap(err, "could not set KubeadmControlPlaneTemplate MaxSurge")
	}

	// This has to be converted to IntOrString type.
	kubeadmControlPlaneMaxSurgeIntOrString := intstrutil.Parse(kcpControlPlaneMaxSurge)
	log.Info(fmt.Sprintf("Setting KubeadmControlPlaneMaxSurge to %q", kubeadmControlPlaneMaxSurgeIntOrString.String()))

	kcpTemplateV1Beta1, ok := obj.(*controlplanev1beta1.KubeadmControlPlaneTemplate)
	if ok {
		if kcpTemplateV1Beta1.Spec.Template.Spec.RolloutStrategy == nil {
			kcpTemplateV1Beta1.Spec.Template.Spec.RolloutStrategy = &controlplanev1beta1.RolloutStrategy{}
		}
		if kcpTemplateV1Beta1.Spec.Template.Spec.RolloutStrategy.RollingUpdate == nil {
			kcpTemplateV1Beta1.Spec.Template.Spec.RolloutStrategy.RollingUpdate = &controlplanev1beta1.RollingUpdate{}
		}
		kcpTemplateV1Beta1.Spec.Template.Spec.RolloutStrategy.RollingUpdate.MaxSurge = &kubeadmControlPlaneMaxSurgeIntOrString
		return nil
	}

	kcpTemplate, ok := obj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if ok {
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
func patchKubeadmConfigTemplate(_ context.Context, _ runtime.Object, _ map[string]apiextensionsv1.JSON) error {
	return nil
}

// patchDockerMachineTemplate patches the DockerMachineTemplate.
// It sets the CustomImage to an image for the version in use by the controlPlane or by the MachineDeployment
// the DockerMachineTemplate belongs to.
// NOTE: this patch is not required anymore after the introduction of the kind mapper in kind, however we keep it
// as example of version aware patches.
func patchDockerMachineTemplate(ctx context.Context, obj runtime.Object, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)

	// If the DockerMachineTemplate belongs to the ControlPlane, set the images using the ControlPlane version.
	// NOTE: ControlPlane version might be different than Cluster.version or MachineDeployment's versions;
	// the builtin variables provides the right version to use.
	// NOTE: This works by checking the existence of a builtin variable that exists only for templates linked to the ControlPlane.
	cpVersion, err := topologymutation.GetStringVariable(templateVariables, "builtin.controlPlane.version")
	if err != nil && !topologymutation.IsNotFoundError(err) {
		return errors.Wrap(err, "could not set customImage to control plane dockerMachineTemplate")
	}

	// if found
	if err == nil {
		semVer, err := semver.ParseTolerant(cpVersion)
		if err != nil {
			return errors.Wrap(err, "could not parse control plane version")
		}
		kindMapping := kind.GetMapping(semVer, "")

		log.Info(fmt.Sprintf("Setting control plane custom image to %q", kindMapping.Image))

		dockerMachineTemplateV1Beta1, ok := obj.(*infrav1beta1.DockerMachineTemplate)
		if ok {
			dockerMachineTemplateV1Beta1.Spec.Template.Spec.CustomImage = kindMapping.Image
		}

		dockerMachineTemplate, ok := obj.(*infrav1.DockerMachineTemplate)
		if ok {
			dockerMachineTemplate.Spec.Template.Spec.CustomImage = kindMapping.Image
		}
		// return early if we have successfully patched a control plane dockerMachineTemplate
		return nil
	}

	// If the DockerMachineTemplate belongs to a MachineDeployment, set the images the MachineDeployment version.
	// NOTE: MachineDeployment version might be different from Cluster.version or other MachineDeployment's versions;
	// the builtin variables provides the right version to use.
	// NOTE: This works by checking the existence of a builtin variable that exists only for templates linked to MachineDeployments.
	mdVersion, err := topologymutation.GetStringVariable(templateVariables, "builtin.machineDeployment.version")
	if err != nil {
		if topologymutation.IsNotFoundError(err) {
			// If the DockerMachineTemplate didn't have variables for either a control plane or a machineDeployment return an error.
			// NOTE: this should never happen because it is enforced by the patch engine.
			return errors.New("no version variables found for DockerMachineTemplate patch")
		}
		return errors.Wrap(err, "could not set customImage to MachineDeployment DockerMachineTemplate")
	}

	semVer, err := semver.ParseTolerant(mdVersion)
	if err != nil {
		return errors.Wrap(err, "could not parse MachineDeployment version")
	}
	kindMapping := kind.GetMapping(semVer, "")

	log.Info(fmt.Sprintf("Setting MachineDeployment customImage to %q", kindMapping.Image))

	dockerMachineTemplateV1Beta1, ok := obj.(*infrav1beta1.DockerMachineTemplate)
	if ok {
		dockerMachineTemplateV1Beta1.Spec.Template.Spec.CustomImage = kindMapping.Image
	}

	dockerMachineTemplate, ok := obj.(*infrav1.DockerMachineTemplate)
	if ok {
		dockerMachineTemplate.Spec.Template.Spec.CustomImage = kindMapping.Image
	}
	return nil
}

// patchDockerMachinePoolTemplate patches the DockerMachinePoolTemplate.
// It sets the CustomImage to an image for the version in use by the MachinePool.
// NOTE: this patch is not required anymore after the introduction of the kind mapper in kind, however we keep it
// as example of version aware patches.
func patchDockerMachinePoolTemplate(ctx context.Context, obj runtime.Object, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)

	// If the DockerMachinePoolTemplate belongs to a MachinePool, set the images the MachinePool version.
	// NOTE: MachinePool version might be different from Cluster.version or other MachinePool's versions;
	// the builtin variables provides the right version to use.
	// NOTE: This works by checking the existence of a builtin variable that exists only for templates linked to MachinePools.
	mpVersion, err := topologymutation.GetStringVariable(templateVariables, "builtin.machinePool.version")
	if err != nil {
		// If the DockerMachinePoolTemplate didn't have variables for a machinePool return an error.
		// NOTE: this should never happen because it is enforced by the patch engine.
		if topologymutation.IsNotFoundError(err) {
			return errors.New("no version variables found for DockerMachinePoolTemplate patch")
		}
		return errors.Wrap(err, "could not set customImage to MachinePool DockerMachinePoolTemplate")
	}

	semVer, err := semver.ParseTolerant(mpVersion)
	if err != nil {
		return errors.Wrap(err, "could not parse MachinePool version")
	}
	kindMapping := kind.GetMapping(semVer, "")

	log.Info(fmt.Sprintf("Setting MachinePool customImage to %q", kindMapping.Image))

	dockerMachinePoolTemplateV1Beta1, ok := obj.(*infraexpv1beta1.DockerMachinePoolTemplate)
	if ok {
		dockerMachinePoolTemplateV1Beta1.Spec.Template.Spec.Template.CustomImage = kindMapping.Image
	}

	dockerMachinePoolTemplate, ok := obj.(*infraexpv1.DockerMachinePoolTemplate)
	if ok {
		dockerMachinePoolTemplate.Spec.Template.Spec.Template.CustomImage = kindMapping.Image
	}

	return nil
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
func (h *ExtensionHandlers) DiscoverVariables(ctx context.Context, _ *runtimehooksv1.DiscoverVariablesRequest, resp *runtimehooksv1.DiscoverVariablesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("DiscoverVariables called")

	resp.Status = runtimehooksv1.ResponseStatusSuccess
	resp.Variables = []clusterv1beta1.ClusterClassVariable{
		{
			Name:     "kubeadmControlPlaneMaxSurge",
			Required: false,
			Schema: clusterv1beta1.VariableSchema{
				OpenAPIV3Schema: clusterv1beta1.JSONSchemaProps{
					Type:        "string",
					Default:     &apiextensionsv1.JSON{Raw: []byte(`""`)},
					Example:     &apiextensionsv1.JSON{Raw: []byte(`"0"`)},
					Description: "kubeadmControlPlaneMaxSurge is the maximum number of control planes that can be scheduled above or under the desired number of control plane machines.",
					XValidations: []clusterv1beta1.ValidationRule{
						{
							Rule:              "self == \"\" || self != \"\"",
							MessageExpression: "'just a test expression, got %s'.format([self])",
						},
					},
				},
			},
		},
		// This variable must be set in the Cluster as it has no default value and is required.
		{
			Name:     "imageRepository",
			Required: true,
			Schema: clusterv1beta1.VariableSchema{
				OpenAPIV3Schema: clusterv1beta1.JSONSchemaProps{
					Type:    "string",
					Example: &apiextensionsv1.JSON{Raw: []byte(`"kindest"`)},
					XMetadata: &clusterv1beta1.VariableSchemaMetadata{
						Labels: map[string]string{
							"objects": "DockerCluster",
						},
						Annotations: map[string]string{
							"description": "Gets set at DockerCluster.Spec.Template.Spec.LoadBalancer.ImageRepository",
						},
					},
				},
			},
			Metadata: clusterv1beta1.ClusterClassVariableMetadata{
				Labels: map[string]string{
					"objects": "DockerCluster",
				},
				Annotations: map[string]string{
					"description": "Gets set at DockerCluster.Spec.Template.Spec.LoadBalancer.ImageRepository",
				},
			},
		},
	}
}

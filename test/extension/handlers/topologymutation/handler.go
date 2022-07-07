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
package topologymutation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/version"
)

// NewHandler returns a new topology mutation Handler.
func NewHandler(scheme *runtime.Scheme) *Handler {
	return &Handler{
		// Add the apiGroups being handled to the decoder
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			infrav1.GroupVersion,
			controlplanev1.GroupVersion,
			bootstrapv1.GroupVersion,
		),
	}
}

var (
	cgroupDriverCgroupfs            = "cgroupfs"
	cgroupDriverPatchVersionCeiling = semver.Version{Major: 1, Minor: 24}
)

// Handler is a topology mutation handler.
type Handler struct {
	decoder runtime.Decoder
}

// GeneratePatches generates patches for the given request.
func (h *Handler) GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("GeneratePatches is called")
	walkTemplates(h.decoder, req, resp, func(obj runtime.Object, variables map[string]apiextensionsv1.JSON, holderRef runtimehooksv1.HolderReference) error {
		holderRefGV, err := schema.ParseGroupVersion(holderRef.APIVersion)
		if err != nil {
			return errors.Wrapf(err, "error generating patches - HolderReference apiVersion %q is not in valid format", holderRef.APIVersion)
		}
		log = log.WithValues(
			"template", logRef{
				Group:   obj.GetObjectKind().GroupVersionKind().Group,
				Version: obj.GetObjectKind().GroupVersionKind().Version,
				Kind:    obj.GetObjectKind().GroupVersionKind().Kind,
			},
			"holder", logRef{
				Group:     holderRefGV.Group,
				Version:   holderRefGV.Version,
				Kind:      holderRef.Kind,
				Namespace: holderRef.Namespace,
				Name:      holderRef.Name,
			},
		)
		log.Info("Patching template")

		switch obj := obj.(type) {
		case *infrav1.DockerClusterTemplate:
			if err := patchDockerClusterTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "error patching DockerClusterTemplate")
				return errors.Wrap(err, "error patching DockerClusterTemplate")
			}
		case *controlplanev1.KubeadmControlPlaneTemplate:
			err := patchKubeadmControlPlaneTemplate(ctx, obj, variables)
			if err != nil {
				log.Error(err, "error patching KubeadmControlPlaneTemplate")
				return errors.Wrapf(err, "error patching KubeadmControlPlaneTemplate")
			}

		case *bootstrapv1.KubeadmConfigTemplate:
			if err := patchKubeadmConfigTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "error patching KubeadmConfigTemplate")
				return errors.Wrap(err, "error patching KubeadmConfigTemplate")
			}
		case *infrav1.DockerMachineTemplate:
			if err := patchDockerMachineTemplate(ctx, obj, variables); err != nil {
				log.Error(err, "error patching DockerMachineTemplate")
				return errors.Wrap(err, "error patching DockerMachineTemplate")
			}
		}
		return nil
	})
}

func patchKubeadmControlPlaneTemplate(ctx context.Context, kcpTemplate *controlplanev1.KubeadmControlPlaneTemplate, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)

	// 1) If the Kubernetes version from builtin.controlPlane.version is below 1.24.0 set "cgroup-driver": "cgroupfs" to
	//    - kubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs
	//    - kubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs
	found, cpVersion, err := getStringValueForVariable(templateVariables, "builtin.controlPlane.version")
	if err != nil {
		return errors.Wrap(err, "could not set cgroup-driver to control plane template kubeletExtraArgs")
	}
	// This is a required variable. Return an error if it's not found.
	if !found {
		return errors.New("could not set cgroup-driver to control plane template kubeletExtraArgs: variable \"builtin.controlPlane.version\" not found")
	}

	controlPlaneVersion, err := version.ParseMajorMinorPatchTolerant(cpVersion)
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
	found, kcpControlPlaneMaxSurge, err := getStringValueForVariable(templateVariables, "kubeadmControlPlaneMaxSurge")
	if err != nil {
		return errors.Wrap(err, "could not set KubeadmControlPlaneTemplate MaxSurge")
	}
	if found {
		// This has to be converted to IntOrString type.
		kubeadmControlPlaneMaxSurgeIntOrString := intstrutil.Parse(kcpControlPlaneMaxSurge)
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

// patchDockerClusterTemplate patches the DockerClusterTemplate.
func patchDockerClusterTemplate(_ context.Context, dockerClusterTemplate *infrav1.DockerClusterTemplate, templateVariables map[string]apiextensionsv1.JSON) error {
	found, lbImageRepo, err := getStringValueForVariable(templateVariables, "lbImageRepository")
	if err != nil {
		return errors.Wrap(err, "could not set DockerClusterTemplate loadBalancer imageRepository")
	}
	if found {
		dockerClusterTemplate.Spec.Template.Spec.LoadBalancer.ImageRepository = lbImageRepo
	}
	return nil
}

func patchKubeadmConfigTemplate(ctx context.Context, k *bootstrapv1.KubeadmConfigTemplate, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)
	// Only patch the customImage if this DockerMachineTemplate belongs to a MachineDeployment with class "default-class"
	found, mdClass, err := getStringValueForVariable(templateVariables, "builtin.machineDeployment.class")
	if err != nil {
		return errors.Wrap(err, "could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs")
	}

	// This is a required variable. Return an error if it's not found.
	if !found {
		return errors.New("could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs: variable \"builtin.machineDeployment.class\" not found")
	}

	if mdClass == "default-worker" {
		// If the Kubernetes version from builtin.machineDeployment.version is below 1.24.0 set "cgroup-driver": "cgroupDriverCgroupfs" to
		//    - InitConfiguration.KubeletExtraArgs
		//    - JoinConfiguration.KubeletExtraArgs
		found, mdVersion, err := getStringValueForVariable(templateVariables, "builtin.machineDeployment.version")
		if err != nil {
			return errors.Wrap(err, "could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs")
		}

		// This is a required variable. Return an error if it's not found.
		if !found {
			return errors.New("could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs: variable \"builtin.machineDeployment.version\" not found")
		}
		machineDeploymentVersion, err := version.ParseMajorMinorPatchTolerant(mdVersion)
		if err != nil {
			return errors.Wrap(err, "could not set cgroup-driver to KubeadmConfigTemplate template kubeletExtraArgs")
		}
		if version.Compare(machineDeploymentVersion, cgroupDriverPatchVersionCeiling) == -1 {
			log.Info(fmt.Sprintf("Setting KubeadmConfigTemplate cgroup-driver to %q", cgroupDriverCgroupfs))
			// Set the cgroupDriver in the InitConfiguration.
			if k.Spec.Template.Spec.InitConfiguration == nil {
				k.Spec.Template.Spec.InitConfiguration = &bootstrapv1.InitConfiguration{}
			}
			if k.Spec.Template.Spec.InitConfiguration.NodeRegistration.KubeletExtraArgs == nil {
				k.Spec.Template.Spec.InitConfiguration.NodeRegistration.KubeletExtraArgs = map[string]string{}
			}
			k.Spec.Template.Spec.InitConfiguration.NodeRegistration.KubeletExtraArgs["cgroup-driver"] = cgroupDriverCgroupfs

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

func patchDockerMachineTemplate(ctx context.Context, dockerMachineTemplate *infrav1.DockerMachineTemplate, templateVariables map[string]apiextensionsv1.JSON) error {
	log := ctrl.LoggerFrom(ctx)
	found, cpVersion, err := getStringValueForVariable(templateVariables, "builtin.controlPlane.version")
	if err != nil {
		return errors.Wrap(err, "could not set customImage to control plane dockerMachineTemplate")
	}
	if found {
		_, err := version.ParseMajorMinorPatchTolerant(cpVersion)
		if err != nil {
			return errors.Wrap(err, "could not parse control plane version")
		}
		customImage := fmt.Sprintf("kindest/node:%s", strings.ReplaceAll(cpVersion, "+", "_"))
		log.Info(fmt.Sprintf("Setting MachineDeployment custom image to %q", customImage))
		dockerMachineTemplate.Spec.Template.Spec.CustomImage = customImage
		// return early if we have successfully patched a control plane dockerMachineTemplate
		return nil
	}

	// If the DockerMachineTemplate does not belong to a ControlPlane check if it belongs to a MachineDeployment
	found, mdVersion, err := getStringValueForVariable(templateVariables, "builtin.machineDeployment.version")
	if err != nil {
		return errors.Wrap(err, "could not set customImage to MachineDeployment DockerMachineTemplate")
	}
	if found {
		_, err := version.ParseMajorMinorPatchTolerant(mdVersion)
		if err != nil {
			return errors.Wrap(err, "could not parse MachineDeployment version")
		}
		customImage := fmt.Sprintf("kindest/node:%s", strings.ReplaceAll(mdVersion, "+", "_"))
		log.Info(fmt.Sprintf("Setting MachineDeployment customImage to %q", customImage))
		dockerMachineTemplate.Spec.Template.Spec.CustomImage = customImage
		return nil
	}
	// If the Docker Machine didn't have variables for either a control plane or a machineDeployment return an error.
	return errors.New("no version variables found for DockerMachineTemplate patch")
}

// ValidateTopology returns a function that validates the given request.
func (h *Handler) ValidateTopology(ctx context.Context, _ *runtimehooksv1.ValidateTopologyRequest, resp *runtimehooksv1.ValidateTopologyResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("ValidateTopology called")

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// getStringValueForVariable Get the variable value as JSON string.
func getStringValueForVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (bool, string, error) {
	value, err := patchvariables.GetVariableValue(templateVariables, variableName)
	if err != nil {
		if patchvariables.IsNotFoundError(err) {
			return false, "", nil
		}
		return false, "", err
	}
	// Unquote the JSON string.
	stringValue, err := strconv.Unquote(string(value.Raw))
	if err != nil {
		return true, "", err
	}
	return true, stringValue, nil
}

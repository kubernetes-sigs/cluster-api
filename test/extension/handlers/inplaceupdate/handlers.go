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

// Package inplaceupdate contains the handlers for the in-place update hooks.
//
// The implementation of the handlers is specifically designed for Cluster API E2E test use cases.
// When implementing custom RuntimeExtension, it is only required to expose HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
package inplaceupdate

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/pkg/errors"
	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// ExtensionHandlers provides a common struct shared across the in-place update hook handlers.
type ExtensionHandlers struct {
	decoder runtime.Decoder
	client  client.Client
	state   sync.Map
}

// NewExtensionHandlers returns a new ExtensionHandlers for the in-place update hook handlers.
func NewExtensionHandlers(client client.Client) *ExtensionHandlers {
	scheme := runtime.NewScheme()
	_ = infrav1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	return &ExtensionHandlers{
		client: client,
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			infrav1.GroupVersion,
			bootstrapv1.GroupVersion,
		),
	}
}

// canUpdateMachineSpec declares that this extension can update:
// * MachineSpec.FailureDomain
// * MachineSpec.Version.
func canUpdateMachineSpec(current, desired *clusterv1.MachineSpec) {
	if current.FailureDomain != desired.FailureDomain {
		current.FailureDomain = desired.FailureDomain
	}
	// TBD if we should keep this, but for now using it to test KCP machinery as this
	// is the only field on Machine that KCP will try to roll out in-place.
	if current.Version != desired.Version {
		current.Version = desired.Version
	}
}

// canUpdateKubeadmConfigSpec declares that this extension can update:
// * KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageTag
// * KubeadmConfigSpec.Files.
func canUpdateKubeadmConfigSpec(current, desired *bootstrapv1.KubeadmConfigSpec) {
	if current.ClusterConfiguration.Etcd.Local.ImageTag != desired.ClusterConfiguration.Etcd.Local.ImageTag {
		current.ClusterConfiguration.Etcd.Local.ImageTag = desired.ClusterConfiguration.Etcd.Local.ImageTag
	}
	if !reflect.DeepEqual(current.Files, desired.Files) {
		current.Files = desired.Files
	}
}

// canUpdateDockerMachineSpec declares that this extension can update:
// * DockerMachineSpec.BootstrapTimeout.
func canUpdateDockerMachineSpec(current, desired *infrav1.DockerMachineSpec) {
	if current.BootstrapTimeout != desired.BootstrapTimeout {
		current.BootstrapTimeout = desired.BootstrapTimeout
	}
}

// DoCanUpdateMachine implements the CanUpdateMachine hook.
func (h *ExtensionHandlers) DoCanUpdateMachine(ctx context.Context, req *runtimehooksv1.CanUpdateMachineRequest, resp *runtimehooksv1.CanUpdateMachineResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(&req.Desired.Machine))
	log.Info("CanUpdateMachine is called")

	currentMachine, desiredMachine,
		currentBootstrapConfig, desiredBootstrapConfig,
		currentInfraMachine, desiredInfraMachine, err := h.getObjectsFromCanUpdateMachineRequest(req)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}

	// Declare changes that this Runtime Extension can update in-place.

	// Machine
	canUpdateMachineSpec(&currentMachine.Spec, &desiredMachine.Spec)

	// BootstrapConfig (we can only update KubeadmConfigs)
	currentKubeadmConfig, isCurrentKubeadmConfig := currentBootstrapConfig.(*bootstrapv1.KubeadmConfig)
	desiredKubeadmConfig, isDesiredKubeadmConfig := desiredBootstrapConfig.(*bootstrapv1.KubeadmConfig)
	if isCurrentKubeadmConfig && isDesiredKubeadmConfig {
		canUpdateKubeadmConfigSpec(&currentKubeadmConfig.Spec, &desiredKubeadmConfig.Spec)
	}

	// InfraMachine (we can only update DockerMachines)
	currentDockerMachine, isCurrentDockerMachine := currentInfraMachine.(*infrav1.DockerMachine)
	desiredDockerMachine, isDesiredDockerMachine := desiredInfraMachine.(*infrav1.DockerMachine)
	if isCurrentDockerMachine && isDesiredDockerMachine {
		canUpdateDockerMachineSpec(&currentDockerMachine.Spec, &desiredDockerMachine.Spec)
	}

	if err := h.computeCanUpdateMachineResponse(req, resp, currentMachine, currentBootstrapConfig, currentInfraMachine); err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoCanUpdateMachineSet implements the CanUpdateMachineSet hook.
func (h *ExtensionHandlers) DoCanUpdateMachineSet(ctx context.Context, req *runtimehooksv1.CanUpdateMachineSetRequest, resp *runtimehooksv1.CanUpdateMachineSetResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("MachineSet", klog.KObj(&req.Desired.MachineSet))
	log.Info("CanUpdateMachineSet is called")

	currentMachineSet, desiredMachineSet,
		currentBootstrapConfigTemplate, desiredBootstrapConfigTemplate,
		currentInfraMachineTemplate, desiredInfraMachineTemplate, err := h.getObjectsFromCanUpdateMachineSetRequest(req)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}

	// Declare changes that this Runtime Extension can update in-place.

	// Machine
	canUpdateMachineSpec(&currentMachineSet.Spec.Template.Spec, &desiredMachineSet.Spec.Template.Spec)

	// BootstrapConfig (we can only update KubeadmConfigs)
	currentKubeadmConfigTemplate, isCurrentKubeadmConfigTemplate := currentBootstrapConfigTemplate.(*bootstrapv1.KubeadmConfigTemplate)
	desiredKubeadmConfigTemplate, isDesiredKubeadmConfigTemplate := desiredBootstrapConfigTemplate.(*bootstrapv1.KubeadmConfigTemplate)
	if isCurrentKubeadmConfigTemplate && isDesiredKubeadmConfigTemplate {
		canUpdateKubeadmConfigSpec(&currentKubeadmConfigTemplate.Spec.Template.Spec, &desiredKubeadmConfigTemplate.Spec.Template.Spec)
	}

	// InfraMachine (we can only update DockerMachines)
	currentDockerMachineTemplate, isCurrentDockerMachineTemplate := currentInfraMachineTemplate.(*infrav1.DockerMachineTemplate)
	desiredDockerMachineTemplate, isDesiredDockerMachineTemplate := desiredInfraMachineTemplate.(*infrav1.DockerMachineTemplate)
	if isCurrentDockerMachineTemplate && isDesiredDockerMachineTemplate {
		canUpdateDockerMachineSpec(&currentDockerMachineTemplate.Spec.Template.Spec, &desiredDockerMachineTemplate.Spec.Template.Spec)
	}

	if err := h.computeCanUpdateMachineSetResponse(req, resp, currentMachineSet, currentBootstrapConfigTemplate, currentInfraMachineTemplate); err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoUpdateMachine implements the UpdateMachine hook.
// Note: We are intentionally not actually applying any in-place changes we are just faking them,
// which is good enough for test purposes.
func (h *ExtensionHandlers) DoUpdateMachine(ctx context.Context, req *runtimehooksv1.UpdateMachineRequest, resp *runtimehooksv1.UpdateMachineResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(&req.Desired.Machine))
	log.Info("UpdateMachine is called")
	defer func() {
		log.Info("UpdateMachine response", "Machine", klog.KObj(&req.Desired.Machine), "status", resp.Status, "message", resp.Message, "retryAfterSeconds", resp.RetryAfterSeconds)
	}()

	key := klog.KObj(&req.Desired.Machine).String()

	// Note: We are intentionally not actually applying any in-place changes we are just faking them,
	// which is good enough for test purposes.
	if firstTimeCalled, ok := h.state.Load(key); ok {
		if time.Since(firstTimeCalled.(time.Time)) > 20*time.Second {
			h.state.Delete(key)
			resp.Status = runtimehooksv1.ResponseStatusSuccess
			resp.Message = "In-place update is done"
			resp.RetryAfterSeconds = 0
			return
		}
	} else {
		h.state.Store(key, time.Now())
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
	resp.Message = "In-place update still in progress"
	resp.RetryAfterSeconds = 5
}

func (h *ExtensionHandlers) getObjectsFromCanUpdateMachineRequest(req *runtimehooksv1.CanUpdateMachineRequest) (*clusterv1.Machine, *clusterv1.Machine, runtime.Object, runtime.Object, runtime.Object, runtime.Object, error) { //nolint:gocritic // accepting high number of return parameters for now
	currentMachine := req.Current.Machine.DeepCopy()
	desiredMachine := req.Desired.Machine.DeepCopy()
	currentBootstrapConfig, _, err := h.decoder.Decode(req.Current.BootstrapConfig.Raw, nil, req.Current.BootstrapConfig.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	desiredBootstrapConfig, _, err := h.decoder.Decode(req.Desired.BootstrapConfig.Raw, nil, req.Desired.BootstrapConfig.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	currentInfraMachine, _, err := h.decoder.Decode(req.Current.InfrastructureMachine.Raw, nil, req.Current.InfrastructureMachine.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	desiredInfraMachine, _, err := h.decoder.Decode(req.Desired.InfrastructureMachine.Raw, nil, req.Desired.InfrastructureMachine.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	return currentMachine, desiredMachine, currentBootstrapConfig, desiredBootstrapConfig, currentInfraMachine, desiredInfraMachine, nil
}

func (h *ExtensionHandlers) computeCanUpdateMachineResponse(req *runtimehooksv1.CanUpdateMachineRequest, resp *runtimehooksv1.CanUpdateMachineResponse, currentMachine *clusterv1.Machine, currentBootstrapConfig, currentInfraMachine runtime.Object) error {
	marshalledCurrentMachine, err := json.Marshal(req.Current.Machine)
	if err != nil {
		return err
	}
	machinePatch, err := createJSONPatch(marshalledCurrentMachine, currentMachine)
	if err != nil {
		return err
	}
	bootstrapConfigPatch, err := createJSONPatch(req.Current.BootstrapConfig.Raw, currentBootstrapConfig)
	if err != nil {
		return err
	}
	infraMachinePatch, err := createJSONPatch(req.Current.InfrastructureMachine.Raw, currentInfraMachine)
	if err != nil {
		return err
	}

	resp.MachinePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     machinePatch,
	}
	resp.BootstrapConfigPatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     bootstrapConfigPatch,
	}
	resp.InfrastructureMachinePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     infraMachinePatch,
	}
	return nil
}

func (h *ExtensionHandlers) getObjectsFromCanUpdateMachineSetRequest(req *runtimehooksv1.CanUpdateMachineSetRequest) (*clusterv1.MachineSet, *clusterv1.MachineSet, runtime.Object, runtime.Object, runtime.Object, runtime.Object, error) { //nolint:gocritic // accepting high number of return parameters for now
	currentMachineSet := req.Current.MachineSet.DeepCopy()
	desiredMachineSet := req.Desired.MachineSet.DeepCopy()
	currentBootstrapConfigTemplate, _, err := h.decoder.Decode(req.Current.BootstrapConfigTemplate.Raw, nil, req.Current.BootstrapConfigTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	desiredBootstrapConfigTemplate, _, err := h.decoder.Decode(req.Desired.BootstrapConfigTemplate.Raw, nil, req.Desired.BootstrapConfigTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	currentInfraMachineTemplate, _, err := h.decoder.Decode(req.Current.InfrastructureMachineTemplate.Raw, nil, req.Current.InfrastructureMachineTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	desiredInfraMachineTemplate, _, err := h.decoder.Decode(req.Desired.InfrastructureMachineTemplate.Raw, nil, req.Desired.InfrastructureMachineTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	return currentMachineSet, desiredMachineSet, currentBootstrapConfigTemplate, desiredBootstrapConfigTemplate, currentInfraMachineTemplate, desiredInfraMachineTemplate, nil
}

func (h *ExtensionHandlers) computeCanUpdateMachineSetResponse(req *runtimehooksv1.CanUpdateMachineSetRequest, resp *runtimehooksv1.CanUpdateMachineSetResponse, currentMachineSet *clusterv1.MachineSet, currentBootstrapConfigTemplate, currentInfraMachineTemplate runtime.Object) error {
	marshalledCurrentMachineSet, err := json.Marshal(req.Current.MachineSet)
	if err != nil {
		return err
	}
	machineSetPatch, err := createJSONPatch(marshalledCurrentMachineSet, currentMachineSet)
	if err != nil {
		return err
	}
	bootstrapConfigTemplatePatch, err := createJSONPatch(req.Current.BootstrapConfigTemplate.Raw, currentBootstrapConfigTemplate)
	if err != nil {
		return err
	}
	infraMachineTemplatePatch, err := createJSONPatch(req.Current.InfrastructureMachineTemplate.Raw, currentInfraMachineTemplate)
	if err != nil {
		return err
	}

	resp.MachineSetPatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     machineSetPatch,
	}
	resp.BootstrapConfigTemplatePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     bootstrapConfigTemplatePatch,
	}
	resp.InfrastructureMachineTemplatePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     infraMachineTemplatePatch,
	}
	return nil
}

// createJSONPatch creates a RFC 6902 JSON patch from the original and the modified object.
func createJSONPatch(marshalledOriginal []byte, modified runtime.Object) ([]byte, error) {
	// TODO: avoid producing patches for status (although they will be ignored by the KCP / MD controllers anyway)
	marshalledModified, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Errorf("failed to marshal modified object: %v", err)
	}

	patch, err := jsonpatch.CreatePatch(marshalledOriginal, marshalledModified)
	if err != nil {
		return nil, errors.Errorf("failed to create patch: %v", err)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, errors.Errorf("failed to marshal patch: %v", err)
	}

	return patchBytes, nil
}

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

// Package inplaceupdate contains the handlers for the in-place Kubernetes version update hooks.
//
// The implementation of the handlers is specifically designed for Cluster API E2E tests use cases,
// focusing on simulating realistic Kubernetes version upgrades through the in-place update mechanism.
// When implementing custom RuntimeExtension, it is only required to expose HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
package inplaceupdate

import (
	"context"
	"encoding/json"
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
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

type ExtensionHandlers struct {
	decoder runtime.Decoder
	client  client.Client
	state   sync.Map
}

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

func (h *ExtensionHandlers) DoCanUpdateMachine(ctx context.Context, req *runtimehooksv1.CanUpdateMachineRequest, resp *runtimehooksv1.CanUpdateMachineResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("CanUpdateMachine is called", "Machine", klog.KObj(&req.Desired.Machine))

	// Create copies of current.
	modifiedCurrentMachine := req.Current.Machine.DeepCopy()
	modifiedCurrentBootstrapConfig, _, err := h.decoder.Decode(req.Current.BootstrapConfig.Raw, nil, req.Current.BootstrapConfig.Object)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	desiredBootstrapConfig, _, err := h.decoder.Decode(req.Desired.BootstrapConfig.Raw, nil, req.Desired.BootstrapConfig.Object)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	modifiedCurrentInfraMachine, _, err := h.decoder.Decode(req.Current.InfrastructureMachine.Raw, nil, req.Current.InfrastructureMachine.Object)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	desiredInfraMachine, _, err := h.decoder.Decode(req.Desired.InfrastructureMachine.Raw, nil, req.Desired.InfrastructureMachine.Object)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}

	// Declare changes that this Runtime Extension can update in-place.

	// Machine
	if modifiedCurrentMachine.Spec.FailureDomain != req.Desired.Machine.Spec.FailureDomain {
		modifiedCurrentMachine.Spec.FailureDomain = req.Desired.Machine.Spec.FailureDomain
	}
	// TODO: TBD if we should really keep this, but for now using it to test KCP machinery
	if modifiedCurrentMachine.Spec.Version != req.Desired.Machine.Spec.Version {
		modifiedCurrentMachine.Spec.Version = req.Desired.Machine.Spec.Version
	}

	// BootstrapConfig
	switch current := modifiedCurrentBootstrapConfig.(type) {
	case *bootstrapv1.KubeadmConfig:
		desired := desiredBootstrapConfig.(*bootstrapv1.KubeadmConfig)
		if current.Spec.ClusterConfiguration.Etcd.Local.ImageTag != desired.Spec.ClusterConfiguration.Etcd.Local.ImageTag {
			current.Spec.ClusterConfiguration.Etcd.Local.ImageTag = desired.Spec.ClusterConfiguration.Etcd.Local.ImageTag
		}
	default:
		// Nothing to do. If we can't in-place update a BootstrapConfig we simply don't make any changes.
	}

	// InfraMachine
	// BootstrapConfig
	switch current := modifiedCurrentInfraMachine.(type) {
	case *infrav1.DockerMachine:
		desired := desiredInfraMachine.(*infrav1.DockerMachine)
		if current.Spec.BootstrapTimeout != desired.Spec.BootstrapTimeout {
			current.Spec.BootstrapTimeout = desired.Spec.BootstrapTimeout
		}
		// TODO: TBD if we should really keep this, but for now using it to test KCP machinery
		if current.Spec.CustomImage != desired.Spec.CustomImage {
			current.Spec.CustomImage = desired.Spec.CustomImage
		}
	default:
		// Nothing to do. If we can't in-place update a BootstrapConfig we simply don't make any changes.
	}

	// Compute patches.
	marshalledCurrentMachine, err := json.Marshal(req.Current.Machine)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	machinePatch, err := createJSONPatch(marshalledCurrentMachine, modifiedCurrentMachine)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	bootstrapConfigPatch, err := createJSONPatch(req.Current.BootstrapConfig.Raw, modifiedCurrentBootstrapConfig)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
	}
	infraMachinePatch, err := createJSONPatch(req.Current.InfrastructureMachine.Raw, modifiedCurrentInfraMachine)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()
		return
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
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

func (h *ExtensionHandlers) DoCanUpdateMachineSet(_ context.Context, _ *runtimehooksv1.CanUpdateMachineSetRequest, resp *runtimehooksv1.CanUpdateMachineSetResponse) {
	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

func (h *ExtensionHandlers) DoUpdateMachine(ctx context.Context, req *runtimehooksv1.UpdateMachineRequest, resp *runtimehooksv1.UpdateMachineResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("UpdateMachine is called", "Machine", klog.KObj(&req.Desired.Machine))
	defer func() {
		log.Info("UpdateMachine response", "Machine", klog.KObj(&req.Desired.Machine), "status", resp.Status, "message", resp.Message, "retryAfterSeconds", resp.RetryAfterSeconds)
	}()

	key := klog.KObj(&req.Desired.Machine).String()

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
	return
}

// createJSONPatch creates a RFC 6902 JSON patch from the original and the modified object.
func createJSONPatch(marshalledOriginal []byte, modified runtime.Object) ([]byte, error) {
	// TODO: avoid producing patches for status
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

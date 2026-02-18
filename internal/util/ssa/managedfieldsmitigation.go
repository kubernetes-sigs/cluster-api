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

package ssa

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/util"
)

const beforeFirstApplyManager = "before-first-apply"

var kcpScheme = runtime.NewScheme()

func init() {
	_ = controlplanev1.AddToScheme(kcpScheme)
	_ = controlplanev1beta1.AddToScheme(kcpScheme)
}

// MitigateManagedFieldsIssue mitigates the managedField apiserver issue,
// where apiserver drops managedFields of an object under certain circumstances:
// https://github.com/kubernetes/kubernetes/issues/136919
//
// It mitigates the issue by removing the before-first-apply entry (if it exists)
// and by ensuring that there is always a (minimal) entry for fieldManager to avoid
// that the apiserver adds a new before-first-apply entry during the next SSA call.
// NOTE: The fieldManager passed in should be used by subsequent SSA calls to increase
// the chances that the managedField entry gets cleaned up soon.
//
// A subsequent regular SSA call by our controller will fully restore managedFields.
//
// This addresses the issue in most scenarios, but it also means that an SSA call
// directly after we added the minimal entry won't be able to unset fields in general.
//
// Because we know that this issue is especially problematic with KCP extraArgs
// we added special handling for KCP to add extraArgs managedField entries based on
// the current KCP object. This allows to unset extraArgs on the next SSA call even
// directly after we just mitigated the issue.
//
// The following cases have to be handled:
// Case 0: no-op
//   - [fieldManager]                                          => [fieldManager]
//
// Case 1: no managedFields
//   - []                                                      => [fieldManager]
//
// Case 2: before-first-apply entry
//   - a: [before-first-apply]                                 => [fieldManager]
//   - b: [before-first-apply,other managers ...]              => [fieldManager,other managers ...]
//   - c: [before-first-apply,fieldManager,other managers ...] => [fieldManager,other managers ...]
//
// We should keep this logic until the issue has been fixed in apiserver and our minimal supported
// Kubernetes version has the fix.
func MitigateManagedFieldsIssue(ctx context.Context, c client.Client, obj client.Object, fieldManager string) (bool, error) {
	if util.IsNil(obj) {
		// Return if object is nil.
		return false, nil
	}

	if len(obj.GetManagedFields()) > 0 && !slices.ContainsFunc(obj.GetManagedFields(), isManager(beforeFirstApplyManager)) {
		// Return if object has managedFields and no before-first-apply entry.
		return false, nil
	}

	log := ctrl.LoggerFrom(ctx)
	objGVK, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return false, errors.Wrapf(err, "failed to mitigate managedFields issue")
	}

	// Remove before-first-apply entry if it exists.
	managedFields := slices.DeleteFunc(obj.GetManagedFields(), isManager(beforeFirstApplyManager))

	// Add fieldManager entry if it does not exist.
	if !slices.ContainsFunc(obj.GetManagedFields(), isManager(fieldManager)) {
		fieldsV1, err := computeManagedFields(obj, objGVK)
		if err != nil {
			return false, errors.Wrapf(err, "failed to mitigate managedFields issue: failed to compute managedFields entry")
		}
		managedFields = append(managedFields, metav1.ManagedFieldsEntry{
			Manager:    fieldManager,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: objGVK.GroupVersion().String(),
			Time:       ptr.To(metav1.Now()),
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: fieldsV1},
		})
	}

	// Create a patch to update only managedFields.
	// Include resourceVersion to avoid race conditions.
	jsonPatch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/metadata/managedFields",
			"value": managedFields,
		},
		{
			"op":    "replace",
			"path":  "/metadata/resourceVersion",
			"value": obj.GetResourceVersion(),
		},
	}
	patch, err := json.Marshal(jsonPatch)
	if err != nil {
		return false, errors.Wrap(err, "failed to mitigate managedFields issue: failed to marshal patch for managedFields entry")
	}

	log.Info("Fixing up managedFields to mitigate kube-apiserver managedFields issue", objGVK.Kind, klog.KObj(obj))
	if err := c.Patch(ctx, obj, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return false, errors.Wrapf(err, "failed to mitigate managedFields issue: failed to patch %s %s", objGVK.Kind, klog.KObj(obj))
	}

	return true, nil
}

func computeManagedFields(obj client.Object, objGVK schema.GroupVersionKind) ([]byte, error) {
	managedFieldSet := fieldpath.NewSet()

	switch {
	// Compute managedField entry with extraArgs for v1beta2 KubeadmControlPlane.
	case objGVK.Kind == "KubeadmControlPlane" && objGVK.Version == controlplanev1.GroupVersion.Version:
		kcp := &controlplanev1.KubeadmControlPlane{}
		if err := kcpScheme.Convert(obj, kcp, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert object to KubeadmControlPlane")
		}

		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs,
			"spec", "kubeadmConfigSpec", "clusterConfiguration", "apiServer", "extraArgs")
		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager.ExtraArgs,
			"spec", "kubeadmConfigSpec", "clusterConfiguration", "controllerManager", "extraArgs")
		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler.ExtraArgs,
			"spec", "kubeadmConfigSpec", "clusterConfiguration", "scheduler", "extraArgs")
		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ExtraArgs,
			"spec", "kubeadmConfigSpec", "clusterConfiguration", "etcd", "local", "extraArgs")
		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs,
			"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs")
		addArgs(managedFieldSet, kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs,
			"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs")
	// Compute managedField entry with extraArgs for v1beta1 KubeadmControlPlane.
	case objGVK.Kind == "KubeadmControlPlane" && objGVK.Version == controlplanev1beta1.GroupVersion.Version:
		kcp := &controlplanev1beta1.KubeadmControlPlane{}
		if err := kcpScheme.Convert(obj, kcp, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert object to KubeadmControlPlane")
		}

		if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
			addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer.ExtraArgs,
				"spec", "kubeadmConfigSpec", "clusterConfiguration", "apiServer", "extraArgs")
			addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager.ExtraArgs,
				"spec", "kubeadmConfigSpec", "clusterConfiguration", "controllerManager", "extraArgs")
			addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler.ExtraArgs,
				"spec", "kubeadmConfigSpec", "clusterConfiguration", "scheduler", "extraArgs")
			if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
				addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ExtraArgs,
					"spec", "kubeadmConfigSpec", "clusterConfiguration", "etcd", "local", "extraArgs")
			}
		}
		if kcp.Spec.KubeadmConfigSpec.InitConfiguration != nil {
			addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.InitConfiguration.NodeRegistration.KubeletExtraArgs,
				"spec", "kubeadmConfigSpec", "initConfiguration", "nodeRegistration", "kubeletExtraArgs")
		}
		if kcp.Spec.KubeadmConfigSpec.JoinConfiguration != nil {
			addArgsV1Beta1(managedFieldSet, kcp.Spec.KubeadmConfigSpec.JoinConfiguration.NodeRegistration.KubeletExtraArgs,
				"spec", "kubeadmConfigSpec", "joinConfiguration", "nodeRegistration", "kubeletExtraArgs")
		}
	}

	if managedFieldSet.Empty() {
		// Add a seeding managedFields entry to ensure apiserver does not add a before-first-apply
		// managedFields entry during the next SSA call.
		// More specifically, if an existing object doesn't have managedFields when applying the next SSA
		// the API server creates a before-first-apply entry. This entry then ends up acting as a co-ownership
		// and we want to prevent this (because co-ownership prevents us from removing fields).
		// NOTE: fieldV1Map cannot be empty, so we add metadata.name which will be cleaned up at the first
		//       SSA patch of the same fieldManager.
		managedFieldSet.Insert(fieldpath.MakePathOrDie("metadata", "name"))
	}

	fieldsV1, err := managedFieldSet.ToJSON()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal managedFields entry")
	}

	return fieldsV1, nil
}

func isManager(manager string) func(entry metav1.ManagedFieldsEntry) bool {
	return func(entry metav1.ManagedFieldsEntry) bool {
		return entry.Manager == manager
	}
}

func addArgs(managedFieldSet *fieldpath.Set, args []bootstrapv1.Arg, fields ...any) {
	if len(args) == 0 {
		return
	}

	for _, arg := range args {
		managedFieldSet.Insert(fieldpath.MakePathOrDie(append(fields, fieldpath.KeyByFields("name", arg.Name, "value", ptr.Deref(arg.Value, "")))...))
		managedFieldSet.Insert(fieldpath.MakePathOrDie(append(fields, fieldpath.KeyByFields("name", arg.Name, "value", ptr.Deref(arg.Value, "")), "name")...))
		managedFieldSet.Insert(fieldpath.MakePathOrDie(append(fields, fieldpath.KeyByFields("name", arg.Name, "value", ptr.Deref(arg.Value, "")), "value")...))
	}
}

func addArgsV1Beta1(managedFieldSet *fieldpath.Set, args map[string]string, fields ...any) {
	if len(args) == 0 {
		return
	}

	for argName := range args {
		managedFieldSet.Insert(fieldpath.MakePathOrDie(append(fields, argName)...))
	}
}

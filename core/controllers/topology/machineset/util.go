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

package machineset

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
)

// GetMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func GetMachineSetsForDeployment(ctx context.Context, c client.Reader, md types.NamespacedName) ([]*clusterv1.MachineSet, error) {
	// List MachineSets based on the MachineDeployment label.
	msList := &clusterv1.MachineSetList{}
	if err := c.List(ctx, msList,
		client.InNamespace(md.Namespace), client.MatchingLabels{clusterv1.MachineDeploymentNameLabel: md.Name}); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineSets for MachineDeployment/%s", md.Name)
	}

	// Copy the MachineSets to an array of MachineSet pointers, to avoid MachineSet copying later.
	res := make([]*clusterv1.MachineSet, 0, len(msList.Items))
	for i := range msList.Items {
		res = append(res, &msList.Items[i])
	}
	return res, nil
}

// CalculateTemplatesInUse returns all templates referenced in non-deleting MachineDeployment and MachineSets.
func CalculateTemplatesInUse(md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) (map[string]bool, error) {
	templatesInUse := map[string]bool{}

	// Templates of the MachineSet are still in use if the MachineSet is not in deleting state.
	for _, ms := range msList {
		if !ms.DeletionTimestamp.IsZero() {
			continue
		}

		bootstrapRef := ms.Spec.Template.Spec.Bootstrap.ConfigRef
		infrastructureRef := ms.Spec.Template.Spec.InfrastructureRef
		addTemplateRef(templatesInUse, bootstrapRef, infrastructureRef)
	}

	// If MachineDeployment has already been deleted or still exists and is in deleting state, then there
	// are no templates referenced in the MachineDeployment which are still in use, so let's return here.
	if md == nil || !md.DeletionTimestamp.IsZero() {
		return templatesInUse, nil
	}

	//  Otherwise, the templates of the MachineDeployment are still in use.
	bootstrapRef := md.Spec.Template.Spec.Bootstrap.ConfigRef
	infrastructureRef := md.Spec.Template.Spec.InfrastructureRef
	addTemplateRef(templatesInUse, bootstrapRef, infrastructureRef)
	return templatesInUse, nil
}

// DeleteTemplateIfUnused deletes the template (ref), if it is not in use (i.e. in templatesInUse).
func DeleteTemplateIfUnused(ctx context.Context, c client.Client, templatesInUse map[string]bool, ref clusterv1.ContractVersionedObjectReference, namespace string) error {
	// If ref is nil, do nothing (this can happen, because bootstrap templates are optional).
	if !ref.IsDefined() {
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues(ref.Kind, klog.KRef(namespace, ref.Name))

	refID := templateRefID(ref)

	// If the template is still in use, do nothing.
	if templatesInUse[refID] {
		log.V(3).Info(fmt.Sprintf("Not deleting %s, because it's still in use", ref.Kind))
		return nil
	}

	apiVersion, err := contract.GetAPIVersion(ctx, c, ref.GroupKind())
	if err != nil {
		return errors.Wrapf(err, "failed to delete %s %s", ref.Kind, klog.KRef(namespace, ref.Name))
	}
	deleteRef := &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
	}

	log.Info(fmt.Sprintf("Deleting %s", ref.Kind))
	if err := external.Delete(ctx, c, deleteRef); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s %s", ref.Kind, klog.KRef(namespace, ref.Name))
	}
	return nil
}

// addTemplateRef adds the refs to the refMap with the templateRefID as key.
func addTemplateRef(refMap map[string]bool, refs ...clusterv1.ContractVersionedObjectReference) {
	for _, ref := range refs {
		if ref.IsDefined() {
			refMap[templateRefID(ref)] = true
		}
	}
}

// templateRefID returns the templateRefID of a ObjectReference in the format: g/k/name.
// Note: We don't include the version as references with different versions should be treated as equal.
func templateRefID(ref clusterv1.ContractVersionedObjectReference) string {
	if !ref.IsDefined() {
		return ""
	}

	return fmt.Sprintf("%s/%s/%s", ref.APIGroup, ref.Kind, ref.Name)
}

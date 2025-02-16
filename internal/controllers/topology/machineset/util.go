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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
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
		infrastructureRef := &ms.Spec.Template.Spec.InfrastructureRef
		if err := addTemplateRef(templatesInUse, bootstrapRef, infrastructureRef); err != nil {
			return nil, errors.Wrapf(err, "failed to add templates of MachineSet %s to templatesInUse", klog.KObj(ms))
		}
	}

	// If MachineDeployment has already been deleted or still exists and is in deleting state, then there
	// are no templates referenced in the MachineDeployment which are still in use, so let's return here.
	if md == nil || !md.DeletionTimestamp.IsZero() {
		return templatesInUse, nil
	}

	//  Otherwise, the templates of the MachineDeployment are still in use.
	bootstrapRef := md.Spec.Template.Spec.Bootstrap.ConfigRef
	infrastructureRef := &md.Spec.Template.Spec.InfrastructureRef
	if err := addTemplateRef(templatesInUse, bootstrapRef, infrastructureRef); err != nil {
		return nil, errors.Wrapf(err, "failed to add templates of MachineDeployment %s to templatesInUse", klog.KObj(md))
	}
	return templatesInUse, nil
}

// DeleteTemplateIfUnused deletes the template (ref), if it is not in use (i.e. in templatesInUse).
func DeleteTemplateIfUnused(ctx context.Context, c client.Client, templatesInUse map[string]bool, ref *corev1.ObjectReference) error {
	// If ref is nil, do nothing (this can happen, because bootstrap templates are optional).
	if ref == nil {
		return nil
	}

	log := ctrl.LoggerFrom(ctx).WithValues(ref.Kind, klog.KRef(ref.Namespace, ref.Name))

	refID, err := templateRefID(ref)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate templateRefID")
	}

	// If the template is still in use, do nothing.
	if templatesInUse[refID] {
		log.V(3).Info(fmt.Sprintf("Not deleting %s, because it's still in use", ref.Kind))
		return nil
	}

	log.Info(fmt.Sprintf("Deleting %s", ref.Kind))
	if err := external.Delete(ctx, c, ref); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s %s", ref.Kind, klog.KRef(ref.Namespace, ref.Name))
	}
	return nil
}

// addTemplateRef adds the refs to the refMap with the templateRefID as key.
func addTemplateRef(refMap map[string]bool, refs ...*corev1.ObjectReference) error {
	for _, ref := range refs {
		if ref != nil {
			refID, err := templateRefID(ref)
			if err != nil {
				return errors.Wrapf(err, "failed to calculate templateRefID")
			}
			refMap[refID] = true
		}
	}
	return nil
}

// templateRefID returns the templateRefID of a ObjectReference in the format: g/k/name.
// Note: We don't include the version as references with different versions should be treated as equal.
func templateRefID(ref *corev1.ObjectReference) (string, error) {
	if ref == nil {
		return "", nil
	}

	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse apiVersion %q", ref.APIVersion)
	}

	return fmt.Sprintf("%s/%s/%s", gv.Group, ref.Kind, ref.Name), nil
}

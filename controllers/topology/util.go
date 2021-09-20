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

package topology

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// bootstrapTemplateNamePrefix calculates the name prefix for a BootstrapTemplate.
func bootstrapTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-bootstrap-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func infrastructureMachineTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-infra-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func controlPlaneInfrastructureMachineTemplateNamePrefix(clusterName string) string {
	return fmt.Sprintf("%s-control-plane-", clusterName)
}

// getReference gets the object referenced in ref.
// If necessary, it updates the ref to the latest apiVersion of the current contract.
func (r *ClusterReconciler) getReference(ctx context.Context, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.New("reference is not set")
	}
	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return nil, err
	}

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, ref.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", ref.Kind, ref.Name, ref.Namespace)
	}
	return obj, nil
}

// refToUnstructured returns an unstructured object with details from an ObjectReference.
func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	uns := &unstructured.Unstructured{}
	uns.SetAPIVersion(ref.APIVersion)
	uns.SetKind(ref.Kind)
	uns.SetNamespace(ref.Namespace)
	uns.SetName(ref.Name)
	return uns
}

// getMachineSetsForDeployment returns a list of MachineSets associated with a MachineDeployment.
func getMachineSetsForDeployment(ctx context.Context, c client.Reader, md types.NamespacedName) ([]*clusterv1.MachineSet, error) {
	// List MachineSets based on the MachineDeployment label.
	msList := &clusterv1.MachineSetList{}
	if err := c.List(ctx, msList,
		client.InNamespace(md.Namespace), client.MatchingLabels{clusterv1.MachineDeploymentLabelName: md.Name}); err != nil {
		return nil, errors.Wrapf(err, "failed to list MachineSets for MachineDeployment/%s", md.Name)
	}

	// Copy the MachineSets to an array of MachineSet pointers, to avoid MachineSet copying later.
	res := make([]*clusterv1.MachineSet, 0, len(msList.Items))
	for i := range msList.Items {
		res = append(res, &msList.Items[i])
	}
	return res, nil
}

// calculateTemplatesInUse returns all templates referenced in non-deleting MachineDeployment and MachineSets.
func calculateTemplatesInUse(md *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) (map[string]bool, error) {
	templatesInUse := map[string]bool{}

	// Templates of the MachineSet are still in use if the MachineSet is not in deleting state.
	for _, ms := range msList {
		if !ms.DeletionTimestamp.IsZero() {
			continue
		}

		bootstrapRef := ms.Spec.Template.Spec.Bootstrap.ConfigRef
		infrastructureRef := &ms.Spec.Template.Spec.InfrastructureRef
		if err := addTemplateRef(templatesInUse, bootstrapRef, infrastructureRef); err != nil {
			return nil, errors.Wrapf(err, "failed to add templates of %s to templatesInUse", tlog.KObj{Obj: ms})
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
		return nil, errors.Wrapf(err, "failed to add templates of %s to templatesInUse", tlog.KObj{Obj: md})
	}
	return templatesInUse, nil
}

// deleteTemplateIfUnused deletes the template (ref), if it is not in use (i.e. in templatesInUse).
func deleteTemplateIfUnused(ctx context.Context, c client.Client, templatesInUse map[string]bool, ref *corev1.ObjectReference) error {
	// If ref is nil, do nothing (this can happen, because bootstrap templates are optional).
	if ref == nil {
		return nil
	}

	log := tlog.LoggerFrom(ctx).WithRef(ref)

	refID, err := templateRefID(ref)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate templateRefID")
	}

	// If the template is still in use, do nothing.
	if templatesInUse[refID] {
		log.V(3).Infof("Not deleting %s, because it's still in use", tlog.KRef{Ref: ref})
		return nil
	}

	log.Infof("Deleting %s", tlog.KRef{Ref: ref})
	if err := external.Delete(ctx, c, ref); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete %s", tlog.KRef{Ref: ref})
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

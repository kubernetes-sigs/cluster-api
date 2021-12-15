/*
Copyright 2018 The Kubernetes Authors.

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

// Package mdutil implements MachineDeployment utilities.
package mdutil

import (
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/integer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conversion"
)

// MachineSetsByCreationTimestamp sorts a list of MachineSet by creation timestamp, using their names as a tie breaker.
type MachineSetsByCreationTimestamp []*clusterv1.MachineSet

func (o MachineSetsByCreationTimestamp) Len() int      { return len(o) }
func (o MachineSetsByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o MachineSetsByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

// MachineSetsBySizeOlder sorts a list of MachineSet by size in descending order, using their creation timestamp or name as a tie breaker.
// By using the creation timestamp, this sorts from old to new machine sets.
type MachineSetsBySizeOlder []*clusterv1.MachineSet

func (o MachineSetsBySizeOlder) Len() int      { return len(o) }
func (o MachineSetsBySizeOlder) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o MachineSetsBySizeOlder) Less(i, j int) bool {
	if *(o[i].Spec.Replicas) == *(o[j].Spec.Replicas) {
		return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
	}
	return *(o[i].Spec.Replicas) > *(o[j].Spec.Replicas)
}

// MachineSetsBySizeNewer sorts a list of MachineSet by size in descending order, using their creation timestamp or name as a tie breaker.
// By using the creation timestamp, this sorts from new to old machine sets.
type MachineSetsBySizeNewer []*clusterv1.MachineSet

func (o MachineSetsBySizeNewer) Len() int      { return len(o) }
func (o MachineSetsBySizeNewer) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o MachineSetsBySizeNewer) Less(i, j int) bool {
	if *(o[i].Spec.Replicas) == *(o[j].Spec.Replicas) {
		return o[j].CreationTimestamp.Before(&o[i].CreationTimestamp)
	}
	return *(o[i].Spec.Replicas) > *(o[j].Spec.Replicas)
}

// SetDeploymentRevision updates the revision for a deployment.
func SetDeploymentRevision(deployment *clusterv1.MachineDeployment, revision string) bool {
	updated := false

	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	if deployment.Annotations[clusterv1.RevisionAnnotation] != revision {
		deployment.Annotations[clusterv1.RevisionAnnotation] = revision
		updated = true
	}

	return updated
}

// MaxRevision finds the highest revision in the machine sets.
func MaxRevision(allMSs []*clusterv1.MachineSet, logger logr.Logger) int64 {
	max := int64(0)
	for _, ms := range allMSs {
		if v, err := Revision(ms); err != nil {
			// Skip the machine sets when it failed to parse their revision information
			logger.Error(err, "Couldn't parse revision for machine set, deployment controller will skip it when reconciling revisions",
				"machineset", ms.Name)
		} else if v > max {
			max = v
		}
	}
	return max
}

// Revision returns the revision number of the input object.
func Revision(obj runtime.Object) (int64, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[clusterv1.RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

var annotationsToSkip = map[string]bool{
	corev1.LastAppliedConfigAnnotation:  true,
	clusterv1.RevisionAnnotation:        true,
	clusterv1.RevisionHistoryAnnotation: true,
	clusterv1.DesiredReplicasAnnotation: true,
	clusterv1.MaxReplicasAnnotation:     true,

	// Exclude the conversion annotation, to avoid infinite loops between the conversion webhook
	// and the MachineDeployment controller syncing the annotations between a MachineDeployment
	// and its linked MachineSets.
	//
	// See https://github.com/kubernetes-sigs/cluster-api/pull/3010#issue-413767831 for more details.
	conversion.DataAnnotation: true,
}

// skipCopyAnnotation returns true if we should skip copying the annotation with the given annotation key
// TODO(tbd): How to decide which annotations should / should not be copied?
//       See https://github.com/kubernetes/kubernetes/pull/20035#issuecomment-179558615
func skipCopyAnnotation(key string) bool {
	return annotationsToSkip[key]
}

// copyDeploymentAnnotationsToMachineSet copies deployment's annotations to machine set's annotations,
// and returns true if machine set's annotation is changed.
// Note that apply and revision annotations are not copied.
func copyDeploymentAnnotationsToMachineSet(deployment *clusterv1.MachineDeployment, ms *clusterv1.MachineSet) bool {
	msAnnotationsChanged := false
	if ms.Annotations == nil {
		ms.Annotations = make(map[string]string)
	}
	for k, v := range deployment.Annotations {
		// newMS revision is updated automatically in getNewMachineSet, and the deployment's revision number is then updated
		// by copying its newMS revision number. We should not copy deployment's revision to its newMS, since the update of
		// deployment revision number may fail (revision becomes stale) and the revision number in newMS is more reliable.
		if skipCopyAnnotation(k) || ms.Annotations[k] == v {
			continue
		}
		ms.Annotations[k] = v
		msAnnotationsChanged = true
	}
	return msAnnotationsChanged
}

func getMaxReplicasAnnotation(ms *clusterv1.MachineSet, logger logr.Logger) (int32, bool) {
	return getIntFromAnnotation(ms, clusterv1.MaxReplicasAnnotation, logger)
}

func getIntFromAnnotation(ms *clusterv1.MachineSet, annotationKey string, logger logr.Logger) (int32, bool) {
	logger = logger.WithValues("machineset", ms.Name, "annotationKey", annotationKey)

	annotationValue, ok := ms.Annotations[annotationKey]
	if !ok {
		return int32(0), false
	}
	intValue, err := strconv.ParseInt(annotationValue, 10, 32)
	if err != nil {
		logger.V(2).Info("Cannot convert the value to integer", "annotationValue", annotationValue)
		return int32(0), false
	}
	return int32(intValue), true
}

// SetNewMachineSetAnnotations sets new machine set's annotations appropriately by updating its revision and
// copying required deployment annotations to it; it returns true if machine set's annotation is changed.
func SetNewMachineSetAnnotations(deployment *clusterv1.MachineDeployment, newMS *clusterv1.MachineSet, newRevision string, exists bool, logger logr.Logger) bool {
	logger = logger.WithValues("machineset", newMS.Name)

	// First, copy deployment's annotations (except for apply and revision annotations)
	annotationChanged := copyDeploymentAnnotationsToMachineSet(deployment, newMS)
	// Then, update machine set's revision annotation
	if newMS.Annotations == nil {
		newMS.Annotations = make(map[string]string)
	}
	oldRevision, ok := newMS.Annotations[clusterv1.RevisionAnnotation]
	// The newMS's revision should be the greatest among all MSes. Usually, its revision number is newRevision (the max revision number
	// of all old MSes + 1). However, it's possible that some of the old MSes are deleted after the newMS revision being updated, and
	// newRevision becomes smaller than newMS's revision. We should only update newMS revision when it's smaller than newRevision.

	oldRevisionInt, err := strconv.ParseInt(oldRevision, 10, 64)
	if err != nil {
		if oldRevision != "" {
			logger.Error(err, "Updating machine set revision OldRevision not int")
			return false
		}
		// If the MS annotation is empty then initialise it to 0
		oldRevisionInt = 0
	}
	newRevisionInt, err := strconv.ParseInt(newRevision, 10, 64)
	if err != nil {
		logger.Error(err, "Updating machine set revision NewRevision not int")
		return false
	}
	if oldRevisionInt < newRevisionInt {
		newMS.Annotations[clusterv1.RevisionAnnotation] = newRevision
		annotationChanged = true
		logger.V(4).Info("Updating machine set revision", "revision", newRevision)
	}
	// If a revision annotation already existed and this machine set was updated with a new revision
	// then that means we are rolling back to this machine set. We need to preserve the old revisions
	// for historical information.
	if ok && annotationChanged {
		revisionHistoryAnnotation := newMS.Annotations[clusterv1.RevisionHistoryAnnotation]
		oldRevisions := strings.Split(revisionHistoryAnnotation, ",")
		if oldRevisions[0] == "" {
			newMS.Annotations[clusterv1.RevisionHistoryAnnotation] = oldRevision
		} else {
			oldRevisions = append(oldRevisions, oldRevision)
			newMS.Annotations[clusterv1.RevisionHistoryAnnotation] = strings.Join(oldRevisions, ",")
		}
	}
	// If the new machine set is about to be created, we need to add replica annotations to it.
	if !exists && SetReplicasAnnotations(newMS, *(deployment.Spec.Replicas), *(deployment.Spec.Replicas)+MaxSurge(*deployment)) {
		annotationChanged = true
	}
	return annotationChanged
}

// FindOneActiveOrLatest returns the only active or the latest machine set in case there is at most one active
// machine set. If there are more than one active machine sets, return nil so machine sets can be scaled down
// to the point where there is only one active machine set.
func FindOneActiveOrLatest(newMS *clusterv1.MachineSet, oldMSs []*clusterv1.MachineSet) *clusterv1.MachineSet {
	if newMS == nil && len(oldMSs) == 0 {
		return nil
	}

	sort.Sort(sort.Reverse(MachineSetsByCreationTimestamp(oldMSs)))
	allMSs := FilterActiveMachineSets(append(oldMSs, newMS))

	switch len(allMSs) {
	case 0:
		// If there is no active machine set then we should return the newest.
		if newMS != nil {
			return newMS
		}
		return oldMSs[0]
	case 1:
		return allMSs[0]
	default:
		return nil
	}
}

// SetReplicasAnnotations sets the desiredReplicas and maxReplicas into the annotations.
func SetReplicasAnnotations(ms *clusterv1.MachineSet, desiredReplicas, maxReplicas int32) bool {
	updated := false
	if ms.Annotations == nil {
		ms.Annotations = make(map[string]string)
	}
	desiredString := fmt.Sprintf("%d", desiredReplicas)
	if hasString := ms.Annotations[clusterv1.DesiredReplicasAnnotation]; hasString != desiredString {
		ms.Annotations[clusterv1.DesiredReplicasAnnotation] = desiredString
		updated = true
	}
	if hasString := ms.Annotations[clusterv1.MaxReplicasAnnotation]; hasString != fmt.Sprintf("%d", maxReplicas) {
		ms.Annotations[clusterv1.MaxReplicasAnnotation] = fmt.Sprintf("%d", maxReplicas)
		updated = true
	}
	return updated
}

// ReplicasAnnotationsNeedUpdate return true if the replicas annotation needs to be updated.
func ReplicasAnnotationsNeedUpdate(ms *clusterv1.MachineSet, desiredReplicas, maxReplicas int32) bool {
	if ms.Annotations == nil {
		return true
	}
	desiredString := fmt.Sprintf("%d", desiredReplicas)
	if hasString := ms.Annotations[clusterv1.DesiredReplicasAnnotation]; hasString != desiredString {
		return true
	}
	if hasString := ms.Annotations[clusterv1.MaxReplicasAnnotation]; hasString != fmt.Sprintf("%d", maxReplicas) {
		return true
	}
	return false
}

// MaxUnavailable returns the maximum unavailable machines a rolling deployment can take.
func MaxUnavailable(deployment clusterv1.MachineDeployment) int32 {
	if !IsRollingUpdate(&deployment) || *(deployment.Spec.Replicas) == 0 {
		return int32(0)
	}
	// Error caught by validation
	_, maxUnavailable, _ := ResolveFenceposts(deployment.Spec.Strategy.RollingUpdate.MaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, *(deployment.Spec.Replicas))
	if maxUnavailable > *deployment.Spec.Replicas {
		return *deployment.Spec.Replicas
	}
	return maxUnavailable
}

// MaxSurge returns the maximum surge machines a rolling deployment can take.
func MaxSurge(deployment clusterv1.MachineDeployment) int32 {
	if !IsRollingUpdate(&deployment) {
		return int32(0)
	}
	// Error caught by validation
	maxSurge, _, _ := ResolveFenceposts(deployment.Spec.Strategy.RollingUpdate.MaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, *(deployment.Spec.Replicas))
	return maxSurge
}

// GetProportion will estimate the proportion for the provided machine set using 1. the current size
// of the parent deployment, 2. the replica count that needs be added on the machine sets of the
// deployment, and 3. the total replicas added in the machine sets of the deployment so far.
func GetProportion(ms *clusterv1.MachineSet, d clusterv1.MachineDeployment, deploymentReplicasToAdd, deploymentReplicasAdded int32, logger logr.Logger) int32 {
	if ms == nil || *(ms.Spec.Replicas) == 0 || deploymentReplicasToAdd == 0 || deploymentReplicasToAdd == deploymentReplicasAdded {
		return int32(0)
	}

	msFraction := getMachineSetFraction(*ms, d, logger)
	allowed := deploymentReplicasToAdd - deploymentReplicasAdded

	if deploymentReplicasToAdd > 0 {
		// Use the minimum between the machine set fraction and the maximum allowed replicas
		// when scaling up. This way we ensure we will not scale up more than the allowed
		// replicas we can add.
		return integer.Int32Min(msFraction, allowed)
	}
	// Use the maximum between the machine set fraction and the maximum allowed replicas
	// when scaling down. This way we ensure we will not scale down more than the allowed
	// replicas we can remove.
	return integer.Int32Max(msFraction, allowed)
}

// getMachineSetFraction estimates the fraction of replicas a machine set can have in
// 1. a scaling event during a rollout or 2. when scaling a paused deployment.
func getMachineSetFraction(ms clusterv1.MachineSet, d clusterv1.MachineDeployment, logger logr.Logger) int32 {
	// If we are scaling down to zero then the fraction of this machine set is its whole size (negative)
	if *(d.Spec.Replicas) == int32(0) {
		return -*(ms.Spec.Replicas)
	}

	deploymentReplicas := *(d.Spec.Replicas) + MaxSurge(d)
	annotatedReplicas, ok := getMaxReplicasAnnotation(&ms, logger)
	if !ok {
		// If we cannot find the annotation then fallback to the current deployment size. Note that this
		// will not be an accurate proportion estimation in case other machine sets have different values
		// which means that the deployment was scaled at some point but we at least will stay in limits
		// due to the min-max comparisons in getProportion.
		annotatedReplicas = d.Status.Replicas
	}

	// We should never proportionally scale up from zero which means ms.spec.replicas and annotatedReplicas
	// will never be zero here.
	newMSsize := (float64(*(ms.Spec.Replicas) * deploymentReplicas)) / float64(annotatedReplicas)
	return integer.RoundToInt32(newMSsize) - *(ms.Spec.Replicas)
}

// EqualMachineTemplate returns true if two given machineTemplateSpec are equal,
// ignoring the diff in value of Labels["machine-template-hash"], and the version from external references.
func EqualMachineTemplate(template1, template2 *clusterv1.MachineTemplateSpec) bool {
	t1Copy := template1.DeepCopy()
	t2Copy := template2.DeepCopy()

	// Remove `machine-template-hash` from the comparison:
	// 1. The hash result would be different upon machineTemplateSpec API changes
	//    (e.g. the addition of a new field will cause the hash code to change)
	// 2. The deployment template won't have hash labels
	delete(t1Copy.Labels, clusterv1.MachineDeploymentUniqueLabel)
	delete(t2Copy.Labels, clusterv1.MachineDeploymentUniqueLabel)

	// Remove the version part from the references APIVersion field,
	// for more details see issue #2183 and #2140.
	t1Copy.Spec.InfrastructureRef.APIVersion = t1Copy.Spec.InfrastructureRef.GroupVersionKind().Group
	if t1Copy.Spec.Bootstrap.ConfigRef != nil {
		t1Copy.Spec.Bootstrap.ConfigRef.APIVersion = t1Copy.Spec.Bootstrap.ConfigRef.GroupVersionKind().Group
	}
	t2Copy.Spec.InfrastructureRef.APIVersion = t2Copy.Spec.InfrastructureRef.GroupVersionKind().Group
	if t2Copy.Spec.Bootstrap.ConfigRef != nil {
		t2Copy.Spec.Bootstrap.ConfigRef.APIVersion = t2Copy.Spec.Bootstrap.ConfigRef.GroupVersionKind().Group
	}

	return apiequality.Semantic.DeepEqual(t1Copy, t2Copy)
}

// FindNewMachineSet returns the new MS this given deployment targets (the one with the same machine template).
func FindNewMachineSet(deployment *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) *clusterv1.MachineSet {
	sort.Sort(MachineSetsByCreationTimestamp(msList))
	for i := range msList {
		if EqualMachineTemplate(&msList[i].Spec.Template, &deployment.Spec.Template) {
			// In rare cases, such as after cluster upgrades, Deployment may end up with
			// having more than one new MachineSets that have the same template,
			// see https://github.com/kubernetes/kubernetes/issues/40415
			// We deterministically choose the oldest new MachineSet with matching template hash.
			return msList[i]
		}
	}
	// new MachineSet does not exist.
	return nil
}

// FindOldMachineSets returns the old machine sets targeted by the given Deployment, with the given slice of MSes.
// Returns two list of machine sets
//  - the first contains all old machine sets with all non-zero replicas
//  - the second contains all old machine sets
func FindOldMachineSets(deployment *clusterv1.MachineDeployment, msList []*clusterv1.MachineSet) ([]*clusterv1.MachineSet, []*clusterv1.MachineSet) {
	var requiredMSs []*clusterv1.MachineSet
	allMSs := make([]*clusterv1.MachineSet, 0, len(msList))
	newMS := FindNewMachineSet(deployment, msList)
	for _, ms := range msList {
		// Filter out new machine set
		if newMS != nil && ms.UID == newMS.UID {
			continue
		}
		allMSs = append(allMSs, ms)
		if *(ms.Spec.Replicas) != 0 {
			requiredMSs = append(requiredMSs, ms)
		}
	}
	return requiredMSs, allMSs
}

// GetReplicaCountForMachineSets returns the sum of Replicas of the given machine sets.
func GetReplicaCountForMachineSets(machineSets []*clusterv1.MachineSet) int32 {
	totalReplicas := int32(0)
	for _, ms := range machineSets {
		if ms != nil {
			totalReplicas += *(ms.Spec.Replicas)
		}
	}
	return totalReplicas
}

// GetActualReplicaCountForMachineSets returns the sum of actual replicas of the given machine sets.
func GetActualReplicaCountForMachineSets(machineSets []*clusterv1.MachineSet) int32 {
	totalActualReplicas := int32(0)
	for _, ms := range machineSets {
		if ms != nil {
			totalActualReplicas += ms.Status.Replicas
		}
	}
	return totalActualReplicas
}

// TotalMachineSetsReplicaSum returns sum of max(ms.Spec.Replicas, ms.Status.Replicas) across all the machine sets.
//
// This is used to guarantee that the total number of machines will not exceed md.Spec.Replicas + maxSurge.
// Use max(spec.Replicas,status.Replicas) to cover the cases that:
// 1. Scale up, where spec.Replicas increased but no machine created yet, so spec.Replicas > status.Replicas
// 2. Scale down, where spec.Replicas decreased but machine not deleted yet, so spec.Replicas < status.Replicas.
func TotalMachineSetsReplicaSum(machineSets []*clusterv1.MachineSet) int32 {
	totalReplicas := int32(0)
	for _, ms := range machineSets {
		if ms != nil {
			totalReplicas += integer.Int32Max(*(ms.Spec.Replicas), ms.Status.Replicas)
		}
	}
	return totalReplicas
}

// GetReadyReplicaCountForMachineSets returns the number of ready machines corresponding to the given machine sets.
func GetReadyReplicaCountForMachineSets(machineSets []*clusterv1.MachineSet) int32 {
	totalReadyReplicas := int32(0)
	for _, ms := range machineSets {
		if ms != nil {
			totalReadyReplicas += ms.Status.ReadyReplicas
		}
	}
	return totalReadyReplicas
}

// GetAvailableReplicaCountForMachineSets returns the number of available machines corresponding to the given machine sets.
func GetAvailableReplicaCountForMachineSets(machineSets []*clusterv1.MachineSet) int32 {
	totalAvailableReplicas := int32(0)
	for _, ms := range machineSets {
		if ms != nil {
			totalAvailableReplicas += ms.Status.AvailableReplicas
		}
	}
	return totalAvailableReplicas
}

// IsRollingUpdate returns true if the strategy type is a rolling update.
func IsRollingUpdate(deployment *clusterv1.MachineDeployment) bool {
	return deployment.Spec.Strategy.Type == clusterv1.RollingUpdateMachineDeploymentStrategyType
}

// DeploymentComplete considers a deployment to be complete once all of its desired replicas
// are updated and available, and no old machines are running.
func DeploymentComplete(deployment *clusterv1.MachineDeployment, newStatus *clusterv1.MachineDeploymentStatus) bool {
	return newStatus.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		newStatus.Replicas == *(deployment.Spec.Replicas) &&
		newStatus.AvailableReplicas == *(deployment.Spec.Replicas) &&
		newStatus.ObservedGeneration >= deployment.Generation
}

// NewMSNewReplicas calculates the number of replicas a deployment's new MS should have.
// When one of the following is true, we're rolling out the deployment; otherwise, we're scaling it.
// 1) The new MS is saturated: newMS's replicas == deployment's replicas
// 2) For RollingUpdateStrategy: Max number of machines allowed is reached: deployment's replicas + maxSurge == all MSs' replicas.
// 3) For OnDeleteStrategy: Max number of machines allowed is reached: deployment's replicas == all MSs' replicas.
func NewMSNewReplicas(deployment *clusterv1.MachineDeployment, allMSs []*clusterv1.MachineSet, newMS *clusterv1.MachineSet) (int32, error) {
	switch deployment.Spec.Strategy.Type {
	case clusterv1.RollingUpdateMachineDeploymentStrategyType:
		// Check if we can scale up.
		maxSurge, err := intstrutil.GetScaledValueFromIntOrPercent(deployment.Spec.Strategy.RollingUpdate.MaxSurge, int(*(deployment.Spec.Replicas)), true)
		if err != nil {
			return 0, err
		}
		// Find the total number of machines
		currentMachineCount := TotalMachineSetsReplicaSum(allMSs)
		maxTotalMachines := *(deployment.Spec.Replicas) + int32(maxSurge)
		if currentMachineCount >= maxTotalMachines {
			// Cannot scale up.
			return *(newMS.Spec.Replicas), nil
		}
		// Scale up.
		scaleUpCount := maxTotalMachines - currentMachineCount
		// Do not exceed the number of desired replicas.
		scaleUpCount = integer.Int32Min(scaleUpCount, *(deployment.Spec.Replicas)-*(newMS.Spec.Replicas))
		return *(newMS.Spec.Replicas) + scaleUpCount, nil
	case clusterv1.OnDeleteMachineDeploymentStrategyType:
		// Find the total number of machines
		currentMachineCount := TotalMachineSetsReplicaSum(allMSs)
		if currentMachineCount >= *(deployment.Spec.Replicas) {
			// Cannot scale up as more replicas exist than desired number of replicas in the MachineDeployment.
			return *(newMS.Spec.Replicas), nil
		}
		// Scale up the latest MachineSet so the total amount of replicas across all MachineSets match
		// the desired number of replicas in the MachineDeployment
		scaleUpCount := *(deployment.Spec.Replicas) - currentMachineCount
		return *(newMS.Spec.Replicas) + scaleUpCount, nil
	default:
		return 0, fmt.Errorf("deployment strategy %v isn't supported", deployment.Spec.Strategy.Type)
	}
}

// IsSaturated checks if the new machine set is saturated by comparing its size with its deployment size.
// Both the deployment and the machine set have to believe this machine set can own all of the desired
// replicas in the deployment and the annotation helps in achieving that. All machines of the MachineSet
// need to be available.
func IsSaturated(deployment *clusterv1.MachineDeployment, ms *clusterv1.MachineSet) bool {
	if ms == nil {
		return false
	}
	desiredString := ms.Annotations[clusterv1.DesiredReplicasAnnotation]
	desired, err := strconv.ParseInt(desiredString, 10, 32)
	if err != nil {
		return false
	}
	return *(ms.Spec.Replicas) == *(deployment.Spec.Replicas) &&
		int32(desired) == *(deployment.Spec.Replicas) &&
		ms.Status.AvailableReplicas == *(deployment.Spec.Replicas)
}

// ResolveFenceposts resolves both maxSurge and maxUnavailable. This needs to happen in one
// step. For example:
//
// 2 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1), then old(-1), then new(+1)
// 1 desired, max unavailable 1%, surge 0% - should scale old(-1), then new(+1)
// 2 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 25%, surge 1% - should scale new(+1), then old(-1)
// 2 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1), then new(+1), then old(-1)
// 1 desired, max unavailable 0%, surge 1% - should scale new(+1), then old(-1).
func ResolveFenceposts(maxSurge, maxUnavailable *intstrutil.IntOrString, desired int32) (int32, int32, error) {
	surge, err := intstrutil.GetScaledValueFromIntOrPercent(maxSurge, int(desired), true)
	if err != nil {
		return 0, 0, err
	}
	unavailable, err := intstrutil.GetScaledValueFromIntOrPercent(maxUnavailable, int(desired), false)
	if err != nil {
		return 0, 0, err
	}

	if surge == 0 && unavailable == 0 {
		// Validation should never allow the user to explicitly use zero values for both maxSurge
		// maxUnavailable. Due to rounding down maxUnavailable though, it may resolve to zero.
		// If both fenceposts resolve to zero, then we should set maxUnavailable to 1 on the
		// theory that surge might not work due to quota.
		unavailable = 1
	}

	return int32(surge), int32(unavailable), nil
}

// FilterActiveMachineSets returns machine sets that have (or at least ought to have) machines.
func FilterActiveMachineSets(machineSets []*clusterv1.MachineSet) []*clusterv1.MachineSet {
	activeFilter := func(ms *clusterv1.MachineSet) bool {
		return ms != nil && ms.Spec.Replicas != nil && *(ms.Spec.Replicas) > 0
	}
	return FilterMachineSets(machineSets, activeFilter)
}

type filterMS func(ms *clusterv1.MachineSet) bool

// FilterMachineSets returns machine sets that are filtered by filterFn (all returned ones should match filterFn).
func FilterMachineSets(mSes []*clusterv1.MachineSet, filterFn filterMS) []*clusterv1.MachineSet {
	var filtered []*clusterv1.MachineSet
	for i := range mSes {
		if filterFn(mSes[i]) {
			filtered = append(filtered, mSes[i])
		}
	}
	return filtered
}

// CloneAndAddLabel clones the given map and returns a new map with the given key and value added.
// Returns the given map, if labelKey is empty.
func CloneAndAddLabel(labels map[string]string, labelKey, labelValue string) map[string]string {
	if labelKey == "" {
		// Don't need to add a label.
		return labels
	}
	// Clone.
	newLabels := map[string]string{}
	for key, value := range labels {
		newLabels[key] = value
	}
	newLabels[labelKey] = labelValue
	return newLabels
}

// CloneSelectorAndAddLabel clones the given selector and returns a new selector with the given key and value added.
// Returns the given selector, if labelKey is empty.
func CloneSelectorAndAddLabel(selector *metav1.LabelSelector, labelKey, labelValue string) *metav1.LabelSelector {
	if labelKey == "" {
		// Don't need to add a label.
		return selector
	}

	// Clone.
	newSelector := new(metav1.LabelSelector)

	// TODO(madhusudancs): Check if you can use deepCopy_extensions_LabelSelector here.
	newSelector.MatchLabels = make(map[string]string)
	if selector.MatchLabels != nil {
		for key, val := range selector.MatchLabels {
			newSelector.MatchLabels[key] = val
		}
	}
	newSelector.MatchLabels[labelKey] = labelValue

	if selector.MatchExpressions != nil {
		newMExps := make([]metav1.LabelSelectorRequirement, len(selector.MatchExpressions))
		for i, me := range selector.MatchExpressions {
			newMExps[i].Key = me.Key
			newMExps[i].Operator = me.Operator
			if me.Values != nil {
				newMExps[i].Values = make([]string, len(me.Values))
				copy(newMExps[i].Values, me.Values)
			} else {
				newMExps[i].Values = nil
			}
		}
		newSelector.MatchExpressions = newMExps
	} else {
		newSelector.MatchExpressions = nil
	}

	return newSelector
}

// SpewHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func SpewHashObject(hasher hash.Hash, objectToWrite interface{}) error {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	if _, err := printer.Fprintf(hasher, "%#v", objectToWrite); err != nil {
		return fmt.Errorf("failed to write object to hasher")
	}
	return nil
}

// ComputeSpewHash computes the hash of a MachineTemplateSpec using the spew library.
func ComputeSpewHash(objectToWrite interface{}) (uint32, error) {
	machineTemplateSpecHasher := fnv.New32a()
	if err := SpewHashObject(machineTemplateSpecHasher, objectToWrite); err != nil {
		return 0, err
	}
	return machineTemplateSpecHasher.Sum32(), nil
}

// GetDeletingMachineCount gets the number of machines that are in the process of being deleted
// in a machineList.
func GetDeletingMachineCount(machineList *clusterv1.MachineList) int32 {
	var deletingMachineCount int32
	for _, machine := range machineList.Items {
		if !machine.GetDeletionTimestamp().IsZero() {
			deletingMachineCount++
		}
	}
	return deletingMachineCount
}

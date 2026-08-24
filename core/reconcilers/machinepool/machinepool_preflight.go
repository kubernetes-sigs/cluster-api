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

package machinepool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

type preflightCheckErrorMessage *string

// preflightFailedRequeueAfter is used to requeue the MachinePool to re-verify the preflight checks if
// the preflight checks fail.
const preflightFailedRequeueAfter = 15 * time.Second

// reconcilePreflightChecks is an advisory-only reconcile phase that surfaces MachinePool version skew.
func (r *Reconciler) reconcilePreflightChecks(ctx context.Context, s *scope) (ctrl.Result, error) {
	// If the MachinePoolPreflightChecks feature gate is disabled return early.
	if !feature.Gates.Enabled(feature.MachinePoolPreflightChecks) {
		return ctrl.Result{}, nil
	}

	mp := s.machinePool

	preflightCheckErrMessages, err := r.runPreflightChecks(ctx, s.cluster, mp, "reconcile")
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(preflightCheckErrMessages) > 0 {
		msg := strings.Join(preflightCheckErrMessages, "; ")
		v1beta1conditions.MarkFalse(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition, clusterv1.PreflightCheckFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning, "%s", msg)
		r.recorder.Eventf(mp, corev1.EventTypeWarning, clusterv1.PreflightCheckFailedV1Beta1Reason, "%s", msg)
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	v1beta1conditions.MarkTrue(mp, clusterv1.PreflightCheckSucceededV1Beta1Condition)
	return ctrl.Result{}, nil
}

// runPreflightChecks runs the MachinePool preflight checks and returns the list of failed check messages.
func (r *Reconciler) runPreflightChecks(ctx context.Context, cluster *clusterv1.Cluster, mp *clusterv1.MachinePool, action string) ([]string, error) {
	log := ctrl.LoggerFrom(ctx)
	// If the MachinePoolPreflightChecks feature gate is disabled return early.
	if !feature.Gates.Enabled(feature.MachinePoolPreflightChecks) {
		return nil, nil
	}

	skipped, err := r.skippedPreflightChecks(ctx, mp)
	if err != nil {
		return nil, err
	}

	// If all the preflight checks are skipped then return early.
	if len(r.PreflightChecks) == 0 || skipped.Has(clusterv1.MachinePoolPreflightCheckAll) {
		return nil, nil
	}

	// If the cluster does not have a control plane reference then there is nothing to do. Return early.
	if !cluster.Spec.ControlPlaneRef.IsDefined() {
		return nil, nil
	}

	// Get the control plane object.
	controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, cluster.Spec.ControlPlaneRef, cluster.Namespace)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to get ControlPlane %s", action, klog.KRef(cluster.Namespace, cluster.Spec.ControlPlaneRef.Name))
	}
	cpKlogRef := klog.KRef(controlPlane.GetNamespace(), controlPlane.GetName())

	// If the Control Plane version is not set then we are dealing with a control plane that does not support version
	// or a control plane where the version is not set. In both cases we cannot perform any preflight checks as
	// we do not have enough information. Return early.
	cpVersion, err := contract.ControlPlane().Version().Get(controlPlane)
	if err != nil {
		if pkgerrors.Is(err, contract.ErrFieldNotFound) {
			return nil, nil
		}
		return nil, pkgerrors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to get the version of ControlPlane %s", action, cpKlogRef)
	}
	cpSemver, err := semver.ParseTolerant(*cpVersion)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to parse version %q of ControlPlane %s", action, *cpVersion, cpKlogRef)
	}

	errList := []error{}
	preflightCheckErrs := []preflightCheckErrorMessage{}
	// Run the control-plane-stable preflight check.
	if shouldRun(r.PreflightChecks, skipped, clusterv1.MachinePoolPreflightCheckControlPlaneIsStable) {
		preflightCheckErr, err := r.controlPlaneStablePreflightCheck(controlPlane, cluster, *cpVersion)
		if err != nil {
			errList = append(errList, err)
		}
		if preflightCheckErr != nil {
			preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
		}
	}

	// Check the version skew policies only if version is defined in the MachinePool.
	if mp.Spec.Template.Spec.Version != "" {
		mpVersion := mp.Spec.Template.Spec.Version
		mpSemver, err := semver.ParseTolerant(mpVersion)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to parse version %q of MachinePool %s", action, mpVersion, klog.KObj(mp))
		}

		// Run the kubernetes-version skew preflight check.
		if shouldRun(r.PreflightChecks, skipped, clusterv1.MachinePoolPreflightCheckKubernetesVersionSkew) {
			if preflightCheckErr := r.kubernetesVersionPreflightCheck(cpSemver, mpSemver); preflightCheckErr != nil {
				preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
			}
		}

		// Run the kubeadm-version skew preflight check.
		if shouldRun(r.PreflightChecks, skipped, clusterv1.MachinePoolPreflightCheckKubeadmVersionSkew) {
			if preflightCheckErr := r.kubeadmVersionPreflightCheck(cpSemver, mpSemver, mp); preflightCheckErr != nil {
				preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
			}
		}

		// Run the control plane version skew preflight check.
		if shouldRun(r.PreflightChecks, skipped, clusterv1.MachinePoolPreflightCheckControlPlaneVersionSkew) {
			if preflightCheckErr := r.controlPlaneVersionPreflightCheck(cluster, *cpVersion, mpVersion); preflightCheckErr != nil {
				preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
			}
		}
	}

	if len(errList) > 0 {
		return nil, pkgerrors.Wrapf(kerrors.NewAggregate(errList), "failed to perform %q: failed to perform preflight checks", action)
	}
	if len(preflightCheckErrs) > 0 {
		preflightCheckErrStrings := []string{}
		for _, v := range preflightCheckErrs {
			preflightCheckErrStrings = append(preflightCheckErrStrings, *v)
		}
		log.Info(fmt.Sprintf("%s: MachinePool preflight check(s) failed: %s", action, strings.Join(preflightCheckErrStrings, "; ")))
		return preflightCheckErrStrings, nil
	}
	return nil, nil
}

func shouldRun(preflightChecks, skippedPreflightChecks sets.Set[clusterv1.MachinePoolPreflightCheck], preflightCheck clusterv1.MachinePoolPreflightCheck) bool {
	return (preflightChecks.Has(clusterv1.MachinePoolPreflightCheckAll) || preflightChecks.Has(preflightCheck)) &&
		(!skippedPreflightChecks.Has(clusterv1.MachinePoolPreflightCheckAll) && !skippedPreflightChecks.Has(preflightCheck))
}

func (r *Reconciler) controlPlaneStablePreflightCheck(controlPlane *unstructured.Unstructured, cluster *clusterv1.Cluster, controlPlaneVersion string) (preflightCheckErrorMessage, error) {
	cpKlogRef := klog.KRef(controlPlane.GetNamespace(), controlPlane.GetName())

	if feature.Gates.Enabled(feature.ClusterTopology) && cluster.Spec.Topology.IsDefined() {
		// Surface when we expect an upgrade to be propagated to the control plane for topology clusters.
		// NOTE: in case the cluster is performing an upgrade, allow the current step for the current version.
		hasSameVersionOfCurrentUpgradeStep := false
		if version, ok := cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
			hasSameVersionOfCurrentUpgradeStep = version == controlPlaneVersion
		}

		if cluster.Spec.Topology.Version != controlPlaneVersion && !hasSameVersionOfCurrentUpgradeStep {
			v := cluster.Spec.Topology.Version
			if version, ok := cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
				v = version
			}
			return ptr.To(fmt.Sprintf("%s %s has a pending version upgrade to %s (%q preflight check failed)", controlPlane.GetKind(), cpKlogRef, v, clusterv1.MachinePoolPreflightCheckControlPlaneIsStable)), nil
		}
	}

	// Check that the control plane is not provisioning.
	isProvisioning, err := contract.ControlPlane().IsProvisioning(controlPlane)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to perform %q preflight check: failed to check if %s %s is provisioning", clusterv1.MachinePoolPreflightCheckControlPlaneIsStable, controlPlane.GetKind(), cpKlogRef)
	}
	if isProvisioning {
		return ptr.To(fmt.Sprintf("%s %s is provisioning (%q preflight check failed)", controlPlane.GetKind(), cpKlogRef, clusterv1.MachinePoolPreflightCheckControlPlaneIsStable)), nil
	}

	// Check that the control plane is not upgrading.
	isUpgrading, err := contract.ControlPlane().IsUpgrading(controlPlane)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to perform %q preflight check: failed to check if the %s %s is upgrading", clusterv1.MachinePoolPreflightCheckControlPlaneIsStable, controlPlane.GetKind(), cpKlogRef)
	}
	if isUpgrading {
		return ptr.To(fmt.Sprintf("%s %s is upgrading (%q preflight check failed)", controlPlane.GetKind(), cpKlogRef, clusterv1.MachinePoolPreflightCheckControlPlaneIsStable)), nil
	}

	return nil, nil
}

func (r *Reconciler) kubernetesVersionPreflightCheck(cpSemver, mpSemver semver.Version) preflightCheckErrorMessage {
	// Check the Kubernetes version skew policy.
	// => MP minor version cannot be greater than the Control Plane minor version.
	// => MP minor version cannot be outside of the supported skew.
	// Kubernetes skew policy: https://kubernetes.io/releases/version-skew-policy/#kubelet
	if mpSemver.Minor > cpSemver.Minor {
		return ptr.To(fmt.Sprintf("MachinePool version (%s) and ControlPlane version (%s) do not conform to the kubernetes version skew policy as MachinePool version is higher than ControlPlane version (%q preflight check failed)", mpSemver.String(), cpSemver.String(), clusterv1.MachinePoolPreflightCheckKubernetesVersionSkew))
	}
	minorSkew := uint64(3)
	if mpSemver.Minor < cpSemver.Minor-minorSkew {
		return ptr.To(fmt.Sprintf("MachinePool version (%s) and ControlPlane version (%s) do not conform to the kubernetes version skew policy as MachinePool version is more than %d minor versions older than the ControlPlane version (%q preflight check failed)", mpSemver.String(), cpSemver.String(), minorSkew, clusterv1.MachinePoolPreflightCheckKubernetesVersionSkew))
	}

	return nil
}

func (r *Reconciler) kubeadmVersionPreflightCheck(cpSemver, mpSemver semver.Version, mp *clusterv1.MachinePool) preflightCheckErrorMessage {
	// If the bootstrap.configRef is nil return early.
	if !mp.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		return nil
	}

	// If using kubeadm bootstrap provider, check the kubeadm version skew policy.
	// => MP version should match (major+minor) the Control Plane version.
	// kubeadm skew policy: https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#kubeadm-s-skew-against-kubeadm
	bootstrapConfigRef := mp.Spec.Template.Spec.Bootstrap.ConfigRef
	kubeadmBootstrapProviderUsed := bootstrapConfigRef.Kind == "KubeadmConfigTemplate" &&
		bootstrapConfigRef.APIGroup == bootstrapv1.GroupVersion.Group
	if kubeadmBootstrapProviderUsed {
		if cpSemver.Minor != mpSemver.Minor {
			return ptr.To(fmt.Sprintf("MachinePool version (%s) and ControlPlane version (%s) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (%q preflight check failed)", mpSemver.String(), cpSemver.String(), clusterv1.MachinePoolPreflightCheckKubeadmVersionSkew))
		}
	}
	return nil
}

func (r *Reconciler) controlPlaneVersionPreflightCheck(cluster *clusterv1.Cluster, cpVersion, mpVersion string) preflightCheckErrorMessage {
	if feature.Gates.Enabled(feature.ClusterTopology) && cluster.Spec.Topology.IsDefined() {
		if cpVersion != mpVersion {
			return ptr.To(fmt.Sprintf("MachinePool version (%s) is not yet the same as the ControlPlane version (%s), waiting for version to be propagated to the MachinePool (%q preflight check failed)", mpVersion, cpVersion, clusterv1.MachinePoolPreflightCheckControlPlaneVersionSkew))
		}
	}

	return nil
}

func (r *Reconciler) skippedPreflightChecks(ctx context.Context, mp *clusterv1.MachinePool) (sets.Set[clusterv1.MachinePoolPreflightCheck], error) {
	skipped := sets.Set[clusterv1.MachinePoolPreflightCheck]{}
	if mp == nil {
		return skipped, nil
	}

	// Try to read skip annotation from MachinePool.
	skip := mp.Annotations[clusterv1.MachinePoolSkipPreflightChecksAnnotation]

	// Fallback to try to read skip annotation from BootstrapConfigTemplate.
	if skip == "" && mp.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		apiVersion, err := contract.GetAPIVersion(ctx, r.Client, mp.Spec.Template.Spec.Bootstrap.ConfigRef.GroupKind())
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to read %s annotation", clusterv1.MachinePoolSkipPreflightChecksAnnotation)
		}
		templateRef := &corev1.ObjectReference{
			APIVersion: apiVersion,
			Kind:       mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind,
			Namespace:  mp.Namespace,
			Name:       mp.Spec.Template.Spec.Bootstrap.ConfigRef.Name,
		}
		template, err := external.Get(ctx, r.Client, templateRef)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to read %s annotation", clusterv1.MachinePoolSkipPreflightChecksAnnotation)
		}
		skip = template.GetAnnotations()[clusterv1.MachinePoolSkipPreflightChecksAnnotation]
	}

	// Return early if skip annotation is not set
	if skip == "" {
		return skipped, nil
	}

	skippedList := strings.Split(skip, ",")
	for i := range skippedList {
		skipped.Insert(clusterv1.MachinePoolPreflightCheck(strings.TrimSpace(skippedList[i])))
	}
	return skipped, nil
}

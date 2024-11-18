/*
Copyright 2023 The Kubernetes Authors.

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
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
)

type preflightCheckErrorMessage *string

// preflightFailedRequeueAfter is used to requeue the MachineSet to re-verify the preflight checks if
// the preflight checks fail.
const preflightFailedRequeueAfter = 15 * time.Second

var minVerKubernetesKubeletVersionSkewThree = semver.MustParse("1.28.0")

func (r *Reconciler) runPreflightChecks(ctx context.Context, cluster *clusterv1.Cluster, ms *clusterv1.MachineSet, action string) ([]string, error) {
	log := ctrl.LoggerFrom(ctx)
	// If the MachineSetPreflightChecks feature gate is disabled return early.
	if !feature.Gates.Enabled(feature.MachineSetPreflightChecks) {
		return nil, nil
	}

	skipped := skippedPreflightChecks(ms)
	// If all the preflight checks are skipped then return early.
	if skipped.Has(clusterv1.MachineSetPreflightCheckAll) {
		return nil, nil
	}

	// If the cluster does not have a control plane reference then there is nothing to do. Return early.
	if cluster.Spec.ControlPlaneRef == nil {
		return nil, nil
	}

	// Get the control plane object.
	controlPlane, err := external.Get(ctx, r.Client, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to get ControlPlane %s", action, klog.KRef(cluster.Spec.ControlPlaneRef.Namespace, cluster.Spec.ControlPlaneRef.Name))
	}
	cpKlogRef := klog.KRef(controlPlane.GetNamespace(), controlPlane.GetName())

	// If the Control Plane version is not set then we are dealing with a control plane that does not support version
	// or a control plane where the version is not set. In both cases we cannot perform any preflight checks as
	// we do not have enough information. Return early.
	cpVersion, err := contract.ControlPlane().Version().Get(controlPlane)
	if err != nil {
		if errors.Is(err, contract.ErrFieldNotFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to get the version of ControlPlane %s", action, cpKlogRef)
	}
	cpSemver, err := semver.ParseTolerant(*cpVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to parse version %q of ControlPlane %s", action, *cpVersion, cpKlogRef)
	}

	errList := []error{}
	preflightCheckErrs := []preflightCheckErrorMessage{}
	// Run the control-plane-stable preflight check.
	if !skipped.Has(clusterv1.MachineSetPreflightCheckControlPlaneIsStable) {
		preflightCheckErr, err := r.controlPlaneStablePreflightCheck(controlPlane)
		if err != nil {
			errList = append(errList, err)
		}
		if preflightCheckErr != nil {
			preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
		}
	}

	// Check the version skew policies only if version is defined in the MachineSet.
	if ms.Spec.Template.Spec.Version != nil {
		msVersion := *ms.Spec.Template.Spec.Version
		msSemver, err := semver.ParseTolerant(msVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to perform %q: failed to perform preflight checks: failed to parse version %q of MachineSet %s", action, msVersion, klog.KObj(ms))
		}

		// Run the kubernetes-version skew preflight check.
		if !skipped.Has(clusterv1.MachineSetPreflightCheckKubernetesVersionSkew) {
			preflightCheckErr := r.kubernetesVersionPreflightCheck(cpSemver, msSemver)
			if preflightCheckErr != nil {
				preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
			}
		}

		// Run the kubeadm-version skew preflight check.
		if !skipped.Has(clusterv1.MachineSetPreflightCheckKubeadmVersionSkew) {
			preflightCheckErr, err := r.kubeadmVersionPreflightCheck(cpSemver, msSemver, ms)
			if err != nil {
				errList = append(errList, err)
			}
			if preflightCheckErr != nil {
				preflightCheckErrs = append(preflightCheckErrs, preflightCheckErr)
			}
		}
	}

	if len(errList) > 0 {
		return nil, errors.Wrapf(kerrors.NewAggregate(errList), "failed to perform %q: failed to perform preflight checks", action)
	}
	if len(preflightCheckErrs) > 0 {
		preflightCheckErrStrings := []string{}
		for _, v := range preflightCheckErrs {
			preflightCheckErrStrings = append(preflightCheckErrStrings, *v)
		}
		log.Info(fmt.Sprintf("%s on hold because %s. The operation will continue after the preflight check(s) pass", action, strings.Join(preflightCheckErrStrings, "; ")))
		return preflightCheckErrStrings, nil
	}
	return nil, nil
}

func (r *Reconciler) controlPlaneStablePreflightCheck(controlPlane *unstructured.Unstructured) (preflightCheckErrorMessage, error) {
	cpKlogRef := klog.KRef(controlPlane.GetNamespace(), controlPlane.GetName())

	// Check that the control plane is not provisioning.
	isProvisioning, err := contract.ControlPlane().IsProvisioning(controlPlane)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform %q preflight check: failed to check if %s %s is provisioning", clusterv1.MachineSetPreflightCheckControlPlaneIsStable, controlPlane.GetKind(), cpKlogRef)
	}
	if isProvisioning {
		return ptr.To(fmt.Sprintf("%s %s is provisioning (%q preflight check failed)", controlPlane.GetKind(), cpKlogRef, clusterv1.MachineSetPreflightCheckControlPlaneIsStable)), nil
	}

	// Check that the control plane is not upgrading.
	isUpgrading, err := contract.ControlPlane().IsUpgrading(controlPlane)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform %q preflight check: failed to check if the %s %s is upgrading", clusterv1.MachineSetPreflightCheckControlPlaneIsStable, controlPlane.GetKind(), cpKlogRef)
	}
	if isUpgrading {
		return ptr.To(fmt.Sprintf("%s %s is upgrading (%q preflight check failed)", controlPlane.GetKind(), cpKlogRef, clusterv1.MachineSetPreflightCheckControlPlaneIsStable)), nil
	}

	return nil, nil
}

func (r *Reconciler) kubernetesVersionPreflightCheck(cpSemver, msSemver semver.Version) preflightCheckErrorMessage {
	// Check the Kubernetes version skew policy.
	// => MS minor version cannot be greater than the Control Plane minor version.
	// => MS minor version cannot be outside of the supported skew.
	// Kubernetes skew policy: https://kubernetes.io/releases/version-skew-policy/#kubelet
	if msSemver.Minor > cpSemver.Minor {
		return ptr.To(fmt.Sprintf("MachineSet version (%s) and ControlPlane version (%s) do not conform to the kubernetes version skew policy as MachineSet version is higher than ControlPlane version (%q preflight check failed)", msSemver.String(), cpSemver.String(), clusterv1.MachineSetPreflightCheckKubernetesVersionSkew))
	}
	minorSkew := uint64(3)
	// For Control Planes running Kubernetes < v1.28, the version skew policy for kubelets is two.
	if cpSemver.LT(minVerKubernetesKubeletVersionSkewThree) {
		minorSkew = 2
	}
	if msSemver.Minor < cpSemver.Minor-minorSkew {
		return ptr.To(fmt.Sprintf("MachineSet version (%s) and ControlPlane version (%s) do not conform to the kubernetes version skew policy as MachineSet version is more than %d minor versions older than the ControlPlane version (%q preflight check failed)", msSemver.String(), cpSemver.String(), minorSkew, clusterv1.MachineSetPreflightCheckKubernetesVersionSkew))
	}

	return nil
}

func (r *Reconciler) kubeadmVersionPreflightCheck(cpSemver, msSemver semver.Version, ms *clusterv1.MachineSet) (preflightCheckErrorMessage, error) {
	// If the bootstrap.configRef is nil return early.
	if ms.Spec.Template.Spec.Bootstrap.ConfigRef == nil {
		return nil, nil
	}

	// If using kubeadm bootstrap provider, check the kubeadm version skew policy.
	// => MS version should match (major+minor) the Control Plane version.
	// kubeadm skew policy: https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#kubeadm-s-skew-against-kubeadm
	bootstrapConfigRef := ms.Spec.Template.Spec.Bootstrap.ConfigRef
	groupVersion, err := schema.ParseGroupVersion(bootstrapConfigRef.APIVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform %q preflight check: failed to parse bootstrap configRef APIVersion %s", clusterv1.MachineSetPreflightCheckKubeadmVersionSkew, bootstrapConfigRef.APIVersion)
	}
	kubeadmBootstrapProviderUsed := bootstrapConfigRef.Kind == "KubeadmConfigTemplate" &&
		groupVersion.Group == bootstrapv1.GroupVersion.Group
	if kubeadmBootstrapProviderUsed {
		if cpSemver.Minor != msSemver.Minor {
			return ptr.To(fmt.Sprintf("MachineSet version (%s) and ControlPlane version (%s) do not conform to kubeadm version skew policy as kubeadm only supports joining with the same major+minor version as the control plane (%q preflight check failed)", msSemver.String(), cpSemver.String(), clusterv1.MachineSetPreflightCheckKubeadmVersionSkew)), nil
		}
	}
	return nil, nil
}

func skippedPreflightChecks(ms *clusterv1.MachineSet) sets.Set[clusterv1.MachineSetPreflightCheck] {
	skipped := sets.Set[clusterv1.MachineSetPreflightCheck]{}
	if ms == nil {
		return skipped
	}
	skip := ms.Annotations[clusterv1.MachineSetSkipPreflightChecksAnnotation]
	if skip == "" {
		return skipped
	}
	skippedList := strings.Split(skip, ",")
	for i := range skippedList {
		skipped.Insert(clusterv1.MachineSetPreflightCheck(strings.TrimSpace(skippedList[i])))
	}
	return skipped
}

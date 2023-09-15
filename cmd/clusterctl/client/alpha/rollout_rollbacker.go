/*
Copyright 2020 The Kubernetes Authors.

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

package alpha

import (
	"context"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/version"
)

// ObjectRollbacker will issue a rollback on the specified cluster-api resource.
func (r *rollout) ObjectRollbacker(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference, toRevision int64, force bool) error {
	switch ref.Kind {
	case MachineDeployment:
		deployment, err := getMachineDeployment(ctx, proxy, ref.Name, ref.Namespace)
		if err != nil || deployment == nil {
			return errors.Wrapf(err, "failed to get %v/%v", ref.Kind, ref.Name)
		}
		if deployment.Spec.Paused {
			return errors.Errorf("can't rollback a paused MachineDeployment: please run 'clusterctl rollout resume %v/%v' first", ref.Kind, ref.Name)
		}
		if err := rollbackMachineDeployment(ctx, proxy, deployment, toRevision, force); err != nil {
			return err
		}
	default:
		return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validRollbackResourceTypes)
	}
	return nil
}

// rollbackMachineDeployment will rollback to a previous MachineSet revision used by this MachineDeployment.
func rollbackMachineDeployment(ctx context.Context, proxy cluster.Proxy, md *clusterv1.MachineDeployment, toRevision int64, force bool) error {
	log := logf.Log
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}

	if toRevision < 0 {
		return errors.Errorf("revision number cannot be negative: %v", toRevision)
	}
	msList, err := getMachineSetsForDeployment(ctx, proxy, md)
	if err != nil {
		return err
	}
	log.V(7).Info("Found MachineSets", "count", len(msList))
	msForRevision, err := findMachineDeploymentRevision(toRevision, msList)
	if err != nil {
		return err
	}
	log.V(7).Info("Found revision", "revision", msForRevision)

	if !force {
		if msForRevision.Spec.Template.Spec.Version == nil {
			return errors.Errorf("can't validate version skew policy because verion field of MachineSet %v is not set"+
				" The result of the operation may not comply with Kubernetes' version skew policy."+
				" If you want to rollback anyway, use --force option.", msForRevision.Name)
		}

		msVersion, err := version.ParseMajorMinorPatch(*msForRevision.Spec.Template.Spec.Version)
		if err != nil {
			return errors.Wrapf(err, "can't retrieve version from MachineSet: %v", msForRevision.Name)
		}

		var cpVersion semver.Version
		cluster, err := getCluster(ctx, proxy, md.Spec.ClusterName, md.Namespace)
		if err != nil {
			return err
		}
		ref := cluster.Spec.ControlPlaneRef
		switch strings.ToLower(ref.Kind) {
		case KubeadmControlPlane:
			kcp, err := getKubeadmControlPlane(ctx, proxy, ref.Name, ref.Namespace)
			if err != nil {
				return errors.Wrapf(err, "failed to fetch %v/%v", ref.Kind, ref.Name)
			}
			cpVersion, err = version.ParseMajorMinorPatch(kcp.Spec.Version)
			if err != nil {
				return errors.Wrapf(err, "can't retrieve version from KubeadmControlPlane: %v", kcp.Name)
			}
		default:
			return errors.Errorf("invalid resource type %q, valid values are %v", ref.Kind, validRollbackResourceTypes)
		}

		if err := validateVersionSkewPolicy(cpVersion, msVersion); err != nil {
			return err
		}
	}

	patchHelper, err := patch.NewHelper(md, c)
	if err != nil {
		return err
	}
	// Copy template into the machinedeployment (excluding the hash)
	revMSTemplate := *msForRevision.Spec.Template.DeepCopy()
	delete(revMSTemplate.Labels, clusterv1.MachineDeploymentUniqueLabel)

	md.Spec.Template = revMSTemplate
	return patchHelper.Patch(ctx, md)
}

func validateVersionSkewPolicy(cpVersion semver.Version, msVersion semver.Version) error {
	// More info: https://kubernetes.io/releases/version-skew-policy/"
	if cpVersion.Major != msVersion.Major || cpVersion.Minor < msVersion.Minor || cpVersion.Minor-msVersion.Minor > 2 {
		return errors.Errorf("version skew between ControlPlane %v and MachineSet %v is not supporeted by Kubernetes version skew policy:"+
			" MachineSet must not be newer than ControlPlane, and may be up to two minor versions older.\n"+
			"If you want to rollback anyway, use --force option.", cpVersion, msVersion)
	}
	return nil
}

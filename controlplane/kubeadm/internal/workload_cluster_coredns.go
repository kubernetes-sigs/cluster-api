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

package internal

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/coredns/corefile-migration/migration"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/kubeadm"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	corefileKey            = "Corefile"
	corefileBackupKey      = "Corefile-backup"
	coreDNSKey             = "coredns"
	coreDNSVolumeKey       = "config-volume"
	coreDNSClusterRoleName = "system:coredns"

	oldCoreDNSImageName = "coredns"
	coreDNSImageName    = "coredns/coredns"

	oldControlPlaneTaint = "node-role.kubernetes.io/master" // Deprecated: https://github.com/kubernetes/kubeadm/issues/2200
	controlPlaneTaint    = "node-role.kubernetes.io/control-plane"
)

var (
	// Source: https://github.com/kubernetes/kubernetes/blob/v1.22.0-beta.1/cmd/kubeadm/app/phases/addons/dns/manifests.go#L178-L207
	coreDNS181PolicyRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"endpoints", "services", "pods", "namespaces"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{""},
			Resources: []string{"nodes"},
		},
		{
			Verbs:     []string{"list", "watch"},
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
		},
	}
)

type coreDNSMigrator interface {
	Migrate(currentVersion string, toVersion string, corefile string, deprecations bool) (string, error)
}

// CoreDNSMigrator is a shim that can be used to migrate CoreDNS files from one version to another.
type CoreDNSMigrator struct{}

// Migrate calls the CoreDNS migration library to migrate a corefile.
func (c *CoreDNSMigrator) Migrate(fromCoreDNSVersion, toCoreDNSVersion, corefile string, deprecations bool) (string, error) {
	return migration.Migrate(fromCoreDNSVersion, toCoreDNSVersion, corefile, deprecations)
}

type coreDNSInfo struct {
	Corefile   string
	Deployment *appsv1.Deployment

	FromImageTag string
	ToImageTag   string

	CurrentMajorMinorPatch string
	TargetMajorMinorPatch  string

	FromImage string
	ToImage   string
}

// UpdateCoreDNS updates the kubeadm configmap, coredns corefile and coredns
// deployment.
func (w *Workload) UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, version semver.Version) error {
	// Return early if we've been asked to skip CoreDNS upgrades entirely.
	if _, ok := kcp.Annotations[controlplanev1.SkipCoreDNSAnnotation]; ok {
		return nil
	}

	// Return early if the configuration is nil.
	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration == nil {
		return nil
	}

	clusterConfig := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration

	// Get the CoreDNS info needed for the upgrade.
	info, err := w.getCoreDNSInfo(ctx, clusterConfig, version)
	if err != nil {
		// Return early if we get a not found error, this can happen if any of the CoreDNS components
		// cannot be found, e.g. configmap, deployment.
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
		return err
	}

	// Update the cluster role independent of image change. Kubernetes may get updated
	// to v1.22 which requires updating the cluster role without image changes.
	if err := w.updateCoreDNSClusterRole(ctx, version, info); err != nil {
		return err
	}

	// Return early if the from/to image is the same.
	if info.FromImage == info.ToImage {
		return nil
	}

	// Validate the image tag.
	if err := validateCoreDNSImageTag(info.FromImageTag, info.ToImageTag); err != nil {
		return errors.Wrapf(err, "failed to validate CoreDNS")
	}

	// Perform the upgrade.
	if err := w.UpdateClusterConfiguration(ctx, version, w.updateCoreDNSImageInfoInKubeadmConfigMap(&clusterConfig.DNS)); err != nil {
		return err
	}
	if err := w.updateCoreDNSCorefile(ctx, info); err != nil {
		return err
	}

	if err := w.updateCoreDNSDeployment(ctx, info, version); err != nil {
		return errors.Wrap(err, "unable to update coredns deployment")
	}
	return nil
}

// getCoreDNSInfo returns all necessary coredns based information.
func (w *Workload) getCoreDNSInfo(ctx context.Context, clusterConfig *bootstrapv1.ClusterConfiguration, version semver.Version) (*coreDNSInfo, error) {
	// Get the coredns configmap and corefile.
	key := ctrlclient.ObjectKey{Name: coreDNSKey, Namespace: metav1.NamespaceSystem}
	cm, err := w.getConfigMap(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting %v config map from target cluster", key)
	}
	corefile, ok := cm.Data[corefileKey]
	if !ok {
		return nil, errors.New("unable to find the CoreDNS Corefile data")
	}

	// Get the current CoreDNS deployment.
	deployment := &appsv1.Deployment{}
	if err := w.Client.Get(ctx, key, deployment); err != nil {
		return nil, errors.Wrapf(err, "unable to get %v deployment from target cluster", key)
	}

	var container *corev1.Container
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == coreDNSKey {
			container = c.DeepCopy()
			break
		}
	}
	if container == nil {
		return nil, errors.Errorf("failed to update coredns deployment: deployment spec has no %q container", coreDNSKey)
	}

	// Parse container image.
	parsedImage, err := containerutil.ImageFromString(container.Image)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse %q deployment image", container.Image)
	}

	// Handle imageRepository.
	toImageRepository := parsedImage.Repository
	// Overwrite the image repository if a value was explicitly set or an upgrade is required.
	if imageRegistryRepository := ImageRepositoryFromClusterConfig(clusterConfig, version); imageRegistryRepository != "" {
		if imageRegistryRepository == kubeadm.DefaultImageRepository {
			// Only patch to DefaultImageRepository if OldDefaultImageRepository is set as prefix.
			if strings.HasPrefix(toImageRepository, kubeadm.OldDefaultImageRepository) {
				// Ensure to keep the repository subpaths when patching from OldDefaultImageRepository to new DefaultImageRepository.
				toImageRepository = strings.TrimSuffix(imageRegistryRepository+strings.TrimPrefix(toImageRepository, kubeadm.OldDefaultImageRepository), "/")
			}
		} else {
			toImageRepository = strings.TrimSuffix(imageRegistryRepository, "/")
		}
	}
	if clusterConfig.DNS.ImageRepository != "" {
		toImageRepository = strings.TrimSuffix(clusterConfig.DNS.ImageRepository, "/")
	}

	// Handle imageTag.
	if parsedImage.Tag == "" {
		return nil, errors.Errorf("failed to update coredns deployment: does not have a valid image tag: %q", container.Image)
	}
	currentMajorMinorPatch, err := extractImageVersion(parsedImage.Tag)
	if err != nil {
		return nil, err
	}
	toImageTag := parsedImage.Tag
	if clusterConfig.DNS.ImageTag != "" {
		toImageTag = clusterConfig.DNS.ImageTag
	}
	targetMajorMinorPatch, err := extractImageVersion(toImageTag)
	if err != nil {
		return nil, err
	}

	// Handle the renaming of the upstream image from:
	// * "registry.k8s.io/coredns" to "registry.k8s.io/coredns/coredns" or
	// * "k8s.gcr.io/coredns" to "k8s.gcr.io/coredns/coredns"
	toImageName := parsedImage.Name
	if (toImageRepository == kubeadm.OldDefaultImageRepository || toImageRepository == kubeadm.DefaultImageRepository) &&
		toImageName == oldCoreDNSImageName && targetMajorMinorPatch.GTE(semver.MustParse("1.8.0")) {
		toImageName = coreDNSImageName
	}

	return &coreDNSInfo{
		Corefile:               corefile,
		Deployment:             deployment,
		CurrentMajorMinorPatch: currentMajorMinorPatch.String(),
		TargetMajorMinorPatch:  targetMajorMinorPatch.String(),
		FromImageTag:           parsedImage.Tag,
		ToImageTag:             toImageTag,
		FromImage:              container.Image,
		ToImage:                fmt.Sprintf("%s/%s:%s", toImageRepository, toImageName, toImageTag),
	}, nil
}

// updateCoreDNSDeployment will patch the deployment image to the
// imageRepo:imageTag in the KCP dns. It will also ensure the volume of the
// deployment uses the Corefile key of the coredns configmap.
func (w *Workload) updateCoreDNSDeployment(ctx context.Context, info *coreDNSInfo, kubernetesVersion semver.Version) error {
	helper, err := patch.NewHelper(info.Deployment, w.Client)
	if err != nil {
		return err
	}
	// Form the final image before issuing the patch.
	patchCoreDNSDeploymentImage(info.Deployment, info.ToImage)

	// Flip the deployment volume back to Corefile (from the backup key).
	patchCoreDNSDeploymentVolume(info.Deployment, corefileBackupKey, corefileKey)

	// Patch the tolerations according to the Kubernetes Version.
	patchCoreDNSDeploymentTolerations(info.Deployment, kubernetesVersion)
	return helper.Patch(ctx, info.Deployment)
}

// updateCoreDNSImageInfoInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) updateCoreDNSImageInfoInKubeadmConfigMap(dns *bootstrapv1.DNS) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.DNS.ImageRepository = dns.ImageRepository
		c.DNS.ImageTag = dns.ImageTag
	}
}

// updateCoreDNSClusterRole updates the CoreDNS ClusterRole when necessary.
// CoreDNS >= 1.8.1 uses EndpointSlices. kubeadm < 1.22 doesn't include the EndpointSlice rule in the CoreDNS ClusterRole.
// To support Kubernetes clusters >= 1.22 (which have been initialized with kubeadm < 1.22) with CoreDNS versions >= 1.8.1
// we have to update the ClusterRole accordingly.
func (w *Workload) updateCoreDNSClusterRole(ctx context.Context, kubernetesVersion semver.Version, info *coreDNSInfo) error {
	// Do nothing for Kubernetes < 1.22.
	if version.Compare(kubernetesVersion, semver.Version{Major: 1, Minor: 22, Patch: 0}, version.WithoutPreReleases()) < 0 {
		return nil
	}

	// Do nothing for CoreDNS < 1.8.1.
	targetCoreDNSVersion, err := extractImageVersion(info.ToImageTag)
	if err != nil {
		return err
	}
	if targetCoreDNSVersion.LT(semver.Version{Major: 1, Minor: 8, Patch: 1}) {
		return nil
	}

	sourceCoreDNSVersion, err := extractImageVersion(info.FromImageTag)
	if err != nil {
		return err
	}
	// Do nothing for Kubernetes > 1.22 and sourceCoreDNSVersion >= 1.8.1.
	// With those versions we know that the ClusterRole has already been updated,
	// as there must have been a previous upgrade to Kubernetes 1.22
	// (Kubernetes minor versions cannot be skipped) and to CoreDNS >= v1.8.1.
	if kubernetesVersion.GTE(semver.Version{Major: 1, Minor: 23, Patch: 0}) &&
		sourceCoreDNSVersion.GTE(semver.Version{Major: 1, Minor: 8, Patch: 1}) {
		return nil
	}

	key := ctrlclient.ObjectKey{Name: coreDNSClusterRoleName, Namespace: metav1.NamespaceSystem}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		currentClusterRole := &rbacv1.ClusterRole{}
		if err := w.Client.Get(ctx, key, currentClusterRole); err != nil {
			return fmt.Errorf("failed to get ClusterRole %q", coreDNSClusterRoleName)
		}

		if !semanticDeepEqualPolicyRules(currentClusterRole.Rules, coreDNS181PolicyRules) {
			currentClusterRole.Rules = coreDNS181PolicyRules
			if err := w.Client.Update(ctx, currentClusterRole); err != nil {
				return errors.Wrapf(err, "failed to update ClusterRole %q", coreDNSClusterRoleName)
			}
		}
		return nil
	})
}

func semanticDeepEqualPolicyRules(r1, r2 []rbacv1.PolicyRule) bool {
	return reflect.DeepEqual(generateClusterRolePolicies(r1), generateClusterRolePolicies(r2))
}

// generateClusterRolePolicies generates a nested map with the full data of an array of PolicyRules so it can
// be compared with reflect.DeepEqual. If we would use reflect.DeepEqual directly on the PolicyRule array,
// differences in the order of the array elements would lead to the arrays not being considered equal.
func generateClusterRolePolicies(policyRules []rbacv1.PolicyRule) map[string]map[string]map[string]struct{} {
	policies := map[string]map[string]map[string]struct{}{}
	for _, policyRule := range policyRules {
		for _, apiGroup := range policyRule.APIGroups {
			if _, ok := policies[apiGroup]; !ok {
				policies[apiGroup] = map[string]map[string]struct{}{}
			}

			for _, resource := range policyRule.Resources {
				if _, ok := policies[apiGroup][resource]; !ok {
					policies[apiGroup][resource] = map[string]struct{}{}
				}

				for _, verb := range policyRule.Verbs {
					policies[apiGroup][resource][verb] = struct{}{}
				}
			}
		}
	}
	return policies
}

// updateCoreDNSCorefile migrates the coredns corefile if there is an increase
// in version number. It also creates a corefile backup and patches the
// deployment to point to the backup corefile before migrating.
func (w *Workload) updateCoreDNSCorefile(ctx context.Context, info *coreDNSInfo) error {
	// Run the CoreDNS migration tool first because if it cannot migrate the
	// corefile, then there's no point in continuing further.
	updatedCorefile, err := w.CoreDNSMigrator.Migrate(info.CurrentMajorMinorPatch, info.TargetMajorMinorPatch, info.Corefile, false)
	if err != nil {
		return errors.Wrap(err, "unable to migrate CoreDNS corefile")
	}

	// First we backup the Corefile by backing it up.
	if err := w.Client.Update(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			corefileKey:       info.Corefile,
			corefileBackupKey: info.Corefile,
		},
	}); err != nil {
		return errors.Wrap(err, "unable to update CoreDNS config map with backup Corefile")
	}

	// Patching the coredns deployment to point to the Corefile-backup
	// contents before performing the migration.
	helper, err := patch.NewHelper(info.Deployment, w.Client)
	if err != nil {
		return err
	}
	patchCoreDNSDeploymentVolume(info.Deployment, corefileKey, corefileBackupKey)
	if err := helper.Patch(ctx, info.Deployment); err != nil {
		return err
	}

	if err := w.Client.Update(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coreDNSKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			corefileKey:       updatedCorefile,
			corefileBackupKey: info.Corefile,
		},
	}); err != nil {
		return errors.Wrap(err, "unable to update CoreDNS config map")
	}

	return nil
}

func patchCoreDNSDeploymentVolume(deployment *appsv1.Deployment, fromKey, toKey string) {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == coreDNSVolumeKey && volume.ConfigMap != nil && volume.ConfigMap.Name == coreDNSKey {
			for i, item := range volume.ConfigMap.Items {
				if item.Key == fromKey || item.Key == toKey {
					volume.ConfigMap.Items[i].Key = toKey
				}
			}
		}
	}
}

func patchCoreDNSDeploymentImage(deployment *appsv1.Deployment, image string) {
	containers := deployment.Spec.Template.Spec.Containers
	for idx, c := range containers {
		if c.Name == coreDNSKey {
			containers[idx].Image = image
		}
	}
}

// patchCoreDNSDeploymentTolerations patches the CoreDNS Deployment to make sure
// it has the right control plane tolerations.
// Kubernetes nodes created with kubeadm have the following taints depending on version:
// * -v1.23: only old taint is set
// * v1.24: both taints are set
// * v1.25+: only new taint is set
// To be absolutely safe this func will ensure that both tolerations are present
// for Kubernetes < v1.26.0. Starting with v1.26.0 we will only set the new toleration.
func patchCoreDNSDeploymentTolerations(deployment *appsv1.Deployment, kubernetesVersion semver.Version) {
	// We always add the toleration for the new control plane taint.
	tolerations := []corev1.Toleration{
		{
			Key:    controlPlaneTaint,
			Effect: corev1.TaintEffectNoSchedule,
		},
	}

	// We add the toleration for the old control plane taint for Kubernetes < v1.26.0.
	if kubernetesVersion.LT(semver.Version{Major: 1, Minor: 26, Patch: 0}) {
		tolerations = append(tolerations, corev1.Toleration{
			Key:    oldControlPlaneTaint,
			Effect: corev1.TaintEffectNoSchedule,
		})
	}

	// Add all other already existing tolerations.
	for _, currentToleration := range deployment.Spec.Template.Spec.Tolerations {
		// Skip the old control plane toleration as it has been already added above,
		// for Kubernetes < v1.26.0.
		if currentToleration.Key == oldControlPlaneTaint &&
			currentToleration.Effect == corev1.TaintEffectNoSchedule &&
			currentToleration.Value == "" {
			continue
		}

		// Skip the new control plane toleration as it has been already added above.
		if currentToleration.Key == controlPlaneTaint &&
			currentToleration.Effect == corev1.TaintEffectNoSchedule &&
			currentToleration.Value == "" {
			continue
		}

		tolerations = append(tolerations, currentToleration)
	}

	deployment.Spec.Template.Spec.Tolerations = tolerations
}

func extractImageVersion(tag string) (semver.Version, error) {
	ver, err := version.ParseMajorMinorPatchTolerant(tag)
	if err != nil {
		return semver.Version{}, errors.Wrapf(err, "error parsing semver from %q", tag)
	}
	return ver, nil
}

// validateCoreDNSImageTag returns error if the versions don't meet requirements.
// Some of the checks come from
// https://github.com/coredns/corefile-migration/blob/v1.0.6/migration/migrate.go#L414
func validateCoreDNSImageTag(fromTag, toTag string) error {
	from, err := version.ParseMajorMinorPatchTolerant(fromTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS current version %q", fromTag)
	}
	to, err := version.ParseMajorMinorPatchTolerant(toTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS target version %q", toTag)
	}
	// make sure that the version we're upgrading to is greater or equal to the current one,
	if x := from.Compare(to); x > 0 {
		return fmt.Errorf("toVersion %q must be greater than or equal to fromVersion %q", toTag, fromTag)
	}
	// check if the from version is even in the list of coredns versions
	if _, ok := migration.Versions[fmt.Sprintf("%d.%d.%d", from.Major, from.Minor, from.Patch)]; !ok {
		return fmt.Errorf("fromVersion %q is not a compatible coredns version", from.String())
	}
	return nil
}

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
	"strings"

	"github.com/blang/semver/v4"
	"github.com/coredns/corefile-migration/migration"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	corefileKey       = "Corefile"
	corefileBackupKey = "Corefile-backup"
	coreDNSKey        = "coredns"
	coreDNSVolumeKey  = "config-volume"

	oldCoreDNSImageName = "coredns"
	coreDNSImageName    = "coredns/coredns"
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
func (w *Workload) UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error {
	// Return early if we've been asked to skip CoreDNS upgrades entirely.
	if _, ok := kcp.Annotations[controlplanev1.SkipCoreDNSAnnotation]; ok {
		return nil
	}

	// Return early if the configuration is nil.
	if !kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.IsDefined() {
		return nil
	}

	clusterConfig := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration

	// Get the CoreDNS info needed for the upgrade.
	info, err := w.getCoreDNSInfo(ctx, clusterConfig)
	if err != nil {
		// Return early if we get a not found error, this can happen if any of the CoreDNS components
		// cannot be found, e.g. configmap, deployment.
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil
		}
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

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Kubernetes version %q", kcp.Spec.Version)
	}

	// Perform the upgrade.
	if err := w.UpdateClusterConfiguration(ctx, parsedVersion, w.updateCoreDNSImageInfoInKubeadmConfigMap(&clusterConfig.DNS)); err != nil {
		return err
	}
	if err := w.updateCoreDNSCorefile(ctx, info); err != nil {
		return err
	}

	if err := w.updateCoreDNSDeployment(ctx, info); err != nil {
		return errors.Wrap(err, "unable to update coredns deployment")
	}
	return nil
}

// getCoreDNSInfo returns all necessary coredns based information.
func (w *Workload) getCoreDNSInfo(ctx context.Context, clusterConfig bootstrapv1.ClusterConfiguration) (*coreDNSInfo, error) {
	// Get the CoreDNS configmap and corefile.
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
	if imageRegistryRepository := ImageRepositoryFromClusterConfig(clusterConfig); imageRegistryRepository != "" {
		toImageRepository = strings.TrimSuffix(imageRegistryRepository, "/")
	}
	if clusterConfig.DNS.ImageRepository != "" {
		toImageRepository = strings.TrimSuffix(clusterConfig.DNS.ImageRepository, "/")
	}

	// Handle imageTag.
	if parsedImage.Tag == "" {
		return nil, errors.Errorf("failed to update coredns deployment: does not have a valid image tag: %q", container.Image)
	}
	currentVersion, err := version.ParseTolerantImageTag(parsedImage.Tag)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing semver from %q", parsedImage.Tag)
	}
	toImageTag := parsedImage.Tag
	if clusterConfig.DNS.ImageTag != "" {
		toImageTag = clusterConfig.DNS.ImageTag
	}
	targetVersion, err := version.ParseTolerantImageTag(toImageTag)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing semver from %q", toImageTag)
	}

	// Handle the renaming of the upstream image from:
	// * "registry.k8s.io/coredns" to "registry.k8s.io/coredns/coredns" or
	// * "k8s.gcr.io/coredns" to "k8s.gcr.io/coredns/coredns"
	toImageName := parsedImage.Name
	if (toImageRepository == "k8s.gcr.io" || toImageRepository == "registry.k8s.io") &&
		toImageName == oldCoreDNSImageName && version.Compare(targetVersion, semver.MustParse("1.8.0"), version.WithoutPreReleases()) >= 0 {
		toImageName = coreDNSImageName
	}

	return &coreDNSInfo{
		Corefile:               corefile,
		Deployment:             deployment,
		CurrentMajorMinorPatch: version.MajorMinorPatch(currentVersion).String(),
		TargetMajorMinorPatch:  version.MajorMinorPatch(targetVersion).String(),
		FromImageTag:           parsedImage.Tag,
		ToImageTag:             toImageTag,
		FromImage:              container.Image,
		ToImage:                fmt.Sprintf("%s/%s:%s", toImageRepository, toImageName, toImageTag),
	}, nil
}

// updateCoreDNSDeployment will patch the deployment image to the
// imageRepo:imageTag in the KCP dns. It will also ensure the volume of the
// deployment uses the Corefile key of the coredns configmap.
func (w *Workload) updateCoreDNSDeployment(ctx context.Context, info *coreDNSInfo) error {
	helper, err := patch.NewHelper(info.Deployment, w.Client)
	if err != nil {
		return err
	}
	// Form the final image before issuing the patch.
	patchCoreDNSDeploymentImage(info.Deployment, info.ToImage)

	// Flip the deployment volume back to Corefile (from the backup key).
	patchCoreDNSDeploymentVolume(info.Deployment, corefileBackupKey, corefileKey)

	return helper.Patch(ctx, info.Deployment)
}

// updateCoreDNSImageInfoInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) updateCoreDNSImageInfoInKubeadmConfigMap(dns *bootstrapv1.DNS) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.DNS.ImageRepository = dns.ImageRepository
		c.DNS.ImageTag = dns.ImageTag
	}
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

// validateCoreDNSImageTag returns error if the versions don't meet requirements.
// Some of the checks come from
// https://github.com/coredns/corefile-migration/blob/v1.0.6/migration/migrate.go#L414
func validateCoreDNSImageTag(fromTag, toTag string) error {
	from, err := version.ParseTolerantImageTag(fromTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS current version %q", fromTag)
	}
	to, err := version.ParseTolerantImageTag(toTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS target version %q", toTag)
	}
	// make sure that the version we're upgrading to is greater or equal to the current one,
	if version.Compare(from, to, version.WithoutPreReleases()) > 0 {
		return fmt.Errorf("toVersion %q must be greater than or equal to fromVersion %q", toTag, fromTag)
	}
	// check if the from version is even in the list of coredns versions
	if _, ok := migration.Versions[fmt.Sprintf("%d.%d.%d", from.Major, from.Minor, from.Patch)]; !ok {
		return fmt.Errorf("fromVersion %q is not a compatible coredns version", from.String())
	}
	return nil
}

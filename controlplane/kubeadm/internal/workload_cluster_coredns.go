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

	"github.com/blang/semver"
	"github.com/coredns/corefile-migration/migration"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	corefileKey            = "Corefile"
	corefileBackupKey      = "Corefile-backup"
	coreDNSKey             = "coredns"
	coreDNSVolumeKey       = "config-volume"
	coreDNSClusterRoleName = "system:coredns"

	kubernetesImageRepository = "k8s.gcr.io"
	oldCoreDNSImageName       = "coredns"
	coreDNSImageName          = "coredns/coredns"
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

type CoreDNSMigrator struct{}

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
	// Return early if the type is anything other than empty (default), or CoreDNS.
	if clusterConfig.DNS.Type != "" && clusterConfig.DNS.Type != kubeadmv1.CoreDNS {
		return nil
	}

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

	// Perform the upgrade.
	if err := w.updateCoreDNSImageInfoInKubeadmConfigMap(ctx, &clusterConfig.DNS); err != nil {
		return err
	}
	if err := w.updateCoreDNSCorefile(ctx, info); err != nil {
		return err
	}
	if err := w.updateCoreDNSClusterRole(ctx, version, info); err != nil {
		return err
	}
	if err := w.updateCoreDNSDeployment(ctx, info); err != nil {
		return errors.Wrap(err, "unable to update coredns deployment")
	}
	return nil
}

// getCoreDNSInfo returns all necessary coredns based information.
func (w *Workload) getCoreDNSInfo(ctx context.Context, clusterConfig *kubeadmv1.ClusterConfiguration) (*coreDNSInfo, error) {
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
	if clusterConfig.ImageRepository != "" {
		toImageRepository = strings.TrimSuffix(clusterConfig.ImageRepository, "/")
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

	// Handle the renaming of the upstream image from "k8s.gcr.io/coredns" to "k8s.gcr.io/coredns/coredns"
	toImageName := parsedImage.Name
	if toImageRepository == kubernetesImageRepository && toImageName == oldCoreDNSImageName && targetMajorMinorPatch.GTE(semver.MustParse("1.8.0")) {
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

// UpdateCoreDNSDeployment will patch the deployment image to the
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

// UpdateCoreDNSImageInfoInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) updateCoreDNSImageInfoInKubeadmConfigMap(ctx context.Context, dns *kubeadmv1.DNS) error {
	configMapKey := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	if err := config.UpdateCoreDNSImageInfo(dns.ImageRepository, dns.ImageTag); err != nil {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// updateCoreDNSClusterRole updates the CoreDNS ClusterRole when necessary.
// CoreDNS >= 1.8.1 uses EndpointSlices. kubeadm < 1.22 doesn't include the EndpointSlice rule in the CoreDNS ClusterRole.
// To support Kubernetes clusters >= 1.22 (which have been initialized with kubeadm < 1.22) with CoreDNS versions >= 1.8.1
// we have to update the ClusterRole accordingly.
func (w *Workload) updateCoreDNSClusterRole(ctx context.Context, kubernetesVersion semver.Version, info *coreDNSInfo) error {
	// Do nothing for Kubernetes < 1.22.
	if kubernetesVersion.LT(semver.Version{Major: 1, Minor: 22, Patch: 0}) {
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

func extractImageVersion(tag string) (semver.Version, error) {
	ver, err := util.ParseMajorMinorPatch(tag)
	if err != nil {
		return semver.Version{}, err
	}
	return ver, nil
}

// validateCoreDNSImageTag returns error if the versions don't meet requirements.
// Some of the checks come from
// https://github.com/coredns/corefile-migration/blob/v1.0.6/migration/migrate.go#L414
func validateCoreDNSImageTag(fromTag, toTag string) error {
	from, err := util.ParseMajorMinorPatch(fromTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS current version %q", fromTag)
	}
	to, err := util.ParseMajorMinorPatch(toTag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse CoreDNS target version %q", toTag)
	}
	// make sure that the version we're upgrading to is greater than the current one,
	// or if they're the same version, the raw tags should be different (e.g. allow from `v1.17.4-somevendor.0` to `v1.17.4-somevendor.1`).
	if x := from.Compare(to); x > 0 || (x == 0 && fromTag == toTag) {
		return fmt.Errorf("toVersion %q must be greater than fromVersion %q", toTag, fromTag)
	}
	// check if the from version is even in the list of coredns versions
	if _, ok := migration.Versions[fmt.Sprintf("%d.%d.%d", from.Major, from.Minor, from.Patch)]; !ok {
		return fmt.Errorf("fromVersion %q is not a compatible coredns version", from.String())
	}
	return nil
}

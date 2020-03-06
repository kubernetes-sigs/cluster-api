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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/yaml"
)

const (
	clusterStatusKey        = "ClusterStatus"
	clusterConfigurationKey = "ClusterConfiguration"
	statusAPIEndpointsKey   = "apiEndpoints"
	configVersionKey        = "kubernetesVersion"
	dnsKey                  = "dns"
	dnsTypeKey              = "type"
	dnsImageRepositoryKey   = "imageRepository"
	dnsImageTagKey          = "imageTag"
)

// kubeadmConfig wraps up interactions necessary for modifying the kubeadm config during an upgrade.
type kubeadmConfig struct {
	ConfigMap *corev1.ConfigMap
}

// RemoveAPIEndpoint removes an APIEndpoint fromt he kubeadm config cluster status config map
func (k *kubeadmConfig) RemoveAPIEndpoint(endpoint string) error {
	data, ok := k.ConfigMap.Data[clusterStatusKey]
	if !ok {
		return errors.Errorf("could not find key %q in kubeadm config", clusterStatusKey)
	}
	status, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrap(err, "unable to convert YAML to unstructured")
	}
	endpoints, _, err := unstructured.NestedMap(status.UnstructuredContent(), statusAPIEndpointsKey)
	if err != nil {
		return errors.Wrap(err, "unable to extract apiEndpoints from kubeadm config map")
	}
	delete(endpoints, endpoint)
	if err := unstructured.SetNestedMap(status.UnstructuredContent(), endpoints, statusAPIEndpointsKey); err != nil {
		return errors.Wrap(err, "unable to set apiEndpoints on kubeadm config map")
	}
	updated, err := yaml.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "error encoding kubeadm ClusterStatus object")
	}
	k.ConfigMap.Data[clusterStatusKey] = string(updated)
	return nil
}

// UpdateKubernetesVersion changes the kubernetes version found in the kubeadm config map
func (k *kubeadmConfig) UpdateKubernetesVersion(version string) error {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return errors.Errorf("could not find key %q in kubeadm config", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrap(err, "unable to convert YAML to unstructured")
	}
	if err := unstructured.SetNestedField(configuration.UnstructuredContent(), version, configVersionKey); err != nil {
		return errors.Wrap(err, "unable to update kubernetes version on kubeadm config map")
	}
	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return errors.Wrap(err, "error encoding kubeadm cluster configuration object")
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return nil
}

// UpdateEtcdMeta sets the local etcd's configuration's image repository and image tag
func (k *kubeadmConfig) UpdateEtcdMeta(imageRepository, imageTag string) (bool, error) {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return false, errors.Errorf("could not find key %q in kubeadm config", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return false, errors.Wrap(err, "unable to convert YAML to unstructured")
	}

	var changed bool

	// Handle etcd.local.imageRepository.
	imageRepositoryPath := []string{"etcd", "local", "imageRepository"}
	currentImageRepository, _, err := unstructured.NestedString(configuration.UnstructuredContent(), imageRepositoryPath...)
	if err != nil {
		return false, errors.Wrap(err, "unable to retrieve current image repository from kubeadm configmap")
	}
	if currentImageRepository != imageRepository {
		if err := unstructured.SetNestedField(configuration.UnstructuredContent(), imageRepository, imageRepositoryPath...); err != nil {
			return false, errors.Wrap(err, "unable to update etcd.local.imageRepository on kubeadm configmap")
		}
		changed = true
	}

	// Handle etcd.local.imageTag.
	imageTagPath := []string{"etcd", "local", "imageTag"}
	currentImageTag, _, err := unstructured.NestedString(configuration.UnstructuredContent(), imageTagPath...)
	if err != nil {
		return false, errors.Wrap(err, "unable to retrieve current image repository from kubeadm configmap")
	}
	if currentImageTag != imageTag {
		if err := unstructured.SetNestedField(configuration.UnstructuredContent(), imageTag, imageTagPath...); err != nil {
			return false, errors.Wrap(err, "unable to update etcd.local.imageTag on kubeadm configmap")
		}
		changed = true
	}

	// Return early if no changes have been performed.
	if !changed {
		return changed, nil
	}

	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return false, errors.Wrap(err, "error encoding kubeadm cluster configuration object")
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return changed, nil
}

// UpdateCoreDNSImageInfo changes the dns.ImageTag and dns.ImageRepository
// found in the kubeadm config map
func (k *kubeadmConfig) UpdateCoreDNSImageInfo(repository, tag string) error {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return errors.Errorf("could not find key %q in kubeadm config", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrap(err, "unable to convert YAML to unstructured")
	}
	dnsMap := map[string]string{
		dnsTypeKey:            string(kubeadmv1.CoreDNS),
		dnsImageRepositoryKey: repository,
		dnsImageTagKey:        tag,
	}
	if err := unstructured.SetNestedStringMap(configuration.UnstructuredContent(), dnsMap, dnsKey); err != nil {
		return errors.Wrap(err, "unable to update dns on kubeadm config map")
	}
	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return errors.Wrap(err, "error encoding kubeadm cluster configuration object")
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return nil
}

// yamlToUnstructured looks inside a config map for a specific key and extracts the embedded YAML into an
// *unstructured.Unstructured.
func yamlToUnstructured(rawYAML []byte) (*unstructured.Unstructured, error) {
	unst := &unstructured.Unstructured{}
	err := yaml.Unmarshal(rawYAML, unst)
	return unst, err
}

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
	"sigs.k8s.io/yaml"
)

const (
	clusterStatusKey        = "ClusterStatus"
	clusterConfigurationKey = "ClusterConfiguration"
	statusAPIEndpointsKey   = "apiEndpoints"
	configVersionKey        = "kubernetesVersion"
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

// yamlToUnstructured looks inside a config map for a specific key and extracts the embedded YAML into an
// *unstructured.Unstructured.
func yamlToUnstructured(rawYAML []byte) (*unstructured.Unstructured, error) {
	unst := &unstructured.Unstructured{}
	err := yaml.Unmarshal(rawYAML, unst)
	return unst, err
}

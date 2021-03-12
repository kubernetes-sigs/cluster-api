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
	"reflect"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/yaml"
)

const (
	clusterStatusKey         = "ClusterStatus"
	clusterConfigurationKey  = "ClusterConfiguration"
	apiVersionKey            = "apiVersion"
	statusAPIEndpointsKey    = "apiEndpoints"
	configVersionKey         = "kubernetesVersion"
	dnsKey                   = "dns"
	dnsTypeKey               = "type"
	dnsImageRepositoryKey    = "imageRepository"
	dnsImageTagKey           = "imageTag"
	configImageRepositoryKey = "imageRepository"
	apiServerKey             = "apiServer"
	controllerManagerKey     = "controllerManager"
	schedulerKey             = "scheduler"
)

// kubeadmConfig wraps up interactions necessary for modifying the kubeadm config during an upgrade.
type kubeadmConfig struct {
	ConfigMap *corev1.ConfigMap
}

// RemoveAPIEndpoint removes an APIEndpoint fromt he kubeadm config cluster status config map
func (k *kubeadmConfig) RemoveAPIEndpoint(endpoint string) error {
	data, ok := k.ConfigMap.Data[clusterStatusKey]
	if !ok {
		return errors.Errorf("unable to find %q key in kubeadm ConfigMap", clusterStatusKey)
	}
	status, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterStatusKey)
	}
	endpoints, _, err := unstructured.NestedMap(status.UnstructuredContent(), statusAPIEndpointsKey)
	if err != nil {
		return errors.Wrapf(err, "unable to extract %q from kubeadm ConfigMap's %q", statusAPIEndpointsKey, clusterStatusKey)
	}
	delete(endpoints, endpoint)
	if err := unstructured.SetNestedMap(status.UnstructuredContent(), endpoints, statusAPIEndpointsKey); err != nil {
		return errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap's %q", statusAPIEndpointsKey, clusterStatusKey)
	}
	updated, err := yaml.Marshal(status)
	if err != nil {
		return errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterStatusKey)
	}
	k.ConfigMap.Data[clusterStatusKey] = string(updated)
	return nil
}

// UpdateKubernetesVersion changes the kubernetes version found in the kubeadm config map
func (k *kubeadmConfig) UpdateKubernetesVersion(version string) error {
	if k.ConfigMap == nil {
		return errors.New("unable to operate on a nil config map")
	}
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return errors.Errorf("unable to find %q key in kubeadm ConfigMap", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterConfigurationKey)
	}
	if err := unstructured.SetNestedField(configuration.UnstructuredContent(), version, configVersionKey); err != nil {
		return errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap's %q", configVersionKey, clusterConfigurationKey)
	}

	// Fix the ClusterConfiguration according to the target Kubernetes version
	// IMPORTANT: This is a stop-gap explicitly designed for back-porting on the v1alpha3 branch.
	// This allows to unblock removal of the v1beta1 API in kubeadm by making Cluster API to use the v1beta2 kubeadm API
	// under the assumption that the serialized version of the two APIs is equal as discussed; see
	// "Insulate users from kubeadm API version changes" CAEP for more details.
	// NOTE: This solution will stop to work when kubeadm will drop then v1beta2 kubeadm API, but this gives
	// enough time (9/12 months from the deprecation date, not yet announced) for the users to migrate to
	// the v1alpha4 release of Cluster API, where a proper conversion mechanism is going to be supported.
	gv, err := kubeadmv1.KubeVersionToKubeadmAPIGroupVersion(version)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(configuration.UnstructuredContent(), gv.String(), apiVersionKey); err != nil {
		return errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap's %q", apiVersionKey, clusterConfigurationKey)
	}

	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterConfigurationKey)
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return nil
}

// UpdateImageRepository changes the image repository found in the kubeadm config map
func (k *kubeadmConfig) UpdateImageRepository(imageRepository string) error {
	if imageRepository == "" {
		return nil
	}
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return errors.Errorf("unable to find %q key in kubeadm ConfigMap", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterConfigurationKey)
	}
	if err := unstructured.SetNestedField(configuration.UnstructuredContent(), imageRepository, configImageRepositoryKey); err != nil {
		return errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap's %q", imageRepository, clusterConfigurationKey)
	}
	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterConfigurationKey)
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return nil
}

// UpdateEtcdMeta sets the local etcd's configuration's image repository and image tag
func (k *kubeadmConfig) UpdateEtcdMeta(imageRepository, imageTag string) (bool, error) {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return false, errors.Errorf("unable to find %q in kubeadm ConfigMap", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return false, errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterConfigurationKey)
	}

	var changed bool

	// Handle etcd.local.imageRepository.
	imageRepositoryPath := []string{"etcd", "local", "imageRepository"}
	currentImageRepository, _, err := unstructured.NestedString(configuration.UnstructuredContent(), imageRepositoryPath...)
	if err != nil {
		return false, errors.Wrapf(err, "unable to retrieve %q from kubeadm ConfigMap", strings.Join(imageRepositoryPath, "."))
	}
	if currentImageRepository != imageRepository {
		if err := unstructured.SetNestedField(configuration.UnstructuredContent(), imageRepository, imageRepositoryPath...); err != nil {
			return false, errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap", strings.Join(imageRepositoryPath, "."))
		}
		changed = true
	}

	// Handle etcd.local.imageTag.
	imageTagPath := []string{"etcd", "local", "imageTag"}
	currentImageTag, _, err := unstructured.NestedString(configuration.UnstructuredContent(), imageTagPath...)
	if err != nil {
		return false, errors.Wrapf(err, "unable to retrieve %q from kubeadm ConfigMap", strings.Join(imageTagPath, "."))
	}
	if currentImageTag != imageTag {
		if err := unstructured.SetNestedField(configuration.UnstructuredContent(), imageTag, imageTagPath...); err != nil {
			return false, errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap", strings.Join(imageTagPath, "."))
		}
		changed = true
	}

	// Return early if no changes have been performed.
	if !changed {
		return changed, nil
	}

	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return false, errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterConfigurationKey)
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return changed, nil
}

// UpdateCoreDNSImageInfo changes the dns.ImageTag and dns.ImageRepository
// found in the kubeadm config map
func (k *kubeadmConfig) UpdateCoreDNSImageInfo(repository, tag string) error {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return errors.Errorf("unable to find %q in kubeadm ConfigMap", clusterConfigurationKey)
	}
	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterConfigurationKey)
	}
	dnsMap := map[string]string{
		dnsTypeKey:            string(kubeadmv1.CoreDNS),
		dnsImageRepositoryKey: repository,
		dnsImageTagKey:        tag,
	}
	if err := unstructured.SetNestedStringMap(configuration.UnstructuredContent(), dnsMap, dnsKey); err != nil {
		return errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap", dnsKey)
	}
	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterConfigurationKey)
	}
	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return nil
}

// UpdateAPIServer sets the api server configuration to values set in `apiServer` in kubeadm config map.
func (k *kubeadmConfig) UpdateAPIServer(apiServer kubeadmv1.APIServer) (bool, error) {
	changed, err := k.updateClusterConfiguration(apiServer, apiServerKey)
	if err != nil {
		return false, errors.Wrap(err, "unable to update api server configuration in kubeadm config map")
	}
	return changed, nil
}

// UpdateControllerManager sets the controller manager configuration to values set in `controllerManager` in kubeadm config map.
func (k *kubeadmConfig) UpdateControllerManager(controllerManager kubeadmv1.ControlPlaneComponent) (bool, error) {
	changed, err := k.updateClusterConfiguration(controllerManager, controllerManagerKey)
	if err != nil {
		return false, errors.Wrap(err, "unable to update controller manager configuration in kubeadm config map")
	}
	return changed, nil
}

// UpdateScheduler sets the scheduler configuration to values set in `scheduler` in kubeadm config map.
func (k *kubeadmConfig) UpdateScheduler(scheduler kubeadmv1.ControlPlaneComponent) (bool, error) {
	changed, err := k.updateClusterConfiguration(scheduler, schedulerKey)
	if err != nil {
		return false, errors.Wrap(err, "unable to update scheduler configuration in kubeadm config map")
	}
	return changed, nil
}

// updateClusterConfiguration is a generic method to update any kubeadm ClusterConfiguration spec with custom types in the specified path.
func (k *kubeadmConfig) updateClusterConfiguration(config interface{}, path ...string) (bool, error) {
	data, ok := k.ConfigMap.Data[clusterConfigurationKey]
	if !ok {
		return false, errors.Errorf("unable to find %q in kubeadm ConfigMap", clusterConfigurationKey)
	}

	configuration, err := yamlToUnstructured([]byte(data))
	if err != nil {
		return false, errors.Wrapf(err, "unable to decode kubeadm ConfigMap's %q to Unstructured object", clusterConfigurationKey)
	}

	currentConfig, _, err := unstructured.NestedFieldCopy(configuration.UnstructuredContent(), path...)
	if err != nil {
		return false, errors.Wrapf(err, "unable to retrieve %q from kubeadm ConfigMap", strings.Join(path, "."))
	}

	// convert config to map[string]interface because unstructured.SetNestedField does not accept custom structs.
	newConfig, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&config)
	if err != nil {
		return false, errors.Wrap(err, "unable to convert config to unstructured")
	}

	// if there are no changes, return early.
	if reflect.DeepEqual(newConfig, currentConfig) {
		return false, nil
	}

	if err := unstructured.SetNestedField(configuration.UnstructuredContent(), newConfig, path...); err != nil {
		return false, errors.Wrapf(err, "unable to update %q on kubeadm ConfigMap", strings.Join(path, "."))
	}

	updated, err := yaml.Marshal(configuration)
	if err != nil {
		return false, errors.Wrapf(err, "unable to encode kubeadm ConfigMap's %q to YAML", clusterConfigurationKey)
	}

	k.ConfigMap.Data[clusterConfigurationKey] = string(updated)
	return true, nil
}

// yamlToUnstructured looks inside a config map for a specific key and extracts the embedded YAML into an
// *unstructured.Unstructured.
func yamlToUnstructured(rawYAML []byte) (*unstructured.Unstructured, error) {
	unst := &unstructured.Unstructured{}
	err := yaml.Unmarshal(rawYAML, unst)
	return unst, err
}

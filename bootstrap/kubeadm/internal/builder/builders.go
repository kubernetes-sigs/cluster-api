/*
Copyright 2021 The Kubernetes Authors.

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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

// KubeadmConfigBuilder contains the information needed to produce a KubeadmConfig.
type KubeadmConfigBuilder struct {
	name          string
	namespace     string
	joinConfig    *bootstrapv1.JoinConfiguration
	initConfig    *bootstrapv1.InitConfiguration
	clusterConfig *bootstrapv1.ClusterConfiguration
}

// KubeadmConfig returns a KubeadmConfigBuilder with the supplied name and namespace.
func KubeadmConfig(namespace, name string) *KubeadmConfigBuilder {
	return &KubeadmConfigBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithJoinConfig adds the passed JoinConfig to the KubeadmConfigBuilder.
func (k *KubeadmConfigBuilder) WithJoinConfig(joinConf *bootstrapv1.JoinConfiguration) *KubeadmConfigBuilder {
	k.joinConfig = joinConf
	return k
}

// WithClusterConfig adds the passed ClusterConfig to the KubeadmConfigBuilder.
func (k *KubeadmConfigBuilder) WithClusterConfig(clusterConf *bootstrapv1.ClusterConfiguration) *KubeadmConfigBuilder {
	k.clusterConfig = clusterConf
	return k
}

// WithInitConfig adds the passed InitConfig to the KubeadmConfigBuilder.
func (k *KubeadmConfigBuilder) WithInitConfig(initConf *bootstrapv1.InitConfiguration) *KubeadmConfigBuilder {
	k.initConfig = initConf
	return k
}

// Unstructured produces a KubeadmConfig as an unstructured Kubernetes object.
func (k *KubeadmConfigBuilder) Unstructured() *unstructured.Unstructured {
	config := k.Build()
	rawMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: rawMap}
}

// Build produces a KubeadmConfig from the variable in the KubeadmConfigBuilder.
func (k *KubeadmConfigBuilder) Build() *bootstrapv1.KubeadmConfig {
	config := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: k.namespace,
			Name:      k.name,
		},
	}
	if k.initConfig != nil {
		config.Spec.InitConfiguration = k.initConfig
	}
	if k.joinConfig != nil {
		config.Spec.JoinConfiguration = k.joinConfig
	}
	if k.clusterConfig != nil {
		config.Spec.ClusterConfiguration = k.clusterConfig
	}
	return config
}

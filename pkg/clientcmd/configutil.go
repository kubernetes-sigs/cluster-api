/*
Copyright 2018 The Kubernetes Authors.

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

// Package clientcmd contains convenience methods for working with the kubeconfig and loading specific configurations
// of api.Config and rest.Config.
package clientcmd

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

// This is a convienience method to prevent the need of importing both this version of clientcmd and the client-go version
func NewConfigOverrides() clientcmd.ConfigOverrides {
	return clientcmd.ConfigOverrides{}
}

// NewCoreClientSetForDefaultSearchPath creates a core kubernetes clientset. If the kubeconfigPath is specified then the configuration is loaded from that path.
// Otherwise the default kubeconfig search path is used.
// The overrides parameter is used to select a specific context of the config, for example, select the context with a given cluster name or namespace.
func NewCoreClientSetForDefaultSearchPath(kubeconfigPath string, overrides clientcmd.ConfigOverrides) (*kubernetes.Clientset, error) {
	config, err := newRestConfigForDefaultSearchPath(kubeconfigPath, overrides)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// NewCoreClientSetForKubeconfig creates a core kubernetes clientset for the given kubeconfig string.
func NewCoreClientSetForKubeconfig(kubeconfig string) (*kubernetes.Clientset, error) {
	config, err := newRestConfigForKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// NewClusterApiClientForDefaultSearchPath creates a Cluster API clientset. If the kubeconfigPath is specified then the configuration is loaded from that path.
// Otherwise the default kubeconfig search path is used.
// The overrides parameter is used to select a specific context of the config, for example, select the context with a given cluster name or namespace.
func NewClusterApiClientForDefaultSearchPath(kubeconfigPath string, overrides clientcmd.ConfigOverrides) (*clientset.Clientset, error) {
	config, err := newRestConfigForDefaultSearchPath(kubeconfigPath, overrides)
	if err != nil {
		return nil, err
	}
	return clientset.NewForConfig(config)
}

// NewClusterApiClientForKubeconfig creates a Cluster API clientset for the given kubeconfig string.
func NewClusterApiClientForKubeconfig(kubeconfig string) (*clientset.Clientset, error) {
	config, err := newRestConfigForKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return clientset.NewForConfig(config)
}

// NewClientsForDefaultSearchpath creates both a core kubernetes clientset and a cluster-api clientset. If the kubeconfigPath
// is specified then the configuration is loaded from that path. Otherwise the default kubeconfig search path is used.
// The overrides parameter is used to select a specific context of the config, for example, select the context with a given cluster name or namespace.
func NewClientsForDefaultSearchpath(kubeconfigPath string, overrides clientcmd.ConfigOverrides) (*kubernetes.Clientset, *clientset.Clientset, error) {
	config, err := newRestConfigForDefaultSearchPath(kubeconfigPath, overrides)
	if err != nil {
		return nil, nil, err
	}
	return newClientsFromRestConfig(config)
}

// newClientsFromRestConfig creates both a core kubernetes clientset and a cluster-api clientset from a given rest.Config
func newClientsFromRestConfig(config *rest.Config) (*kubernetes.Clientset, *clientset.Clientset, error) {
	coreClients, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating core clients: %v", err)
	}
	clusterApiClient, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating cluster-api clients: %v", err)
	}
	return coreClients, clusterApiClient, nil
}

// newRestConfig creates a rest.Config for the given apiConfig
// The overrides parameter is used to select a specific context of the config, for example, select the context with a given cluster name or namespace.
func newRestConfig(apiConfig *api.Config, overrides clientcmd.ConfigOverrides) (*rest.Config, error) {
	return clientcmd.NewDefaultClientConfig(*apiConfig, &overrides).ClientConfig()
}

// newRestConfigForDefaultSearchPath creates a rest.Config by searching for the kubeconfig on the default search path. If an override 'kubeconfigPath' is
// given then that path is used instead of the default path. If no override is given, an attempt is made to load the
// 'in cluster' config. If this fails, then the default search path is used.
//
// The overrides parameter is used to select a specific context of the config, for example, select the context with a given cluster name or namespace.
func newRestConfigForDefaultSearchPath(kubeconfigPath string, overrides clientcmd.ConfigOverrides) (*rest.Config, error) {
	if kubeconfigPath == "" {
		config, err := rest.InClusterConfig()
		// if there is no err, continue because InClusterConfig is only expected to succeed if running inside of a pod.
		if err == nil {
			return config, nil
		}
	}
	apiConfig, err := newApiConfigForDefaultSearchPath(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	return newRestConfig(apiConfig, overrides)
}

// newRestConfigForKubeconfig creates a rest.Config for a given kubeconfig string.
func newRestConfigForKubeconfig(kubeconfig string) (*rest.Config, error) {
	apiConfig, err := newApiConfigForKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return newRestConfig(apiConfig, clientcmd.ConfigOverrides{})
}

// newApiConfigForDefaultSearchPath creates an api.Config by searching for the kubeconfig on the default search path. If an override 'kubeconfigPath' is
// given then that path is used instead of the default path.
func newApiConfigForDefaultSearchPath(kubeconfigPath string) (*api.Config, error) {
	configLoader := clientcmd.NewDefaultClientConfigLoadingRules()
	configLoader.ExplicitPath = kubeconfigPath
	return configLoader.Load()
}

// newApiConfigForKubeconfig creates an api.Config for a given kubeconfig string.
func newApiConfigForKubeconfig(kubeconfig string) (*api.Config, error) {
	return clientcmd.Load([]byte(kubeconfig))
}

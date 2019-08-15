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

package remote

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterClient is an interface encapsulating methods
// to access a remote cluster.
type ClusterClient interface {
	RESTConfig() *restclient.Config
	CoreV1() (corev1.CoreV1Interface, error)
}

// clusterClient is a helper struct to connect to remote workload clusters.
type clusterClient struct {
	restConfig *restclient.Config
	cluster    *v1alpha2.Cluster
}

// NewClusterClient creates a new ClusterClient.
func NewClusterClient(c client.Client, cluster *v1alpha2.Cluster) (ClusterClient, error) {
	secret, err := GetKubeConfigSecret(c, cluster.Name, cluster.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve kubeconfig secret for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}

	kubeconfig, err := KubeConfigFromSecret(secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig from secret for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client configuration for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}

	return &clusterClient{
		restConfig: restConfig,
		cluster:    cluster,
	}, nil
}

// RESTConfig returns a configuration instance to be used with a Kubernetes client.
func (c *clusterClient) RESTConfig() *restclient.Config {
	return c.restConfig
}

// CoreV1 returns a new Kubernetes CoreV1 client.
func (c *clusterClient) CoreV1() (corev1.CoreV1Interface, error) {
	return corev1.NewForConfig(c.RESTConfig())
}

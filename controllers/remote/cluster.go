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
	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewClusterClient returns a Client for interacting with a remote Cluster using the given scheme for encoding and decoding objects.
func NewClusterClient(c client.Client, cluster *clusterv1.Cluster, scheme *runtime.Scheme) (client.Client, error) {

	restConfig, err := RESTConfig(c, cluster)
	if err != nil {
		return nil, err
	}
	mapper, err := apiutil.NewDynamicRESTMapper(restConfig, apiutil.WithLazyDiscovery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create a DynamicRESTMapper for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	ret, err := client.New(restConfig, client.Options{Scheme: scheme, Mapper: mapper})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	return ret, nil
}

// RESTConfig returns a configuration instance to be used with a Kubernetes client.
func RESTConfig(c client.Client, cluster *clusterv1.Cluster) (*restclient.Config, error) {
	kubeConfig, err := kcfg.FromSecret(c, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve kubeconfig secret for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create REST configuration for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	return restConfig, nil
}

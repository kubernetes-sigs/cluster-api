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
	"context"
	"time"

	"github.com/pkg/errors"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultClientTimeout = 10 * time.Second
)

// ClusterClientGetter returns a new remote client.
type ClusterClientGetter func(ctx context.Context, sourceName string, c client.Client, cluster client.ObjectKey) (client.Client, error)

// NewClusterClient returns a Client for interacting with a remote Cluster using the given scheme for encoding and decoding objects.
func NewClusterClient(ctx context.Context, sourceName string, c client.Client, cluster client.ObjectKey) (client.Client, error) {
	restConfig, err := RESTConfig(ctx, sourceName, c, cluster)
	if err != nil {
		return nil, err
	}
	ret, err := client.New(restConfig, client.Options{Scheme: c.Scheme()})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	return ret, nil
}

// RESTConfig returns a configuration instance to be used with a Kubernetes client.
func RESTConfig(ctx context.Context, sourceName string, c client.Reader, cluster client.ObjectKey) (*restclient.Config, error) {
	kubeConfig, err := kcfg.FromSecret(ctx, c, cluster)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve kubeconfig secret for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create REST configuration for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	restConfig.UserAgent = DefaultClusterAPIUserAgent(sourceName)
	restConfig.Timeout = defaultClientTimeout

	return restConfig, nil
}

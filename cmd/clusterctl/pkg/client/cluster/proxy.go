/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"github.com/pkg/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Scheme = scheme.Scheme
)

type proxy struct {
	kubeconfig string
}

var _ Proxy = &proxy{}

func (k *proxy) CurrentNamespace() (string, error) {
	config, err := clientcmd.LoadFromFile(k.kubeconfig)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load Kubeconfig file from %q", k.kubeconfig)
	}

	if config.CurrentContext == "" {
		return "", errors.Wrapf(err, "failed to get current-context from %q", k.kubeconfig)
	}

	v, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return "", errors.Wrapf(err, "failed to get context %q from %q", config.CurrentContext, k.kubeconfig)
	}

	if v.Namespace != "" {
		return v.Namespace, nil
	}

	return "default", nil
}

func (k *proxy) NewClient() (client.Client, error) {
	config, err := k.getConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create controller-runtime client")
	}

	c, err := client.New(config, client.Options{Scheme: Scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create controller-runtime client")
	}

	return c, nil
}

func newK8SProxy(kubeconfig string) Proxy {
	// If a kubeconfig file is provided use it, otherwise find a config file in the standard locations
	if kubeconfig == "" {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	return &proxy{
		kubeconfig: kubeconfig,
	}
}

func (k *proxy) getConfig() (*rest.Config, error) {
	config, err := clientcmd.LoadFromFile(k.kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load Kubeconfig file from %q", k.kubeconfig)
	}

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to rest client")
	}

	return restConfig, nil
}

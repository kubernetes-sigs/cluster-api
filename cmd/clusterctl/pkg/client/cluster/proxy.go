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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
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

	mapper, err := apiutil.NewDynamicRESTMapper(config, apiutil.WithLazyDiscovery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create the controller-runtime DynamicRESTMapper")
	}

	c, err := client.New(config, client.Options{Scheme: Scheme, Mapper: mapper})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create controller-runtime client")
	}

	return c, nil
}

func (k *proxy) ListResources(namespace string, labels map[string]string) ([]unstructured.Unstructured, error) {
	cs, err := k.newClientSet()
	if err != nil {
		return nil, err
	}

	c, err := k.NewClient()
	if err != nil {
		return nil, err
	}

	// Get all the API resources in the cluster.
	resourceList, err := cs.Discovery().ServerPreferredResources()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list api resources")
	}

	// Select resources with list and delete methods (list is required by this method, delete by the callers of this method)
	resourceList = discovery.FilteredBy(discovery.SupportsAllVerbs{Verbs: []string{"list", "delete"}}, resourceList)

	var ret []unstructured.Unstructured
	for _, resourceGroup := range resourceList {
		for _, resourceKind := range resourceGroup.APIResources {
			// Discard the resourceKind that exists in two api groups (we are excluding one of the two groups arbitrarily).
			if resourceGroup.GroupVersion == "extensions/v1beta1" &&
				(resourceKind.Name == "daemonsets" || resourceKind.Name == "deployments" || resourceKind.Name == "replicasets" || resourceKind.Name == "networkpolicies" || resourceKind.Name == "ingresses") {
				continue
			}

			// List all the object instances of this resourceKind with the given labels
			selectors := []client.ListOption{
				client.MatchingLabels(labels),
			}

			if namespace != "" && resourceKind.Namespaced {
				selectors = append(selectors, client.InNamespace(namespace))
			}

			objList := new(unstructured.UnstructuredList)
			objList.SetAPIVersion(resourceGroup.GroupVersion)
			objList.SetKind(resourceKind.Kind)

			if err := c.List(ctx, objList, selectors...); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return nil, errors.Wrapf(err, "failed to list objects for the %q GroupVersionKind", objList.GroupVersionKind())
			}

			// Add obj to the result.
			ret = append(ret, objList.Items...)
		}
	}
	return ret, nil
}

func newProxy(kubeconfig string) Proxy {
	// If a kubeconfig file isn't provided, find one in the standard locations.
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

func (k *proxy) newClientSet() (*kubernetes.Clientset, error) {
	config, err := k.getConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the client-go client")
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the client-go client")
	}

	return cs, nil
}

/*
Copyright 2017 The Kubernetes Authors.

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

package client

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	clustersv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	scheme "k8s.io/client-go/kubernetes/scheme"
)

type ClusterAPIV1Alpha1Interface interface {
	RESTClient() rest.Interface
	MachinesGetter
	ClustersGetter
}

type ClusterAPIV1Alpha1Client struct {
	restClient rest.Interface
}

func (c *ClusterAPIV1Alpha1Client) Machines() MachinesInterface {
	return newMachines(c)
}

func (c *ClusterAPIV1Alpha1Client) Clusters() ClustersInterface {
	return newClusters(c)
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *ClusterAPIV1Alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

func NewForConfig(c *rest.Config) (*ClusterAPIV1Alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &ClusterAPIV1Alpha1Client{client}, nil
}

func NewForConfigOrDie(c *rest.Config) *ClusterAPIV1Alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

func New(c rest.Interface) *ClusterAPIV1Alpha1Client {
	return &ClusterAPIV1Alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	global_scheme := scheme.Scheme
	if err := machinesv1.AddToScheme(global_scheme); err != nil {
		return err
	}
	if err := clustersv1.AddToScheme(global_scheme); err != nil {
		return err
	}

	gv := machinesv1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(global_scheme)}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

package client

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
)

type ClusterAPIV1Alpha1Interface interface {
	RESTClient() rest.Interface
	MachinesGetter
}

type ClusterAPIV1Alpha1Client struct {
	restClient rest.Interface
}

func (c *ClusterAPIV1Alpha1Client) Machines() MachinesInterface {
	return newMachines(c)
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
	scheme := runtime.NewScheme()
	if err := machinesv1.AddToScheme(scheme); err != nil {
		return err
	}

	gv := machinesv1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

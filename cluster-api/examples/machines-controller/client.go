package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
)

func restClient() (*rest.RESTClient, *runtime.Scheme, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	scheme := runtime.NewScheme()
	if err := machinesv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	config := *cfg
	config.GroupVersion = &machinesv1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(scheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}

	return client, scheme, nil
}

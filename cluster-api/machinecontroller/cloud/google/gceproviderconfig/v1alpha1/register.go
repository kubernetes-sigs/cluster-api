package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kube-deploy/cluster-api/machinecontroller/cloud/google/gceproviderconfig"
)

const GroupName = "gceproviderconfig"

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

var (
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func init() {
	localSchemeBuilder.Register(addKnownTypes)
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&GCEProviderConfig{},
	)
	return nil
}

func NewSchemeAndCodecs() (*runtime.Scheme, *serializer.CodecFactory, error) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	if err := gceproviderconfig.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	codecs := serializer.NewCodecFactory(scheme)
	return scheme, &codecs, nil
}

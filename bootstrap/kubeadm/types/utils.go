/*
Copyright 2021 The Kubernetes Authors.

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

// Package utils contains Kubeadm utility types.
package utils

import (
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstream"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamhub"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta4"
	"sigs.k8s.io/cluster-api/util/version"
)

var (
	v1beta3KubeadmVersion = semver.MustParse("1.22.0")
	v1beta4KubeadmVersion = semver.MustParse("1.31.0")

	clusterConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta4.GroupVersion: &upstreamv1beta4.ClusterConfiguration{},
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.ClusterConfiguration{},
	}

	initConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta4.GroupVersion: &upstreamv1beta4.InitConfiguration{},
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.InitConfiguration{},
	}

	joinConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta4.GroupVersion: &upstreamv1beta4.JoinConfiguration{},
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.JoinConfiguration{},
	}
)

// KubeVersionToKubeadmAPIGroupVersion maps a Kubernetes version to the correct Kubeadm API Group supported.
func KubeVersionToKubeadmAPIGroupVersion(v semver.Version) (schema.GroupVersion, error) {
	switch {
	case version.Compare(v, v1beta3KubeadmVersion, version.WithoutPreReleases()) < 0:
		return schema.GroupVersion{}, errors.New("the bootstrap provider for kubeadm doesn't support Kubernetes version lower than v1.22.0")
	case version.Compare(v, v1beta4KubeadmVersion, version.WithoutPreReleases()) < 0:
		// NOTE: All the Kubernetes version >= v1.22 and < v1.31 should use the kubeadm API version v1beta3
		return upstreamv1beta3.GroupVersion, nil
	default:
		// NOTE: All the Kubernetes version greater or equal to v1.31
		// are assumed to use the kubeadm API version v1beta4 or to work with Cluster API using it.
		// This should be reconsidered whenever kubeadm introduces a new API version.
		return upstreamv1beta4.GroupVersion, nil
	}
}

// MarshalClusterConfigurationForVersion converts a Cluster API ClusterConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalClusterConfigurationForVersion(clusterConfiguration *bootstrapv1.ClusterConfiguration, version semver.Version, data *upstream.AdditionalData) (string, error) {
	hub := &upstreamhub.ClusterConfiguration{}
	if clusterConfiguration != nil {
		hub.ClusterConfiguration = *clusterConfiguration
	}
	return marshalForVersion(hub, version, clusterConfigurationVersionTypeMap, data)
}

// MarshalInitConfigurationForVersion converts a Cluster API InitConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalInitConfigurationForVersion(initConfiguration *bootstrapv1.InitConfiguration, version semver.Version) (string, error) {
	hub := &upstreamhub.InitConfiguration{}
	if initConfiguration != nil {
		hub.InitConfiguration = *initConfiguration
	}
	return marshalForVersion(hub, version, initConfigurationVersionTypeMap, nil)
}

// MarshalJoinConfigurationForVersion converts a Cluster API JoinConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalJoinConfigurationForVersion(joinConfiguration *bootstrapv1.JoinConfiguration, version semver.Version) (string, error) {
	hub := &upstreamhub.JoinConfiguration{}
	if joinConfiguration != nil {
		hub.JoinConfiguration = *joinConfiguration
	}
	return marshalForVersion(hub, version, joinConfigurationVersionTypeMap, nil)
}

func marshalForVersion(obj conversion.Hub, version semver.Version, kubeadmObjVersionTypeMap map[schema.GroupVersion]conversion.Convertible, data *upstream.AdditionalData) (string, error) {
	kubeadmAPIGroupVersion, err := KubeVersionToKubeadmAPIGroupVersion(version)
	if err != nil {
		return "", err
	}

	targetKubeadmObj, ok := kubeadmObjVersionTypeMap[kubeadmAPIGroupVersion]
	if !ok {
		return "", errors.Errorf("missing KubeadmAPI type mapping for version %s", kubeadmAPIGroupVersion)
	}

	targetKubeadmObj = targetKubeadmObj.DeepCopyObject().(conversion.Convertible)
	if err := targetKubeadmObj.ConvertFrom(obj); err != nil {
		return "", errors.Wrapf(err, "failed to convert to KubeadmAPI type for version %s", kubeadmAPIGroupVersion)
	}

	if additionalDataSetter, ok := targetKubeadmObj.(upstream.AdditionalDataSetter); ok && data != nil {
		additionalDataSetter.SetAdditionalData(*data)
	}

	codecs, err := getCodecsFor(kubeadmAPIGroupVersion, targetKubeadmObj)
	if err != nil {
		return "", err
	}

	yaml, err := toYaml(targetKubeadmObj, kubeadmAPIGroupVersion, codecs)
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate yaml for the Kubeadm API for version %s", kubeadmAPIGroupVersion)
	}
	return string(yaml), nil
}

func getCodecsFor(gv schema.GroupVersion, obj runtime.Object) (serializer.CodecFactory, error) {
	sb := &scheme.Builder{GroupVersion: gv}
	sb.Register(obj)
	kubeadmScheme, err := sb.Build()
	if err != nil {
		return serializer.CodecFactory{}, errors.Wrapf(err, "failed to build scheme for kubeadm types conversions")
	}
	return serializer.NewCodecFactory(kubeadmScheme), nil
}

func toYaml(obj runtime.Object, gv runtime.GroupVersioner, codecs serializer.CodecFactory) ([]byte, error) {
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), runtime.ContentTypeYAML)
	if !ok {
		return []byte{}, errors.Errorf("unsupported media type %q", runtime.ContentTypeYAML)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, gv)
	return runtime.Encode(encoder, obj)
}

// UnmarshalClusterConfiguration tries to translate a Kubeadm API yaml back to the Cluster API ClusterConfiguration type.
// NOTE: The yaml could be any of the known formats for the kubeadm ClusterConfiguration type.
func UnmarshalClusterConfiguration(yaml string) (*bootstrapv1.ClusterConfiguration, *upstream.AdditionalData, error) {
	obj := &upstreamhub.ClusterConfiguration{}
	data := &upstream.AdditionalData{}
	if err := unmarshalFromVersions(yaml, clusterConfigurationVersionTypeMap, obj, data); err != nil {
		return nil, nil, err
	}
	return &obj.ClusterConfiguration, data, nil
}

// UnmarshalInitConfiguration tries to translate a Kubeadm API yaml back to the InitConfiguration type.
// NOTE: The yaml could be any of the known formats for the kubeadm InitConfiguration type.
func UnmarshalInitConfiguration(yaml string) (*bootstrapv1.InitConfiguration, error) {
	obj := &upstreamhub.InitConfiguration{}
	if err := unmarshalFromVersions(yaml, initConfigurationVersionTypeMap, obj, nil); err != nil {
		return nil, err
	}
	return &obj.InitConfiguration, nil
}

// UnmarshalJoinConfiguration tries to translate a Kubeadm API yaml back to the JoinConfiguration type.
// NOTE: The yaml could be any of the known formats for the kubeadm JoinConfiguration type.
func UnmarshalJoinConfiguration(yaml string) (*bootstrapv1.JoinConfiguration, error) {
	obj := &upstreamhub.JoinConfiguration{}
	if err := unmarshalFromVersions(yaml, joinConfigurationVersionTypeMap, obj, nil); err != nil {
		return nil, err
	}
	return &obj.JoinConfiguration, nil
}

func unmarshalFromVersions(yaml string, kubeadmAPIVersions map[schema.GroupVersion]conversion.Convertible, capiObj conversion.Hub, data *upstream.AdditionalData) error {
	// For each know kubeadm API version
	for gv, obj := range kubeadmAPIVersions {
		// Tries conversion from yaml to the corresponding kubeadmObj
		kubeadmObj := obj.DeepCopyObject()
		gvk := kubeadmObj.GetObjectKind().GroupVersionKind()
		codecs, err := getCodecsFor(gv, kubeadmObj)
		if err != nil {
			return errors.Wrapf(err, "failed to build scheme for kubeadm types conversions")
		}

		_, _, err = codecs.UniversalDeserializer().Decode([]byte(yaml), &gvk, kubeadmObj)
		if err == nil {
			// If conversion worked, then converts the kubeadmObj (spoke) back to the Cluster API type (hub).
			if err := kubeadmObj.(conversion.Convertible).ConvertTo(capiObj); err != nil {
				return errors.Wrapf(err, "failed to convert kubeadm types to Cluster API types")
			}

			if additionalDataGetter, ok := kubeadmObj.(upstream.AdditionalDataGetter); ok && data != nil {
				additionalDataGetter.GetAdditionalData(data)
			}
			return nil
		}
	}
	return errors.New("unknown kubeadm types")
}

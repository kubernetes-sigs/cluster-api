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
	"github.com/blang/semver"
	"github.com/pkg/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/upstreamv1beta3"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	v1beta1KubeadmVersion = semver.MustParse("1.13.0")
	v1beta2KubeadmVersion = semver.MustParse("1.15.0")
	v1beta3KubeadmVersion = semver.MustParse("1.22.0")

	clusterConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.ClusterConfiguration{},
		upstreamv1beta2.GroupVersion: &upstreamv1beta2.ClusterConfiguration{},
		upstreamv1beta1.GroupVersion: &upstreamv1beta1.ClusterConfiguration{},
	}

	clusterStatusVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		// ClusterStatus has been removed in v1beta3, so we don't need an entry for v1beta3
		upstreamv1beta2.GroupVersion: &upstreamv1beta2.ClusterStatus{},
		upstreamv1beta1.GroupVersion: &upstreamv1beta1.ClusterStatus{},
	}

	initConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.InitConfiguration{},
		upstreamv1beta2.GroupVersion: &upstreamv1beta2.InitConfiguration{},
		upstreamv1beta1.GroupVersion: &upstreamv1beta1.InitConfiguration{},
	}

	joinConfigurationVersionTypeMap = map[schema.GroupVersion]conversion.Convertible{
		upstreamv1beta3.GroupVersion: &upstreamv1beta3.JoinConfiguration{},
		upstreamv1beta2.GroupVersion: &upstreamv1beta2.JoinConfiguration{},
		upstreamv1beta1.GroupVersion: &upstreamv1beta1.JoinConfiguration{},
	}
)

// KubeVersionToKubeadmAPIGroupVersion maps a Kubernetes version to the correct Kubeadm API Group supported.
func KubeVersionToKubeadmAPIGroupVersion(version semver.Version) (schema.GroupVersion, error) {
	switch {
	case version.LT(v1beta1KubeadmVersion):
		return schema.GroupVersion{}, errors.New("the bootstrap provider for kubeadm doesn't support Kubernetes version lower than v1.13.0")
	case version.LT(v1beta2KubeadmVersion):
		// NOTE: All the Kubernetes version >= v1.13 and < v1.15 should use the kubeadm API version v1beta1
		return upstreamv1beta1.GroupVersion, nil
	case version.LT(v1beta3KubeadmVersion):
		// NOTE: All the Kubernetes version >= v1.15 and < v1.22 should use the kubeadm API version v1beta2
		return upstreamv1beta2.GroupVersion, nil
	default:
		// NOTE: All the Kubernetes version greater or equal to v1.22 should use the kubeadm API version v1beta3.
		// Also future Kubernetes versions (not yet released at the time of writing this code) are going to use v1beta3,
		// no matter if kubeadm API versions newer than v1beta3 could be introduced by those release.
		// This is acceptable because v1beta3 will be supported by kubeadm until the deprecation cycle completes
		// (9 months minimum after the deprecation date, not yet announced now); this gives Cluster API project time to
		// introduce support for newer releases without blocking users to deploy newer version of Kubernetes.
		return upstreamv1beta3.GroupVersion, nil
	}
}

// MarshalClusterConfigurationForVersion converts a Cluster API ClusterConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalClusterConfigurationForVersion(obj *bootstrapv1.ClusterConfiguration, version semver.Version) (string, error) {
	return marshalForVersion(obj, version, clusterConfigurationVersionTypeMap)
}

// MarshalClusterStatusForVersion converts a Cluster API ClusterStatus type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalClusterStatusForVersion(obj *bootstrapv1.ClusterStatus, version semver.Version) (string, error) {
	return marshalForVersion(obj, version, clusterStatusVersionTypeMap)
}

// MarshalInitConfigurationForVersion converts a Cluster API InitConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalInitConfigurationForVersion(obj *bootstrapv1.InitConfiguration, version semver.Version) (string, error) {
	return marshalForVersion(obj, version, initConfigurationVersionTypeMap)
}

// MarshalJoinConfigurationForVersion converts a Cluster API JoinConfiguration type to the kubeadm API type
// for the given Kubernetes Version.
// NOTE: This assumes Kubernetes Version equals to kubeadm version.
func MarshalJoinConfigurationForVersion(obj *bootstrapv1.JoinConfiguration, version semver.Version) (string, error) {
	return marshalForVersion(obj, version, joinConfigurationVersionTypeMap)
}

func marshalForVersion(obj conversion.Hub, version semver.Version, kubeadmObjVersionTypeMap map[schema.GroupVersion]conversion.Convertible) (string, error) {
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
func UnmarshalClusterConfiguration(yaml string) (*bootstrapv1.ClusterConfiguration, error) {
	obj := &bootstrapv1.ClusterConfiguration{}
	if err := unmarshalFromVersions(yaml, clusterConfigurationVersionTypeMap, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// UnmarshalClusterStatus tries to translate a Kubeadm API yaml back to the Cluster API ClusterStatus type.
// NOTE: The yaml could be any of the known formats for the kubeadm ClusterStatus type.
func UnmarshalClusterStatus(yaml string) (*bootstrapv1.ClusterStatus, error) {
	obj := &bootstrapv1.ClusterStatus{}
	if err := unmarshalFromVersions(yaml, clusterStatusVersionTypeMap, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func unmarshalFromVersions(yaml string, kubeadmAPIVersions map[schema.GroupVersion]conversion.Convertible, capiObj conversion.Hub) error {
	// For each know kubeadm API version
	for gv, obj := range kubeadmAPIVersions {
		// Tries conversion from yaml to the corresponding kubeadmObj
		kubeadmObj := obj.DeepCopyObject()
		gvk := kubeadmObj.GetObjectKind().GroupVersionKind()
		codecs, err := getCodecsFor(gv, kubeadmObj)
		if err != nil {
			return errors.Wrapf(err, "failed to build scheme for kubeadm types conversions")
		}

		if _, _, err := codecs.UniversalDeserializer().Decode([]byte(yaml), &gvk, kubeadmObj); err == nil {
			// If conversion worked, then converts the kubeadmObj (spoke) back to the Cluster API ClusterConfiguration type (hub).
			if err := kubeadmObj.(conversion.Convertible).ConvertTo(capiObj); err != nil {
				return errors.Wrapf(err, "failed to convert kubeadm types to Cluster API types")
			}
			return nil
		}
	}
	return errors.New("unknown kubeadm types")
}

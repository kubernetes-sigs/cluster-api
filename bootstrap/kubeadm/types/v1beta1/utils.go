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

package v1beta1

import (
	"strings"

	"github.com/pkg/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	versionutil "k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

func KubeVersionToKubeadmAPIGroupVersion(version string) (schema.GroupVersion, error) {
	if version == "" {
		return schema.GroupVersion{}, errors.New("version cannot be empty")
	}
	semVersion, err := versionutil.ParseSemantic(version)
	if err != nil {
		return schema.GroupVersion{}, errors.Wrap(err, "error parsing the Kubernetes version")
	}
	switch {
	case semVersion.LessThan(versionutil.MustParseSemantic("v1.13.0")):
		return schema.GroupVersion{}, errors.New("the bootstrap provider for kubeadm doesn't support Kubernetes version lower than v1.13.0")
	case semVersion.LessThan(versionutil.MustParseSemantic("v1.15.0")):
		// NOTE: All the Kubernetes version >= v1.13 and < v1.15 should use the kubeadm API version v1beta1
		return GroupVersion, nil
	default:
		// NOTE: All the Kubernetes version greater or equal to v1.15  should use the kubeadm API version v1beta2.
		// Also future Kubernetes versions (not yet released at the time of writing this code) are going to use v1beta2,
		// no matter if kubeadm API versions newer than v1beta2 could be introduced by those release.
		// This is acceptable because but v1beta2 will be supported by kubeadm until the deprecation cycle completes
		// (9 months minimum after the deprecation date, not yet announced now); this gives Cluster API project time to
		// introduce support for newer releases without blocking users to deploy newer version of Kubernetes.
		return v1beta2.GroupVersion, nil
	}
}

// ConfigurationToYAMLForVersion converts a kubeadm configuration type to its YAML
// representation.
func ConfigurationToYAMLForVersion(obj runtime.Object, k8sVersion string) (string, error) {
	yamlBytes, err := MarshalToYamlForCodecs(obj, GroupVersion, GetCodecs())
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal configuration")
	}

	yaml := string(yamlBytes)

	// Fix the YAML according to the target Kubernetes version
	// IMPORTANT: This is a stop-gap explicitly designed for back-porting on the v1alpha3 branch.
	// This allows to unblock removal of the v1beta1 API in kubeadm by making Cluster API to use the v1beta2 kubeadm API
	// under the assumption that the serialized version of the two APIs is equal as discussed; see
	// "Insulate users from kubeadm API version changes" CAEP for more details.
	// NOTE: This solution will stop to work when kubeadm will drop then v1beta2 kubeadm API, but this gives
	// enough time (9/12 months from the deprecation date, not yet announced) for the users to migrate to
	// the v1alpha4 release of Cluster API, where a proper conversion mechanism is going to be supported.
	gv, err := KubeVersionToKubeadmAPIGroupVersion(k8sVersion)
	if err != nil {
		return "", err
	}

	if gv != GroupVersion {
		yaml = strings.Replace(yaml, GroupVersion.String(), gv.String(), -1)
	}

	return yaml, nil
}

// GetCodecs returns a type that can be used to deserialize most kubeadm
// configuration types.
func GetCodecs() serializer.CodecFactory {
	sb := &scheme.Builder{GroupVersion: GroupVersion}

	sb.Register(&JoinConfiguration{}, &InitConfiguration{}, &ClusterConfiguration{})
	kubeadmScheme, err := sb.Build()
	if err != nil {
		panic(err)
	}
	return serializer.NewCodecFactory(kubeadmScheme)
}

// ConfigurationToYAML converts a kubeadm configuration type to its YAML
// representation.
func ConfigurationToYAML(obj runtime.Object) (string, error) {
	initcfg, err := MarshalToYamlForCodecs(obj, GroupVersion, GetCodecs())
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal configuration")
	}
	return string(initcfg), nil
}

// MarshalToYamlForCodecs marshals an object into yaml using the specified codec
// TODO: Is specifying the gv really needed here?
// TODO: Can we support json out of the box easily here?
func MarshalToYamlForCodecs(obj runtime.Object, gv runtime.GroupVersioner, codecs serializer.CodecFactory) ([]byte, error) {
	mediaType := "application/yaml"
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return []byte{}, errors.Errorf("unsupported media type %q", mediaType)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, gv)
	return runtime.Encode(encoder, obj)
}

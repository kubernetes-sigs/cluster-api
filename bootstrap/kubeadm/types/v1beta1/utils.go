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
	"github.com/pkg/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

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

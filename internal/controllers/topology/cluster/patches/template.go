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

package patches

import (
	"encoding/json"
	"strconv"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
)

// unstructuredDecoder is used to decode byte arrays into Unstructured objects.
var unstructuredDecoder = serializer.NewCodecFactory(nil).UniversalDeserializer()

// requestItemBuilder builds a GeneratePatchesRequestItem.
type requestItemBuilder struct {
	template *unstructured.Unstructured
	holder   runtimehooksv1.HolderReference
}

// newRequestItemBuilder returns a new requestItemBuilder.
func newRequestItemBuilder(template *unstructured.Unstructured) *requestItemBuilder {
	return &requestItemBuilder{
		template: template,
	}
}

// WithHolder adds holder to the requestItemBuilder.
func (t *requestItemBuilder) WithHolder(object client.Object, fieldPath string) *requestItemBuilder {
	t.holder = runtimehooksv1.HolderReference{
		APIVersion: object.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       object.GetObjectKind().GroupVersionKind().Kind,
		Namespace:  object.GetNamespace(),
		Name:       object.GetName(),
		FieldPath:  fieldPath,
	}
	return t
}

// uuidGenerator is defined as a package variable to enable changing it during testing.
var uuidGenerator func() types.UID = uuid.NewUUID

// Build builds a GeneratePatchesRequestItem.
func (t *requestItemBuilder) Build() (*runtimehooksv1.GeneratePatchesRequestItem, error) {
	tpl := &runtimehooksv1.GeneratePatchesRequestItem{
		HolderReference: t.holder,
		UID:             uuidGenerator(),
	}

	jsonObj, err := json.Marshal(t.template)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal template to JSON")
	}

	tpl.Object = runtime.RawExtension{
		Raw:    jsonObj,
		Object: t.template,
	}

	return tpl, nil
}

// getTemplateAsUnstructured is a utility func that returns a template matching the holderKind, holderFieldPath
// and mdTopologyName from a GeneratePatchesRequest.
func getTemplateAsUnstructured(req *runtimehooksv1.GeneratePatchesRequest, holderKind, holderFieldPath, mdTopologyName string) (*unstructured.Unstructured, error) {
	// Find the requestItem.
	requestItem := getRequestItem(req, holderKind, holderFieldPath, mdTopologyName)

	if requestItem == nil {
		return nil, errors.Errorf("failed to get request item with holder kind %q, holder field path %q and MD topology name %q", holderKind, holderFieldPath, mdTopologyName)
	}

	// Unmarshal the template.
	template, err := bytesToUnstructured(requestItem.Object.Raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert template to Unstructured")
	}

	return template, nil
}

// getRequestItemByUID is a utility func that returns a template matching the uid from a GeneratePatchesRequest.
func getRequestItemByUID(req *runtimehooksv1.GeneratePatchesRequest, uid types.UID) *runtimehooksv1.GeneratePatchesRequestItem {
	for i := range req.Items {
		if req.Items[i].UID == uid {
			return &req.Items[i]
		}
	}
	return nil
}

// getRequestItem is a utility func that returns a template matching the holderKind, holderFiledPath and mdTopologyName from a GeneratePatchesRequest.
func getRequestItem(req *runtimehooksv1.GeneratePatchesRequest, holderKind, holderFieldPath, mdTopologyName string) *runtimehooksv1.GeneratePatchesRequestItem {
	for _, template := range req.Items {
		if holderKind != "" && template.HolderReference.Kind != holderKind {
			continue
		}
		if holderFieldPath != "" && template.HolderReference.FieldPath != holderFieldPath {
			continue
		}
		if mdTopologyName != "" {
			templateVariables := toMap(template.Variables)

			v, err := variables.GetVariableValue(templateVariables, "builtin.machineDeployment.topologyName")
			if err != nil || string(v.Raw) != strconv.Quote(mdTopologyName) {
				continue
			}
		}

		return &template
	}
	return nil
}

// bytesToUnstructured provides a utility method that converts a (JSON) byte array into an Unstructured object.
func bytesToUnstructured(b []byte) (*unstructured.Unstructured, error) {
	// Unmarshal the JSON.
	u := &unstructured.Unstructured{}
	if _, _, err := unstructuredDecoder.Decode(b, nil, u); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal object from json")
	}

	return u, nil
}

// toMap converts a list of Variables to a map of JSON (name is the map key).
func toMap(variables []runtimehooksv1.Variable) map[string]apiextensionsv1.JSON {
	variablesMap := map[string]apiextensionsv1.JSON{}

	for i := range variables {
		variablesMap[variables[i].Name] = variables[i].Value
	}
	return variablesMap
}

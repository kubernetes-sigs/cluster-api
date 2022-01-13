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

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/api"
)

// templateBuilder builds templates.
type templateBuilder struct {
	template     *unstructured.Unstructured
	templateType api.TemplateType
	mdTopology   *clusterv1.MachineDeploymentTopology
}

// newTemplateBuilder returns a new templateBuilder.
func newTemplateBuilder(template *unstructured.Unstructured) *templateBuilder {
	return &templateBuilder{
		template: template,
	}
}

// WithType adds templateType to the templateBuilder.
func (t *templateBuilder) WithType(templateType api.TemplateType) *templateBuilder {
	t.templateType = templateType
	return t
}

// WithMachineDeploymentRef adds a MachineDeploymentTopology to the templateBuilder,
// which is used to add a MachineDeploymentRef to the TemplateRef during BuildTemplateRef.
func (t *templateBuilder) WithMachineDeploymentRef(mdTopology *clusterv1.MachineDeploymentTopology) *templateBuilder {
	t.mdTopology = mdTopology
	return t
}

// BuildTemplateRef builds an api.TemplateRef.
func (t *templateBuilder) BuildTemplateRef() api.TemplateRef {
	templateRef := api.TemplateRef{
		APIVersion:   t.template.GetAPIVersion(),
		Kind:         t.template.GetKind(),
		TemplateType: t.templateType,
	}

	if t.mdTopology != nil {
		templateRef.MachineDeploymentRef = api.MachineDeploymentRef{
			TopologyName: t.mdTopology.Name,
			Class:        t.mdTopology.Class,
		}
	}

	return templateRef
}

// Build builds an api.GenerateRequestTemplate.
func (t *templateBuilder) Build() (*api.GenerateRequestTemplate, error) {
	tpl := &api.GenerateRequestTemplate{
		TemplateRef: t.BuildTemplateRef(),
	}

	jsonObj, err := json.Marshal(t.template)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal object to json")
	}
	tpl.Template = apiextensionsv1.JSON{Raw: jsonObj}

	return tpl, nil
}

// getTemplateAsUnstructured is a utility func that returns a template matching the templateRef from a GenerateRequest.
func getTemplateAsUnstructured(req *api.GenerateRequest, templateRef api.TemplateRef) (*unstructured.Unstructured, error) {
	// Find the template the patch should be applied to.
	template := getTemplate(req, templateRef)

	// If a patch doesn't apply to any object, this is a misconfiguration.
	if template == nil {
		return nil, errors.Errorf("failed to get template %s", templateRef)
	}

	return templateToUnstructured(template)
}

// getTemplate is a utility func that returns a template matching the templateRef from a GenerateRequest.
func getTemplate(req *api.GenerateRequest, templateRef api.TemplateRef) *api.GenerateRequestTemplate {
	for _, template := range req.Items {
		if templateRefsAreEqual(templateRef, template.TemplateRef) {
			return template
		}
	}
	return nil
}

// templateRefsAreEqual returns true if the TemplateRefs are equal.
func templateRefsAreEqual(a, b api.TemplateRef) bool {
	return a.APIVersion == b.APIVersion &&
		a.Kind == b.Kind &&
		a.TemplateType == b.TemplateType &&
		a.MachineDeploymentRef.TopologyName == b.MachineDeploymentRef.TopologyName &&
		a.MachineDeploymentRef.Class == b.MachineDeploymentRef.Class
}

// templateToUnstructured converts a GenerateRequestTemplate into an Unstructured object.
func templateToUnstructured(t *api.GenerateRequestTemplate) (*unstructured.Unstructured, error) {
	// Unmarshal the template.
	u, err := bytesToUnstructured(t.Template.Raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert template to Unstructured")
	}

	// Set the GVK.
	gv, err := schema.ParseGroupVersion(t.TemplateRef.APIVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert template to Unstructured: failed to parse group version")
	}
	u.SetGroupVersionKind(gv.WithKind(t.TemplateRef.Kind))
	return u, nil
}

// bytesToUnstructured provides a utility method that converts a (JSON) byte array into an Unstructured object.
func bytesToUnstructured(b []byte) (*unstructured.Unstructured, error) {
	// Unmarshal the JSON.
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal object from json")
	}

	// Set the content.
	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(m)

	return u, nil
}

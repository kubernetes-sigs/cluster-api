/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"bytes"
	"fmt"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/yaml"
)

func lookup(m map[string]string) func(s string) string {
	return func(s string) string {
		return m[s]
	}
}

func makeFuncMap(cl *clusterv1.Cluster) template.FuncMap {
	return template.FuncMap{
		"annotation": lookup(cl.ObjectMeta.GetAnnotations()),
		"label":      lookup(cl.ObjectMeta.GetLabels()),
	}
}

func renderTemplates(cl *clusterv1.Cluster, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	raw, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to parse object as YAML: %w", err)
	}

	tmpl, err := template.New("job").Funcs(makeFuncMap(cl)).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	buf := bytes.Buffer{}
	err = tmpl.Execute(&buf, cl)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	var updated unstructured.Unstructured
	if err := yaml.Unmarshal(buf.Bytes(), &updated); err != nil {
		return nil, fmt.Errorf("failed to marshal obj: %w", err)
	}
	return &updated, nil
}

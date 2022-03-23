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

package openapi

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
	validation "k8s.io/kube-openapi/pkg/validation/spec"

	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/server"
)

type Spec interface {
	OpenAPI() (*spec3.OpenAPI, error)
}

type spec struct {
	catalog *catalog.Catalog
}

var _ Spec = &spec{}

func (s *spec) OpenAPI() (*spec3.OpenAPI, error) {
	// TODO: Refactor
	// TODO: Validate output

	o := &spec3.OpenAPI{ // TODO: this is missing tags :-(
		Version: "3.0.0",
		Info: &validation.Info{
			InfoProps: validation.InfoProps{
				Description: "Open API spec for Cluster API Runtime Extensions",
				Title:       "Cluster API Runtime Extensions",
				License: &validation.License{
					Name: "Apache 2.0",
					URL:  "http://www.apache.org/licenses/LICENSE-2.0.html",
				},
				Version: "v1.0.1", // TODO: CAPI version
			},
		},
		Paths: &spec3.Paths{
			Paths: map[string]*spec3.Path{},
		},
		Components: &spec3.Components{
			Schemas: map[string]*validation.Schema{},
		},
	}

	for _, gvs := range s.catalog.AllKnownHooks() {
		path := server.GVSToPath(gvs)
		pathItem := &spec3.Path{
			PathProps: spec3.PathProps{
				Parameters: make([]*spec3.Parameter, 0),
			},
		}

		op := &spec3.Operation{
			OperationProps: spec3.OperationProps{
				Tags:        []string{strings.Split(gvs.Group, ".")[0]}, // TODO: improve this; spec3.OpenAPI is missing specs...
				Summary:     "summary",                                  // TODO: add to catalog/RegisterHook.
				Description: "description",                              // TODO: add to catalog/RegisterHook.
				OperationId: "",                                         // TODO: generate from gvs
				Parameters:  nil,
				Responses: &spec3.Responses{
					ResponsesProps: spec3.ResponsesProps{
						StatusCodeResponses: make(map[int]*spec3.Response),
					},
				},
				Deprecated: false, // TODO: add to catalog/RegisterHook.
			},
		}

		inputGvk, err := s.catalog.RequestKind(gvs)
		if err != nil {
			panic("implement me!") // TODO: handle error
		}

		op.RequestBody = &spec3.RequestBody{
			RequestBodyProps: spec3.RequestBodyProps{
				// TODO: this seems repeated (same thing in response)
				Content: map[string]*spec3.MediaType{
					"application/json": {
						MediaTypeProps: spec3.MediaTypeProps{
							Schema: &validation.Schema{
								SchemaProps: validation.SchemaProps{
									Ref: componentRef(inputGvk),
								},
							},
						},
					},
				},
			},
		}

		outputGvk, err := s.catalog.ResponseKind(gvs)
		if err != nil {
			panic("implement me!") // TODO: handle error
		}

		op.Responses.StatusCodeResponses[http.StatusOK] = &spec3.Response{
			ResponseProps: spec3.ResponseProps{
				Description: "OK",
				// TODO: this seems repeated (same thing in requestBody)
				Content: map[string]*spec3.MediaType{
					"application/json": {
						MediaTypeProps: spec3.MediaTypeProps{
							Schema: &validation.Schema{
								SchemaProps: validation.SchemaProps{
									Ref: componentRef(outputGvk),
								},
							},
						},
					},
				},
			},
		}

		// TODO: other response codes?

		pathItem.Post = op

		o.Paths.Paths[path] = pathItem
	}

	types := s.catalog.AllKnownParameters()
	for gvk := range types {
		s.buildComponentsRecursively(gvk, types, o.Components)
	}

	return o, nil
}

func (s *spec) buildComponentsRecursively(gvk schema.GroupVersionKind, types map[schema.GroupVersionKind]reflect.Type, components *spec3.Components) error {
	name := componentName(gvk)
	if _, ok := components.Schemas[name]; ok {
		return nil
	}

	t, ok := types[gvk]
	if !ok {
		panic("implement me!") // TODO: handle error
	}

	getter, err := s.catalog.GetOpenAPIDefinitionsGetter(gvk.GroupVersion())
	if err != nil {
		return err
	}

	getterWithRef := getter(func(name string) validation.Ref {
		return validation.MustCreateRef("#/components/schemas/" + name)
	})

	pkgPath := t.PkgPath()

	getterName := fmt.Sprintf("%s.%s", pkgPath, gvk.Kind)

	if item, ok := getterWithRef[getterName]; ok {
		schema := &validation.Schema{
			VendorExtensible:   item.Schema.VendorExtensible,
			SchemaProps:        item.Schema.SchemaProps,
			SwaggerSchemaProps: item.Schema.SwaggerSchemaProps,
		}

		components.Schemas[name] = schema

		// TODO: investigate recursion (nested schema)
		/*
			for _, v := range item.Dependencies {
				if err := s.buildComponentsRecursively(gvk, components); err != nil {
					return err
				}
			}
		*/
	} else {
		return fmt.Errorf("cannot find model definition for %v. If you added a new type, you may need to add +k8s:openapi-gen=true to the package or type and run code-gen again", name)
	}
	return nil
}

func componentRef(gvk schema.GroupVersionKind) validation.Ref {
	return validation.MustCreateRef(fmt.Sprintf("#/components/schemas/%s", componentName(gvk)))
}

func componentName(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s.%s.%s", gvk.Kind, gvk.Version, gvk.Group)
}

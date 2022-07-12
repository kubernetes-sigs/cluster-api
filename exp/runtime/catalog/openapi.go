/*
Copyright 2022 The Kubernetes Authors.

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

package catalog

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// OpenAPI generates and returns the OpenAPI spec.
func (c *Catalog) OpenAPI(version string) (*spec3.OpenAPI, error) {
	openAPI := &spec3.OpenAPI{
		Version: "3.0.0",
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				Description: "This document defines the Open API specification of the services that Cluster API runtime is going " +
					"to call while managing the Cluster's lifecycle.\n" +
					"\n" +
					"Services described in this specification are also referred to as Runtime Hooks, given that they allow " +
					"external components to hook-in the cluster's lifecycle. The corresponding components implementing handlers " +
					"for Runtime Hooks calls are referred to as Runtime Extensions.\n" +
					"\n" +
					"More information is available in the [Cluster API book](https://cluster-api.sigs.k8s.io/).",
				Title: "Cluster API - Runtime SDK",
				License: &spec.License{
					Name: "Apache 2.0",
					URL:  "http://www.apache.org/licenses/LICENSE-2.0.html",
				},
				Version: version,
			},
		},
		Paths: &spec3.Paths{
			Paths: map[string]*spec3.Path{},
		},
		Components: &spec3.Components{
			Schemas: map[string]*spec.Schema{},
		},
	}

	for gvh, hookDescriptor := range c.gvhToHookDescriptor {
		err := addHookAndTypesToOpenAPI(openAPI, c, gvh, hookDescriptor)
		if err != nil {
			return nil, err
		}
	}

	return openAPI, nil
}

func addHookAndTypesToOpenAPI(openAPI *spec3.OpenAPI, c *Catalog, gvh GroupVersionHook, hookDescriptor hookDescriptor) error {
	// Create the operation.
	operation := &spec3.Operation{
		OperationProps: spec3.OperationProps{
			Tags:        hookDescriptor.metadata.Tags,
			Summary:     hookDescriptor.metadata.Summary,
			Description: hookDescriptor.metadata.Description,
			OperationId: operationID(gvh),
			Responses: &spec3.Responses{
				ResponsesProps: spec3.ResponsesProps{
					StatusCodeResponses: make(map[int]*spec3.Response),
				},
			},
			Deprecated: hookDescriptor.metadata.Deprecated,
		},
	}
	path := GVHToPath(gvh, "")

	// Add name parameter to operation path if necessary.
	// This e.g. reflects the real path which a Runtime Extensions will handle.
	if !hookDescriptor.metadata.Singleton {
		path = GVHToPath(gvh, "{name}")
		operation.Parameters = append(operation.Parameters, &spec3.Parameter{
			ParameterProps: spec3.ParameterProps{
				Name:        "name",
				In:          "path",
				Description: "The handler name. Handlers within a single external component implementing Runtime Extensions must have different names",
				Required:    true,
				Schema: &spec.Schema{
					SchemaProps: spec.SchemaProps{
						Type: []string{"string"},
					},
				},
			},
		})
	}

	// Add request type to operation.
	requestGVK, err := c.Request(gvh)
	if err != nil {
		return err
	}
	requestType, ok := c.scheme.AllKnownTypes()[requestGVK]
	if !ok {
		return errors.Errorf("type for request GVK %q is unknown", requestGVK)
	}
	requestTypeName := typeName(requestType, requestGVK)
	operation.RequestBody = &spec3.RequestBody{
		RequestBodyProps: spec3.RequestBodyProps{
			Content: createContent(requestTypeName),
		},
	}
	if err := addTypeToOpenAPI(openAPI, c, requestTypeName); err != nil {
		return err
	}

	// Add response type to operation.
	responseGVK, err := c.Response(gvh)
	if err != nil {
		return err
	}
	responseType := c.scheme.AllKnownTypes()[responseGVK]
	if !ok {
		return errors.Errorf("type for response GVK %q is unknown", responseGVK)
	}
	responseTypeName := typeName(responseType, responseGVK)
	operation.Responses.StatusCodeResponses[http.StatusOK] = &spec3.Response{
		ResponseProps: spec3.ResponseProps{
			Description: "Status code 200 indicates that the request has been processed successfully. Runtime Extension authors must use fields in the response like e.g. status and message to return processing outcomes.",
			Content:     createContent(responseTypeName),
		},
	}
	if err := addTypeToOpenAPI(openAPI, c, responseTypeName); err != nil {
		return err
	}

	// Add operation to openAPI.
	openAPI.Paths.Paths[path] = &spec3.Path{
		PathProps: spec3.PathProps{
			Post: operation,
		},
	}
	return nil
}

func createContent(typeName string) map[string]*spec3.MediaType {
	return map[string]*spec3.MediaType{
		"application/json": {
			MediaTypeProps: spec3.MediaTypeProps{
				Schema: &spec.Schema{
					SchemaProps: spec.SchemaProps{
						Ref: componentRef(typeName),
					},
				},
			},
		},
	}
}

func addTypeToOpenAPI(openAPI *spec3.OpenAPI, c *Catalog, typeName string) error {
	componentName := componentName(typeName)

	// Check if schema already has been added.
	if _, ok := openAPI.Components.Schemas[componentName]; ok {
		return nil
	}

	// Loop through all OpenAPIDefinitions, so we don't have to lookup typeName => package
	// (which we couldn't do for external packages like clusterv1 because we cannot map typeName
	// to a package without hard-coding the mapping).
	var openAPIDefinition *common.OpenAPIDefinition
	for _, openAPIDefinitionsGetter := range c.openAPIDefinitions {
		openAPIDefinitions := openAPIDefinitionsGetter(componentRef)

		if def, ok := openAPIDefinitions[typeName]; ok {
			openAPIDefinition = &def
			break
		}
	}

	if openAPIDefinition == nil {
		return errors.Errorf("failed to get definition for %v. If you added a new type, you may need to add +k8s:openapi-gen=true to the package or type and run openapi-gen again", typeName)
	}

	// Add schema for component to components.
	openAPI.Components.Schemas[componentName] = &spec.Schema{
		VendorExtensible:   openAPIDefinition.Schema.VendorExtensible,
		SchemaProps:        openAPIDefinition.Schema.SchemaProps,
		SwaggerSchemaProps: openAPIDefinition.Schema.SwaggerSchemaProps,
	}

	// Add schema for dependencies to components recursively.
	for _, d := range openAPIDefinition.Dependencies {
		if err := addTypeToOpenAPI(openAPI, c, d); err != nil {
			return err
		}
	}

	return nil
}

// typeName calculates a type name. This matches the format used in the generated
// GetOpenAPIDefinitions funcs, e.g. "k8s.io/api/core/v1.ObjectReference".
func typeName(t reflect.Type, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s.%s", t.PkgPath(), gvk.Kind)
}

// componentRef calculates a componentRef which is used in the OpenAPI specification
// to reference components in the components section.
func componentRef(typeName string) spec.Ref {
	return spec.MustCreateRef(fmt.Sprintf("#/components/schemas/%s", componentName(typeName)))
}

// componentName calculates the componentName for the OpenAPI specification based on a typeName.
// For example: "k8s.io/api/core/v1.ObjectReference" => "k8s.io.api.core.v1.ObjectReference".
// Note: This is necessary because we cannot use additional Slashes in the componentRef.
func componentName(typeName string) string {
	return strings.ReplaceAll(typeName, "/", ".")
}

// operationID calculates an operationID similar to Kubernetes OpenAPI.
// Kubernetes examples:
// * readRbacAuthorizationV1NamespacedRole
// * listExtensionsV1beta1IngressForAllNamespaces
// In our case:
// * hooksRuntimeClusterV1alpha1Discovery.
func operationID(gvh GroupVersionHook) string {
	shortAPIGroup := strings.TrimSuffix(gvh.Group, ".x-k8s.io")

	split := strings.Split(shortAPIGroup, ".")
	title := cases.Title(language.Und)

	res := split[0]
	for i := 1; i < len(split); i++ {
		res += title.String(split[i])
	}
	res += title.String(gvh.Version) + title.String(gvh.Hook)

	return res
}

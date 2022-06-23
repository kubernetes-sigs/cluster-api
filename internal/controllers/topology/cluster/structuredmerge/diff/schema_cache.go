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

package diff

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gobuffalo/flect"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// CRDSchemaCache caches CRD's Open API schemas.
type CRDSchemaCache interface {
	// LoadOrStore returns the existing value for the gvk if present. Otherwise, it stores and returns the given value.
	LoadOrStore(ctx context.Context, gvk schema.GroupVersionKind) (CRDSchema, error)
}

// NewCRDSchemaCache returns a schema caches implementation that reads schemas from the Open API schema defined in the CRDs.
func NewCRDSchemaCache(c client.Client) CRDSchemaCache {
	return &crdSchemaCache{
		client: c,
		m:      map[schema.GroupVersionKind]CRDSchema{},
	}
}

var _ CRDSchemaCache = &crdSchemaCache{}

type crdSchemaCache struct {
	client client.Client

	lock sync.RWMutex
	m    map[schema.GroupVersionKind]CRDSchema
}

// LoadOrStore returns the CRDSchema value for the gvk, if present.
// Otherwise, if not present, it retrieves the corresponding CRD definition, infer the CRDSchema, stores it, returns it.
func (sc *crdSchemaCache) LoadOrStore(ctx context.Context, gvk schema.GroupVersionKind) (CRDSchema, error) {
	if s, ok := sc.load(gvk); ok {
		return s, nil
	}
	return sc.store(ctx, gvk)
}

// load returns the CRDSchema value for the gvk, if present.
func (sc *crdSchemaCache) load(gvk schema.GroupVersionKind) (CRDSchema, bool) {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	s, ok := sc.m[gvk]
	return s, ok
}

// store retrieves the CRD definition for the gvk, infer the corresponding CRDSchema, stores it, returns it.
func (sc *crdSchemaCache) store(ctx context.Context, gvk schema.GroupVersionKind) (CRDSchema, error) {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	// check if the CRDSchema has been added while waiting for the lock. if yes, return it.
	if s, ok := sc.m[gvk]; ok {
		return s, nil
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	crd.SetName(fmt.Sprintf("%s.%s", flect.Pluralize(strings.ToLower(gvk.Kind)), gvk.Group))
	if err := sc.client.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
		return nil, errors.Wrapf(err, "failed to get CRD for %s", gvk.String())
	}

	schema := CRDSchema{}
	for _, v := range crd.Spec.Versions {
		if v.Name == gvk.Version && v.Schema != nil && v.Schema.OpenAPIV3Schema != nil {
			addToSchema(contract.Path{}, schema, v.Schema.OpenAPIV3Schema)
		}
	}

	sc.m[gvk] = schema
	return schema, nil
}

// addToSchema converts JSONSchemaProps to a CRDSchema.
func addToSchema(p contract.Path, s CRDSchema, c *apiextensionsv1.JSONSchemaProps) {
	if p.String() == "status" {
		return
	}

	switch c.Type {
	case "object":
		// "object" apply to both fields defined as `foo SomeStruct` (struct) or `foo map[string]Something` (maps).

		// If it is field defined as `foo SomeStruct` (struct), Properties are defined for each nested field.
		if c.Properties != nil {
			// NOTE: XMapType is used also for struct.
			st := StructType(pointer.StringDeref(c.XMapType, string(GranularStructType))) // NOTE: by default struct are granular in CRDs.
			s[p.String()] = TypeDef{
				Struct: &StructDef{Type: st},
			}

			// If the struct is defined as atomic, it is not required to process the property's schema given that the entire struct should be treated as an atomic element.
			if st == AtomicStructType {
				return
			}

			// Process the property's schema.
			for field, fieldSchema := range c.Properties {
				fieldSchema := fieldSchema
				addToSchema(p.Append(field), s, &fieldSchema)
			}
		}

		// If it is a field defined as `foo map[string]Something`, AdditionalProperties define the struct of the items.
		if c.AdditionalProperties != nil {
			mt := MapType(pointer.StringDeref(c.XMapType, string(GranularMapType))) // NOTE: by default map are granular in CRDs.
			s[p.String()] = TypeDef{
				Map: &MapDef{Type: mt},
			}
			// If the map is defined as atomic, it is not required to process the item's schema given that each item should be treated as atomic elements.
			if mt == AtomicMapType {
				return
			}

			// Process the item's schema.
			addToSchema(p.Item(), s, c.AdditionalProperties.Schema)
		}

	case "array":
		// "array" apply to all the fields defined as `foo []Something`; how to process such
		// lists depends on +listType and +listMapKey markers.

		// Store the list definition.
		lt := ListType(pointer.StringDeref(c.XListType, string(AtomicListType))) // NOTE: by default list are atomic in CRDs.
		lk := c.XListMapKeys
		s[p.String()] = TypeDef{
			List: &ListDef{
				Type: lt,
				Keys: lk,
			},
		}

		// Process item definition, if required.
		switch lt {
		case ListMapType:
			// Add an entry defining the list item; by design, it is as a struct.
			if c.Items != nil && c.Items.Schema != nil {
				item := p.Item()
				s[item.String()] = TypeDef{
					Struct: &StructDef{
						Type: GranularStructType,
					},
				}
				for field, fieldSchema := range c.Items.Schema.Properties {
					fieldSchema := fieldSchema
					addToSchema(item.Append(field), s, &fieldSchema)
				}
			}
		case ListSetType:
			// It is not required to process the item's schema given that each item of the list set are scalars.
		default: // AtomicListType
			// It is not required to process the item's schema given that items should be treated as atomic elements.
		}
	default:
		// Otherwise, the schema applies to scalar fields, e.g. `foo string`, `foo bool`,
		s[p.String()] = TypeDef{
			Scalar: &ScalarDef{},
		}
	}

	// Add to level fields which are never part of a CRD's OpenAPI schema.
	if p.String() == "" {
		s["apiVersion"] = TypeDef{Scalar: &ScalarDef{}}
		s["kind"] = TypeDef{Scalar: &ScalarDef{}}
	}
	// Expand metadata schema which are store only partially in CRD's OpenAPI schema.
	// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
	if p.String() == "metadata" {
		// NOTE: we add only fields which are relevant for the topology controller.
		s["metadata"] = TypeDef{Struct: &StructDef{Type: GranularStructType}}
		s["metadata.name"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.namespace"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.labels"] = TypeDef{Map: &MapDef{Type: GranularMapType}}
		s["metadata.labels[]"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.annotations"] = TypeDef{Map: &MapDef{Type: GranularMapType}}
		s["metadata.annotations[]"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences"] = TypeDef{List: &ListDef{Type: ListMapType, Keys: []string{"uid"}}}
		s["metadata.ownerReferences[]"] = TypeDef{Struct: &StructDef{Type: GranularStructType}}
		s["metadata.ownerReferences[].apiVersion"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences[].blockOwnerDeletion"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences[].controller"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences[].kind"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences[].name"] = TypeDef{Scalar: &ScalarDef{}}
		s["metadata.ownerReferences[].uid"] = TypeDef{Scalar: &ScalarDef{}}
	}
}

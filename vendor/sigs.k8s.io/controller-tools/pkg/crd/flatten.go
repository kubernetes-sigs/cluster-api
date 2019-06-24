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

package crd

import (
	"fmt"
	"strings"
	"sync"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"sigs.k8s.io/controller-tools/pkg/loader"
)

// flattenAllOfInto copies properties from src to dst, then copies the properties
// of each item in src's allOf to dst's properties as well.
func flattenAllOfInto(dst *apiext.JSONSchemaProps, src apiext.JSONSchemaProps) {
	if len(src.AllOf) > 0 {
		for _, embedded := range src.AllOf {
			flattenAllOfInto(dst, embedded)
		}
	}

	for propName, prop := range src.Properties {
		dst.Properties[propName] = prop
	}
	for _, propName := range src.Required {
		dst.Required = append(dst.Required, propName)
	}
}

// allOfVisitor recursively visits allOf fields in the schema,
// merging nested allOf properties into the root schema.
type allOfVisitor struct{}

func (v allOfVisitor) Visit(schema *apiext.JSONSchemaProps) SchemaVisitor {
	if schema == nil {
		return v
	}
	var outAllOf []apiext.JSONSchemaProps
	for _, embedded := range schema.AllOf {
		if embedded.Ref != nil && len(*embedded.Ref) > 0 {
			outAllOf = append(outAllOf, embedded)
			continue
		}
		// NB(directxman12): only certain schemata are possible here
		// (only a normal struct can be embedded), so we only need
		// to deal with properties, allof, and required
		if len(embedded.Properties) == 0 && len(embedded.AllOf) == 0 {
			outAllOf = append(outAllOf, embedded)
			continue
		}
		flattenAllOfInto(schema, embedded)
	}
	schema.AllOf = outAllOf
	return v
}

// NB(directxman12): FlattenEmbedded is separate from Flattener because
// some tooling wants to flatten out embedded fields, but only actually
// flatten a few specific types first.

// FlattenEmbedded flattens embedded fields (represented via AllOf) which have
// already had their references resolved into simple properties in the containing
// schema.
func FlattenEmbedded(schema *apiext.JSONSchemaProps) *apiext.JSONSchemaProps {
	outSchema := schema.DeepCopy()
	EditSchema(outSchema, allOfVisitor{})
	return outSchema
}

// Flattener knows how to take a root type, and flatten all references in it
// into a single, flat type.  Flattened types are cached, so it's relatively
// cheap to make repeated calls with the same type.
type Flattener struct {
	// Parser is used to lookup package and type details, and parse in new packages.
	Parser *Parser

	// flattenedTypes hold the flattened version of each seen type for later reuse.
	flattenedTypes map[TypeIdent]apiext.JSONSchemaProps
	initOnce       sync.Once
}

func (f *Flattener) init() {
	f.initOnce.Do(func() {
		f.flattenedTypes = make(map[TypeIdent]apiext.JSONSchemaProps)
	})
}

// cacheType saves the flattened version of the given type for later reuse
func (f *Flattener) cacheType(typ TypeIdent, schema apiext.JSONSchemaProps) {
	f.init()
	f.flattenedTypes[typ] = schema
}

// loadUnflattenedSchema fetches a fresh, unflattened schema from the parser.
func (f *Flattener) loadUnflattenedSchema(typ TypeIdent) (*apiext.JSONSchemaProps, error) {
	f.Parser.NeedSchemaFor(typ)

	baseSchema, found := f.Parser.Schemata[typ]
	if !found {
		return nil, fmt.Errorf("unable to locate schema for type %s", typ)
	}
	return &baseSchema, nil
}

// FlattenType flattens the given pre-loaded type, removing any references from it.
// It deep-copies the schema first, so it won't affect the parser's version of the schema.
func (f *Flattener) FlattenType(typ TypeIdent) *apiext.JSONSchemaProps {
	f.init()
	if cachedSchema, isCached := f.flattenedTypes[typ]; isCached {
		return &cachedSchema
	}
	baseSchema, err := f.loadUnflattenedSchema(typ)
	if err != nil {
		typ.Package.AddError(err)
		return nil
	}
	resSchema := f.FlattenSchema(*baseSchema, typ.Package)
	f.cacheType(typ, *resSchema)
	return resSchema
}

// FlattenSchema flattens the given schema, removing any references.
// It deep-copies the schema first, so the input schema won't be affected.
func (f *Flattener) FlattenSchema(baseSchema apiext.JSONSchemaProps, currentPackage *loader.Package) *apiext.JSONSchemaProps {
	resSchema := baseSchema.DeepCopy()
	EditSchema(resSchema, &flattenVisitor{
		Flattener:      f,
		currentPackage: currentPackage,
	})

	return resSchema
}

// identFromRef converts the given schema ref from the given package back
// into the TypeIdent that it represents.
func identFromRef(ref string, contextPkg *loader.Package) (TypeIdent, error) {
	if !strings.HasPrefix(ref, defPrefix) {
		return TypeIdent{}, fmt.Errorf("non-standard reference link %q", ref)
	}
	ref = ref[len(defPrefix):]
	// decode the json pointer encodings
	ref = strings.Replace(ref, "~1", "/", -1)
	ref = strings.Replace(ref, "~0", "~", -1)
	nameParts := strings.SplitN(ref, "~", 2)
	if len(nameParts) == 1 {
		// a local reference
		return TypeIdent{
			Name:    nameParts[0],
			Package: contextPkg,
		}, nil
	}

	// an external reference
	return TypeIdent{
		Name:    nameParts[1],
		Package: contextPkg.Imports()[nameParts[0]],
	}, nil
}

// preserveFields copies documentation fields from src into dst, preserving
// field-level documentation when flattening.
func preserveFields(dst *apiext.JSONSchemaProps, src apiext.JSONSchemaProps) {
	// TODO(directxman12): preserve both description fields?
	dst.Description = src.Description
	dst.Title = src.Title
	dst.Example = src.Example
	if src.Example != nil {
		dst.Example = src.Example
	}
	if src.ExternalDocs != nil {
		dst.ExternalDocs = src.ExternalDocs
	}

	// TODO(directxman12): copy over other fields?
}

// flattenVisitor visits each node in the schema, recursively flattening references.
type flattenVisitor struct {
	*Flattener

	currentPackage *loader.Package
	currentType    *TypeIdent
	currentSchema  *apiext.JSONSchemaProps
}

func (f *flattenVisitor) Visit(baseSchema *apiext.JSONSchemaProps) SchemaVisitor {
	if baseSchema == nil {
		// end-of-node marker, cache the results
		if f.currentType != nil {
			f.cacheType(*f.currentType, *f.currentSchema)
		}
		return f
	}

	// if we get a type that's just a ref, resolve it
	if baseSchema.Ref != nil && len(*baseSchema.Ref) > 0 {
		// resolve this ref
		refIdent, err := identFromRef(*baseSchema.Ref, f.currentPackage)
		if err != nil {
			f.currentPackage.AddError(err)
			return nil
		}

		// load and potentially flatten the schema

		// check the cache first...
		if refSchemaCached, isCached := f.flattenedTypes[refIdent]; isCached {
			// shallow copy is fine, it's just to avoid overwriting the doc fields
			preserveFields(&refSchemaCached, *baseSchema)
			*baseSchema = refSchemaCached
			return nil // don't recurse, we're done
		}

		// ...otherwise, we need to flatten
		refSchema, err := f.loadUnflattenedSchema(refIdent)
		if err != nil {
			f.currentPackage.AddError(err)
			return nil
		}
		refSchema = refSchema.DeepCopy()
		preserveFields(refSchema, *baseSchema)
		*baseSchema = *refSchema

		// avoid loops (which shouldn't exist, but just in case)
		// by marking a nil cached pointer before we start recursing
		f.cacheType(refIdent, apiext.JSONSchemaProps{})

		return &flattenVisitor{
			Flattener: f.Flattener,

			currentPackage: refIdent.Package,
			currentType:    &refIdent,
			currentSchema:  baseSchema,
		}
	}

	// otherwise, continue recursing...
	if f.currentType != nil {
		// ...but don't accidentally end this node early (for caching purposes)
		return &flattenVisitor{
			Flattener:      f.Flattener,
			currentPackage: f.currentPackage,
		}
	}

	return f
}

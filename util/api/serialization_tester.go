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

// Package api provides utils to ensure quality of APIs.
package api

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"gomodules.xyz/jsonpatch/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/set"
)

// SerializationTester provides the ability to test API types for inconsistencies between what the API server
// accepts, which is based on generated CRD schemas, and Marshal/Unmarshal of the corresponding golang types.
// Ensuring consistency between this two dimensions of our API (CRD and golang types) is key for
// use cases when it is required to make assumptions on what diff exists between an object being computed in a
// controller and the same object stored in the API server, like e.g the topology controller and RuntimeExtensions.
type SerializationTester struct {
	customFillFuncs    map[string]any
	acknowledgedErrors map[string]string

	decoder runtime.Decoder

	testObj runtime.Object
	u       map[string]interface{}
	n       int

	cache set.Set[string]
}

// NewSerializationTester returns a new SerializationTester.
func NewSerializationTester() *SerializationTester {
	return &SerializationTester{}
}

// WithCustomFillFunctions allows to set custom fill functions to be used by the SerializationTester.
func (w *SerializationTester) WithCustomFillFunctions(fns map[string]any) *SerializationTester {
	return &SerializationTester{
		customFillFuncs: fns,
	}
}

// AcknowledgedErrors allows to set errors to be acknowledged by the SerializationTester.
func (w *SerializationTester) AcknowledgedErrors(fns map[string]string) *SerializationTester {
	return &SerializationTester{
		acknowledgedErrors: fns,
	}
}

// FillAndTest an object incrementally (taking into account required fields), and runs serialization tests at every step.
func (w *SerializationTester) FillAndTest(scheme *runtime.Scheme, obj runtime.Object, crdPath string) error {
	// Create a copy of the object to be used as the openAPISchema used to drive filling the object json/yaml.
	w.testObj = obj.DeepCopyObject()

	gvks, _, _ := scheme.ObjectKinds(w.testObj)
	if len(gvks) != 1 {
		return errors.New("failed to get a unique gvk for the object")
	}

	// Read the CRD, check it matches with the obj kind & version, and get the open API openAPISchema for the object.
	w.decoder = serializer.NewCodecFactory(scheme).UniversalDecoder(gvks[0].GroupVersion())

	crdBytes, err := os.ReadFile(crdPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, "failed to read crd")
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	_, _, err = w.decoder.Decode(crdBytes, nil, crd)
	if err != nil {
		return errors.Wrap(err, "failed to decode crd")
	}

	if !strings.EqualFold(crd.Spec.Names.Singular, gvks[0].Kind) {
		return errors.New("crd.Spec.Names.Singular does not match obj Kind")
	}

	var openAPISchema *apiextensionsv1.JSONSchemaProps
	for _, version := range crd.Spec.Versions {
		if version.Name == gvks[0].Version {
			openAPISchema = version.Schema.OpenAPIV3Schema
			break
		}
	}

	if openAPISchema == nil {
		return errors.Errorf("crd does not have version %s", w.testObj.GetObjectKind().GroupVersionKind().String())
	}

	// Start testing from the object itself, which is the root of the tree of fields that
	// is going to be filled.
	// NOTE: The object at this stage will be empty (except for TypeMeta and ObjectMeta, if added).

	errs := []error{}
	root := w.newRootField(w.testObj, gvks[0], openAPISchema)
	ctx := withField(nil, root)
	w.n++
	rootCtx := withTestCase(ctx, w.n, "%s without spec", root.Path())
	if err := w.doTest(rootCtx); err != nil {
		errs = append(errs, err)
	}

	// The test will focus on the spec field only, and the goes recursively through all the spec's nested fields.
	spec := root.FieldByName("spec")
	ctx = withField(ctx, spec)
	if err := w.doFill(ctx); err != nil {
		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

func (w *SerializationTester) newRootField(obj runtime.Object, gvk schema.GroupVersionKind, openAPISchema *apiextensionsv1.JSONSchemaProps) *apiField {
	w.u, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(obj)

	delete(w.u, "spec")
	delete(w.u, "status")

	return &apiField{
		path:              field.NewPath(gvk.Kind),
		openAPISchemaPath: field.NewPath(gvk.Kind),
		openAPISchema:     openAPISchema,
		uGetter: func() any {
			return w.u
		},
	}
}

const (
	apiTypeBoolean = "boolean"
	apiTypeString  = "string"
	apiTypeInteger = "integer"
	apiTypeArray   = "array"
	apiTypeObject  = "object"
)

func (w *SerializationTester) doFill(ctx serializationTestContext) error {
	field := ctx.Field()
	ctx = withMessage(ctx, "fill %s value: %s", field.Path(), field.Value())

	// If there is a custom field filler for this field, use it to set the field, and then run the test.
	if w.tryCustom(field) {
		w.n++
		ctx := withTestCase(ctx, w.n, "test: %s set with custom fill function", field.Path())
		return w.doTest(ctx)
	}

	errs := []error{}

	switch field.Type() {
	case apiTypeBoolean:
		if field.Value() == nil {
			if field.isRequired {
				field.SetValue(false)
				w.n++
				ctx := withTestCase(ctx, w.n, "fill: %s set to false", field.Path())
				if err := w.doTest(ctx); err != nil {
					errs = append(errs, err)
				}
			} else {
				ctx = withMessage(ctx, "fill: %s not set to false because it is isRequired", field.Path())
			}
		}
		field.SetValue(true)
		w.n++
		ctx := withTestCase(ctx, w.n, "fill: %s set to true", field.Path())
		if err := w.doTest(ctx); err != nil {
			errs = append(errs, err)
		}
	case apiTypeString:
		if field.Value() == nil {
			if field.isRequired {
				field.SetValue("")
				w.n++
				ctx := withTestCase(ctx, w.n, "fill: %s set to \"\"", field.Path())
				if err := w.doTest(ctx); err != nil {
					errs = append(errs, err)
				}
			} else {
				ctx = withMessage(ctx, "fill: %s not set to \"\" because it is isRequired", field.Path())
			}
		}
		field.SetValue("foo")
		w.n++
		ctx := withTestCase(ctx, w.n, "fill: %s set to \"foo\"", field.Path())
		if err := w.doTest(ctx); err != nil {
			errs = append(errs, err)
		}
	case apiTypeInteger:
		if field.Value() == nil && field.isRequired {
			if field.isRequired {
				field.SetValue(0)
				w.n++
				ctx := withTestCase(ctx, w.n, "fill: %s set to 0", field.Path())
				if err := w.doTest(ctx); err != nil {
					errs = append(errs, err)
				}
			} else {
				ctx = withMessage(ctx, "fill: %s not set to 0 because it is isRequired", field.Path())
			}
		}
		field.SetValue(123)
		w.n++
		ctx := withTestCase(ctx, w.n, "fill: %s set with 123", field.Path())
		if err := w.doTest(ctx); err != nil {
			errs = append(errs, err)
		}
	case apiTypeArray:
		// If the array is still nit, initialize it before staring adding items
		// Also, run a test case with the empty array.
		if field.Value() == nil {
			field.SetValue(make([]any, 0))
			w.n++
			ctx := withTestCase(ctx, w.n, "fill: %s set to empty slice", field.Path())
			if err := w.doTest(ctx); err != nil {
				errs = append(errs, err)
			}
		}

		// Then add one item at time to the slide and fill it.
		n := 2
		for i := range n {
			itemField := field.Index(i)

			itemCtx := withField(ctx, itemField)
			if err := w.doFill(itemCtx); err != nil {
				errs = append(errs, err)
			}
		}
	case apiTypeObject:
		// Default value for fields of type struct is tested at the beginning of this func.

		// If the object has isRequired fields, they must be set (API server do not accept a yaml without them)
		// in case this operation changes the object, run a test.
		var isChanged bool
		ctx, isChanged = w.setRequiredFields(ctx, field)
		if isChanged {
			w.n++
			ctx := withTestCase(ctx, w.n, "fill: %s set with required fields", field.Path())
			if err := w.doTest(ctx); err != nil {
				errs = append(errs, err)
			}
		}

		// If after adding isRequired fields the object is still nit, initialize it before staring adding fields
		// Also, run a test case with the empty object.
		if field.Value() == nil {
			w.n++
			ctx := withTestCase(ctx, w.n, "fill: %s set to empty struct", field.Path())
			field.SetValue(map[string]any{})
			if err := w.doTest(ctx); err != nil {
				errs = append(errs, err)
			}
		}

		// Then add fields one by one and fill them too.
		for i := range field.NumField() {
			structField := field.Field(i)
			structCtx := withField(ctx, structField)
			if err := w.doFill(structCtx); err != nil {
				errs = append(errs, err)
			}

			// After filling one field / testing all its permutation, clean it up so we keep the yaml surface as small as possible.
			structField.UnsetSetValue()
			if structField.IsRequired() {
				w.setRequiredFields(structCtx, structField)
			}
		}

		if field.IsMap() {
			n := 2
			for i := range n {
				key := fmt.Sprintf("key%d", i)
				elemField := field.Key(key)

				elemCtx := withField(ctx, elemField)
				if err := w.doFill(elemCtx); err != nil {
					errs = append(errs, err)
				}
			}
		}
	default:
		panic(fmt.Sprintf("dofill for type %s is not implemented yet", field.Type()))
	}

	return kerrors.NewAggregate(errs)
}

func (w *SerializationTester) setRequiredFields(ctx serializationTestContext, field *apiField) (serializationTestContext, bool) {
	valueBeforeFillRequired := field.Value()

	if w.tryCustom(field) {
		ctx = withMessage(ctx, "field %s set with custom fill function (required field)", field.Path())
		return ctx, !reflect.DeepEqual(field.Value(), valueBeforeFillRequired)
	}

	switch field.Type() {
	case apiTypeString:
		if field.Value() == nil {
			ctx = withMessage(ctx, "field %s set to \"\" (required field)", field.Path())
			field.SetValue("")
		}
	case apiTypeInteger:
		if field.Value() == nil {
			ctx = withMessage(ctx, "field %s set to 0 (required field)", field.Path())
			field.SetValue(0)
		}
	case apiTypeObject:
		requiredFieldNames := field.RequiredFieldNames()

		if (field.Value() == nil || field.Value().(map[string]any) == nil) && len(requiredFieldNames) > 0 {
			ctx = withMessage(ctx, "field %s initialized to host nested required fields (%s)", field.Path(), strings.Join(requiredFieldNames, ","))
			field.SetValue(map[string]any{})
		}
		if (field.Value() == nil || field.Value().(map[string]any) == nil) && field.isRequired {
			ctx = withMessage(ctx, "field %s set to empty object (required field)", field.Path())
			field.SetValue(map[string]any{})
		}
		for i := range field.NumField() {
			structField := field.Field(i)
			if structField.isRequired {
				ctx, _ = w.setRequiredFields(ctx, structField)
			}
		}
	default:
		panic(fmt.Sprintf("setRequiredFields for type %s is not implemented yet", field.Type()))
	}

	return ctx, !reflect.DeepEqual(field.Value(), valueBeforeFillRequired)
}

func (w *SerializationTester) tryCustom(field *apiField) bool {
	if v, ok := w.customFillFuncs[field.openAPISchemaPath.String()]; ok {
		field.SetValue(v)
		return true
	}
	return false
}

func (w *SerializationTester) doTest(ctx serializationTestContext) error {
	testNumber, testTitle := ctx.TestCase()
	field := ctx.Field()

	fmt.Println("---")
	fmt.Printf("TEST #%d: %s\n", testNumber, testTitle)
	fmt.Println("Path: ", field.Path())
	fmt.Println("OpenAPISchemaPath: ", field.OpenAPISchemaPath(), " isRequired: ", field.IsRequired())
	for _, m := range ctx.Messages() {
		fmt.Println(" - ", m)
	}

	fmt.Println()
	// Marshal the unstructured object to json (called original like the input object for the walker in the topology mutation hook)
	jsonOriginal, err := json.Marshal(w.u)
	if err != nil {
		panic(err)
	}
	fmt.Println("Test: ", string(jsonOriginal))

	// If the generated json has been already tested, exit.
	if w.cache == nil {
		w.cache = set.New[string]()
	}
	if w.cache.Has(string(jsonOriginal)) {
		fmt.Println("already tested")
		return nil
	}
	w.cache.Insert(string(jsonOriginal))

	// Otherwise run the test

	// Decode the json into an object (called modified like the object to which the walker in the topology mutation hook applies patches)
	modified := w.testObj.DeepCopyObject()
	modified, _, err = w.decoder.Decode(jsonOriginal, nil, modified)
	if err != nil {
		panic(err)
	}

	// Compute the patch to align original and modified, there should be no patch operations because we are not applying patches.
	// If there are patch operations, it means that the API type in go applies some defaults different to json's defaults.

	jsonModified, err := json.Marshal(modified)
	if err != nil {
		panic(err)
	}

	patch, err := jsonpatch.CreatePatch(jsonOriginal, jsonModified)
	if err != nil {
		panic(err)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		panic(err)
	}
	fmt.Println("Diff: ", string(patchBytes))
	fmt.Println()
	if len(patchBytes) > 2 {
		if acknowledgedDiff := w.acknowledgedErrors[testTitle]; acknowledgedDiff == string(patchBytes) {
			fmt.Println("diff acknowledged")
			return nil
		}
		return errors.Errorf("TEST #%d %s is generating a diff: %s", testNumber, testTitle, string(patchBytes))
	}

	return nil
}

type apiField struct {
	path              *field.Path
	openAPISchemaPath *field.Path
	openAPISchema     *apiextensionsv1.JSONSchemaProps
	isRequired        bool
	// value             any

	uGetter   func() any
	uSetter   func(v any)
	uUnSetter func()
}

func (f *apiField) Name() string {
	if i := strings.LastIndex(f.openAPISchemaPath.String(), "."); i > 0 {
		return f.openAPISchemaPath.String()[i+1:]
	}
	return f.openAPISchemaPath.String()
}

func (f *apiField) Type() string {
	return f.openAPISchema.Type
}

func (f *apiField) Path() *field.Path {
	return f.path
}

func (f *apiField) OpenAPISchemaPath() *field.Path {
	return f.openAPISchemaPath
}

func (f *apiField) Value() any {
	if f.uGetter == nil {
		panic("uGetter function is not defined!")
	}
	return f.uGetter()
}

func (f *apiField) NumField() int {
	return len(f.openAPISchema.Properties)
}

func (f *apiField) IsRequired() bool {
	return f.isRequired
}

func (f *apiField) RequiredFieldNames() []string {
	return f.openAPISchema.Required
}

func (f *apiField) IsMap() bool {
	return f.openAPISchema.AdditionalProperties != nil
}

func (f *apiField) SetValue(v any) {
	if f.uSetter == nil {
		panic("uSetter function is not defined!")
	}
	// f.value = v
	f.uSetter(v)
}

func (f *apiField) UnsetSetValue() {
	if f.uUnSetter == nil {
		panic("uUnSetter function is not defined!")
	}
	// f.value = nil
	f.uUnSetter()
}

// Field returns a struct type's i'th field.
func (f *apiField) Field(i int) *apiField {
	names := make([]string, 0, len(f.openAPISchema.Properties))
	for k := range f.openAPISchema.Properties {
		names = append(names, k)
	}
	sort.Strings(names)
	return f.FieldByName(names[i])
}

// FieldByName returns the struct field with the given name.
func (f *apiField) FieldByName(name string) *apiField {
	if f.Type() != apiTypeObject {
		panic(fmt.Sprintf("field %s is not an object!", f.Name()))
	}
	fi := &apiField{
		path:              f.path.Child(name),
		openAPISchemaPath: f.openAPISchemaPath.Child(name),
		openAPISchema: func() *apiextensionsv1.JSONSchemaProps {
			// Get the open API openAPISchema for the field
			for n, p := range f.openAPISchema.Properties {
				if n == name {
					return &p
				}
			}
			panic("cannot find openapi definition for field " + name)
		}(),
		isRequired: func() bool {
			// Get the isRequired setting for the field
			for _, p := range f.openAPISchema.Required {
				if p == name {
					return true
				}
			}
			return false
		}(),
		// Func to get the field value from u
		uGetter: func() any {
			structValue, ok := f.Value().(map[string]any)
			if !ok {
				return nil
			}
			if structValue == nil {
				return nil
			}
			return structValue[name]
		},
		// Func to set the field value back in u (the root value)
		uSetter: func(v any) {
			structValue, ok := f.Value().(map[string]any)
			if !ok {
				panic(fmt.Sprintf("fieldValue for field %s is not a map[string]any!", f.Name()))
			}
			if structValue == nil {
				structValue = make(map[string]any)
			}
			structValue[name] = v
		},
		// Func to unset the field value back in u (the root value)
		uUnSetter: func() {
			structValue, ok := f.Value().(map[string]any)
			if !ok {
				panic(fmt.Sprintf("fieldValue for field %s is not a map[string]any!", f.Name()))
			}
			delete(structValue, name)
		},
	}

	return fi
}

// Index returns v's i'th element.
func (f *apiField) Index(i int) *apiField {
	if f.Type() != apiTypeArray {
		panic(fmt.Sprintf("field %s is not an array!", f.Name()))
	}
	fi := &apiField{
		path:              f.path.Index(i),
		openAPISchemaPath: f.openAPISchemaPath,
		openAPISchema: func() *apiextensionsv1.JSONSchemaProps {
			// In open API there the openAPISchema of an item of an array is defined in Items.Schema of the array object itself.
			return f.openAPISchema.Items.Schema
		}(),
		isRequired: func() bool {
			// When we get the item in a array, it should be considered isRequired.
			return true
		}(),
		// Func to get the field value from u
		uGetter: func() any {
			array, ok := f.Value().([]any)
			if !ok {
				return nil
			}
			if len(array) < i+1 {
				return nil
			}
			return array[i]
		},
		// Func to set the field value back in u (the root value)
		uSetter: func(v any) {
			array, ok := f.Value().([]any)
			if !ok {
				panic(fmt.Sprintf("fieldValue for field %s is not a []any!", f.Name()))
			}

			// Extend the array if necessary
			missingItems := i + 1 - len(array)
			for range missingItems {
				array = append(array, nil)
			}
			array[i] = v

			f.SetValue(array)
		},
	}

	return fi
}

// Key returns key's element.
func (f *apiField) Key(key string) *apiField {
	if f.Type() != apiTypeObject && !f.IsMap() {
		panic(fmt.Sprintf("field %s is not a map!", f.Name()))
	}
	fi := &apiField{
		path:              f.path.Key(key),
		openAPISchemaPath: f.openAPISchemaPath,
		openAPISchema: func() *apiextensionsv1.JSONSchemaProps {
			// In open API there the openAPISchema of an element of a map is the AdditionalProperties in the openAPISchema of the map object itself.
			return f.openAPISchema.AdditionalProperties.Schema
		}(),
		isRequired: func() bool {
			// When we get the elem in a map, it should be considered isRequired.
			return true
		}(),
		// Func to get the field value from u
		uGetter: func() any {
			structValue, ok := f.Value().(map[string]any)
			if !ok {
				return nil
			}
			if structValue == nil {
				return nil
			}
			return structValue[key]
		},
		// Func to set the field value back in u (the root value)
		uSetter: func(v any) {
			structValue, ok := f.Value().(map[string]any)
			if !ok {
				panic(fmt.Sprintf("fieldValue for field %s is not a map[string]any!", f.Name()))
			}
			structValue[key] = v
		},
	}

	return fi
}

type serializationTestContext interface {
	Field() *apiField
	Messages() []string
	TestCase() (int, string)
}

type fieldCtx struct {
	serializationTestContext
	field *apiField
}

func withField(parent serializationTestContext, field *apiField) serializationTestContext {
	return &fieldCtx{
		serializationTestContext: parent,
		field:                    field,
	}
}

func (c *fieldCtx) Field() *apiField {
	return getField(c)
}

func (c *fieldCtx) Messages() []string {
	return messages(c)
}

func (c *fieldCtx) TestCase() (int, string) {
	return testCase(c)
}

type msgCtx struct {
	serializationTestContext
	message string
}

func withMessage(parent serializationTestContext, format string, a ...any) serializationTestContext {
	return &msgCtx{
		serializationTestContext: parent,
		message:                  fmt.Sprintf(format, a...),
	}
}

func (c *msgCtx) Field() *apiField {
	return getField(c)
}

func (c *msgCtx) Messages() []string {
	return messages(c)
}

func (c *msgCtx) TestCase() (int, string) {
	return testCase(c)
}

type testCaseCtx struct {
	serializationTestContext
	n     int
	title string
}

func withTestCase(parent serializationTestContext, n int, format string, a ...any) serializationTestContext {
	return &testCaseCtx{
		serializationTestContext: parent,
		n:                        n,
		title:                    fmt.Sprintf(format, a...),
	}
}

func (c *testCaseCtx) Field() *apiField {
	return getField(c)
}

func (c *testCaseCtx) Messages() []string {
	return messages(c)
}

func (c *testCaseCtx) TestCase() (int, string) {
	return testCase(c)
}

func getField(c serializationTestContext) *apiField {
	switch ctx := c.(type) {
	case *fieldCtx:
		return ctx.field
	case *msgCtx:
		return getField(ctx.serializationTestContext)
	case *testCaseCtx:
		return getField(ctx.serializationTestContext)
	default:
		return nil
	}
}

func messages(c serializationTestContext) []string {
	var messages []string
	for {
		if c == nil {
			slices.Reverse(messages)
			return messages
		}

		switch ctx := c.(type) {
		case *fieldCtx:
			c = ctx.serializationTestContext
		case *msgCtx:
			messages = append(messages, ctx.message)
			c = ctx.serializationTestContext
		case *testCaseCtx:
			c = ctx.serializationTestContext
		}
	}
}

func testCase(c serializationTestContext) (int, string) {
	for {
		if c == nil {
			return 0, ""
		}

		switch ctx := c.(type) {
		case *fieldCtx:
			c = ctx.serializationTestContext
		case *msgCtx:
			c = ctx.serializationTestContext
		case *testCaseCtx:
			return ctx.n, ctx.title
		}
	}
}

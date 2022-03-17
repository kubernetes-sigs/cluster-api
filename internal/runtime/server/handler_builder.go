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

package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

type Handler interface{}

type GroupVersionHookName struct {
	catalog.GroupVersionHook
	Name string
}

type HandlerBuilder struct {
	catalog       *catalog.Catalog
	hookToHandler map[GroupVersionHookName]Handler
}

func NewHandlerBuilder() *HandlerBuilder {
	return &HandlerBuilder{
		hookToHandler: map[GroupVersionHookName]Handler{},
	}
}

func (bld *HandlerBuilder) WithCatalog(c *catalog.Catalog) *HandlerBuilder {
	bld.catalog = c
	return bld
}

func (bld *HandlerBuilder) AddDiscovery(hook catalog.Hook, h Handler) *HandlerBuilder {
	return bld.AddExtension(hook, "", h)
}

func (bld *HandlerBuilder) AddExtension(hook catalog.Hook, name string, h Handler) *HandlerBuilder {
	gvh, err := bld.catalog.GroupVersionHook(hook)
	if err != nil {
		panic(errors.Wrapf(err, "hook does not exist in catalog"))
	}
	gvhn := GroupVersionHookName{
		GroupVersionHook: gvh,
		Name:             name,
	}

	bld.hookToHandler[gvhn] = h
	return bld
}

func (bld *HandlerBuilder) Build() (http.Handler, error) {
	if bld.catalog == nil {

	}

	r := mux.NewRouter()

	for g, h := range bld.hookToHandler {
		gvhn := g
		handler := h

		in, err := bld.catalog.NewRequest(gvhn.GroupVersionHook)
		if err != nil {
			return nil, err
		}

		out, err := bld.catalog.NewResponse(gvhn.GroupVersionHook)
		if err != nil {
			return nil, err
		}

		// TODO: please use catalog.ValidateRequest/Response.
		// TODO: add context
		if err := validateF(handler, in, out); err != nil {
			return nil, err
		}

		fWrapper := func(w http.ResponseWriter, r *http.Request) {

			reqBody, err := ioutil.ReadAll(r.Body)
			if err != nil {
				// TODO: handle error
			}

			request, err := bld.catalog.NewRequest(gvhn.GroupVersionHook)
			if err != nil {
				// TODO: handle error
			}

			if err := json.Unmarshal(reqBody, request); err != nil {
				// TODO: handle error
			}

			response, err := bld.catalog.NewResponse(gvhn.GroupVersionHook)
			if err != nil {
				// TODO: handle error
			}

			// TODO: build new context with correlation ID and pass it to the call
			// TODO: context with Cancel to enforce timeout? enforce timeout on caller side? both?

			v := reflect.ValueOf(handler)
			ret := v.Call([]reflect.Value{
				reflect.ValueOf(request),
				reflect.ValueOf(response),
			})

			if !ret[0].IsNil() {
				// TODO: handle error
			}

			respBody, err := json.Marshal(response)
			if err != nil {
				// TODO: handle error
			}

			w.WriteHeader(http.StatusOK)
			w.Write(respBody)
		}

		r.HandleFunc(catalog.GVHToPath(gvhn.GroupVersionHook, gvhn.Name), fWrapper).Methods("POST")
	}

	return r, nil
}

func validateF(f interface{}, params ...interface{}) error {
	funcType := reflect.TypeOf(f)

	if funcType.NumIn() != len(params) {
		return errors.New("InvocationCausedPanic called with a function and an incorrect number of parameter(s).")
	}

	for paramIndex, paramValue := range params {
		expectedType := funcType.In(paramIndex)
		actualType := reflect.TypeOf(paramValue)

		if actualType != expectedType {
			return errors.Errorf("InvocationCausedPanic called with a mismatched parameter type [parameter #%v: expected %v; got %v].", paramIndex, expectedType, actualType)
		}
	}

	// TODO: check return is error

	return nil
}

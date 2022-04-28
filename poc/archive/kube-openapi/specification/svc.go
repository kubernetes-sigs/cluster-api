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

package main

import (
	"fmt"
	"strings"

	"github.com/emicklei/go-restful"
	restfulspec "github.com/emicklei/go-restful-openapi"
	"k8s.io/kube-openapi/pkg/util"
)

// CreateWebServices hard-codes a simple WebService which only defines a GET path
// for testing.
func CreateWebServices() []*restful.WebService {
	w := new(restful.WebService)
	w.Route(buildRouteForType(w, "test", "Foo"))

	return []*restful.WebService{w}
}

// Implements OpenAPICanonicalTypeNamer
var _ = util.OpenAPICanonicalTypeNamer(&typeNamer{})

type responseNamer struct {
	name string
}

func (t *responseNamer) OpenAPICanonicalTypeName() string {
	return fmt.Sprintf("%s", t.name)
}

type typeNamer struct {
	pkg  string
	name string
}

func (t *typeNamer) OpenAPICanonicalTypeName() string {
	return fmt.Sprintf("sigs.k8s.io/cluster-api/rte/idl/%s.%s", t.pkg, t.name)
}

func buildRouteForType(ws *restful.WebService, pkg, name string) *restful.RouteBuilder {
	namer := typeNamer{
		pkg:  pkg,
		name: name,
	}

	tags := []string{"cluster-api"}

	return ws.POST(fmt.Sprintf("%s/%s", pkg, strings.ToLower(name))).
		// Dummy func required for the root to exist.
		To(func(*restful.Request, *restful.Response) {}).

		// Metadata.
		// TODO Find out how to define summary, it could make the spec nicer
		// TODO Tags seems to not work does not work, it could make the spec nicer
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Doc("get all users").            // TODO: k8s.io/kube-openapi use this for description, while github.com/emicklei/go-restful-openapi use it for summary
		Notes("get all users and more"). // TODO: k8s.io/kube-openapi ignore this, while "github.com/emicklei/go-restful-openapi" use it for description
		Operation("doSomething").

		// Input.
		Consumes("application/json").
		Reads(&namer, "description").

		// Output.
		// TODO Response seems to not work, it could make the spec nicer
		// .Returns(404, "NotFound", responseNamer{name: "NotFound"})  //Note
		Produces("application/json").
		// Returns(200, "OK", &namer)
		Writes(&namer)
}

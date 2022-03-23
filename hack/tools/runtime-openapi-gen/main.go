package main

import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha2"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog/openapi"
)

// TODO: move to tools.
// TODO: use klogs.

var c = catalog.New()

func init() {
	// TODO: how to make this dynamic (automatically discover all the extensions or use input paths)
	v1alpha1.AddToCatalog(c)
	v1alpha2.AddToCatalog(c)
	v1alpha3.AddToCatalog(c)
}

func main() {
	spec := openapi.NewSpecBuilder().WithCatalog(c).Build()

	openAPI, err := spec.OpenAPI()
	if err != nil {
		panic(fmt.Sprintf("OpenAPI error: %v", err))
	}

	// Marshal the swagger spec into JSON, then write it out.
	openAPIBytes, err := json.MarshalIndent(openAPI, " ", " ")
	if err != nil {
		panic(fmt.Sprintf("MarshalIndent error: %v", err))
	}

	// TODO: make the output path a flag
	err = os.WriteFile("openapi.json", openAPIBytes, 0600)
	if err != nil {
		panic(fmt.Sprintf("WriteFile error: %v", err))
	}
}

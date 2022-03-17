package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"github.com/emicklei/go-restful"
	restfulspec "github.com/emicklei/go-restful-openapi"
	"github.com/getkin/kin-openapi/openapi3"
	builderv3 "k8s.io/kube-openapi/pkg/builder3"
	"k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/validation/spec"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

// OpenAPI v3 intro https://oai.github.io/Documentation/specification.html
// OpenAPI v3 reference doc https://spec.openapis.org/oas/v3.1.0

// Swagger intro https://swagger.io/docs/specification/2-0/

// https://swagger.io/blog/news/whats-new-in-openapi-3-0/

// TODO: Change this to output the generated swagger to stdout.
const defaultSwaggerFile = "generated.v3.json"

func main() {
	// Get the name of the generated swagger file from the args
	// if it exists; otherwise use the default file name.
	swaggerFilename := defaultSwaggerFile
	if len(os.Args) > 1 {
		swaggerFilename = os.Args[1]
	}

	// Create a minimal builder config, then call the builder with the definition names.
	config := CreateOpenAPIBuilderConfig()
	config.GetDefinitions = runtimehooksv1.GetOpenAPIDefinitions
	config.GetOperationIDAndTags = getOperationIDAndTags
	// Build the Paths using a simple WebService for the final spec
	swagger, serr := builderv3.BuildOpenAPISpec(CreateWebServices(), config)
	if serr != nil {
		log.Fatalf("ERROR: %s", serr.Error())
	}

	// Marshal the swagger spec into JSON, then write it out.
	specBytes, err := json.MarshalIndent(swagger, " ", " ")
	if err != nil {
		log.Fatalf("json marshal error: %s", err.Error())
	}

	loader := openapi3.NewLoader()
	specForValidator, err := loader.LoadFromData(specBytes)

	if err != nil {
		log.Fatalf("OpenAPI v3 ref resolve error: %s", err.Error())
	}

	err = specForValidator.Validate(loader.Context)

	if err != nil {
		log.Fatalf("OpenAPI v3 validation error: %s", err.Error())
	}

	err = ioutil.WriteFile(swaggerFilename, specBytes, 0644)
	if err != nil {
		log.Fatalf("stdout write error: %s", err.Error())
	}

}

func getOperationIDAndTags(r *restful.Route) (string, []string, error) {
	if r.Metadata != nil {
		if tags, ok := r.Metadata[restfulspec.KeyOpenAPITags]; ok {
			if tagList, ok := tags.([]string); ok {
				return r.Operation, tagList, nil
			}
		}
	}
	return r.Operation, nil, nil
}

// CreateOpenAPIBuilderConfig hard-codes some values in the API builder
// config for testing.
func CreateOpenAPIBuilderConfig() *common.Config {
	return &common.Config{
		ProtocolList: []string{"https"},
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				// TODO: define how to handle many RTE
				Title:       "MyFirstRuntimeExtension",
				Description: "A fantastic Runtime Extension",
				Version:     "1.0.0",
				License: &spec.License{
					Name: "Apache 2.0",
					URL:  "http://www.apache.org/licenses/LICENSE-2.0.html",
				},
			},
		},

		// TODO: openapi schema v3 moved Responses under components; it seems that with current implementation there is no way to set them
		/*
			ResponseDefinitions: map[string]spec.Response{
				"NotFound": {
					ResponseProps: spec.ResponseProps{
						Description: "Entity not found.",
					},
				},
			},
			CommonResponses: map[int]spec.Response{
				404: *spec.ResponseRef("#/responses/NotFound"),
			},
		*/

		// TODO: the postProcess is for OpenAPi schema v2; it seems that with current implementation there is no equivalent for v3
		// TODO: BasePath could be useful to enforce versioning
		// TODO: Tags could make the spec nicer (in pet store tags seems to be used to define group for paths)
		// PostProcessSpec: postProcessSpec,
	}
}

func postProcessSpec(swo *spec.Swagger) (*spec.Swagger, error) {
	swo.BasePath = "v1" // TODO: openapi schema v3 the field has been moved under server; it seems that with current implementation there is no way to set them
	swo.Tags = []spec.Tag{
		{
			TagProps: spec.TagProps{
				Name:        "cluster-api",
				Description: "Managing users",
			},
		},
	}

	return swo, nil
}

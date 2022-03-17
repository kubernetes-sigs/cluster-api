package main

import (
	"log"
	"path/filepath"

	generatorargs "k8s.io/kube-openapi/cmd/openapi-gen/args"
	"k8s.io/kube-openapi/pkg/generators"
)

func main() {

	// Generates the code for the RegisterOpenAPIDefinitions given types.
	genericArgs, customArgs := generatorargs.NewDefaults()

	// TODO: define how to handle many RTE
	genericArgs.InputDirs = []string{
		filepath.Join("sigs.k8s.io/cluster-api", "rte/idl/test"),
	}
	genericArgs.OutputBase = "rte/idl"
	genericArgs.OutputPackagePath = "test" // TODO: define how to handle many RTE
	genericArgs.OutputFileBaseName = "zz_generated_openapi_definitions"
	// TODO: customize genericArgs.GoHeaderFilePath
	// TODO: customize genericArgs.GeneratedBuildTag
	customArgs.ReportFilename = "-" // stdout

	if err := genericArgs.Execute(
		generators.NameSystems(),
		generators.DefaultNameSystem(),
		generators.Packages,
	); err != nil {
		log.Fatalf("OpenAPI definition generation error: %v", err)
	}

}

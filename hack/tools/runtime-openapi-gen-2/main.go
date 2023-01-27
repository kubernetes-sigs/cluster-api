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

// main is the main package for openapi-gen.
package main

import (
	"fmt"
	"os"
	"path"

	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-tools/pkg/crd"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

var (
	paths      = flag.String("path", "", "Path with the variable definitions.")
	outputFile = flag.String("output-file", "zz_generated.variables.json", "Output file name.")
)

// FIXME: re-evaluate if we should still use openapi-gen in the other case
func main() {
	flag.Parse()

	if *paths == "" {
		klog.Exit("--version must be specified")
	}

	if *outputFile == "" {
		klog.Exit("--output-file must be specified")
	}

	outputFileExt := path.Ext(*outputFile)
	if outputFileExt != ".json" {
		klog.Exit("--output-file must have 'json' extension")
	}

	genName := "crd"
	//var gen genall.Generator
	gen := crd.Generator{}
	optionsRegistry := &markers.Registry{}

	// make the generator options marker itself
	defn := markers.Must(markers.MakeDefinition(genName, markers.DescribesPackage, gen))
	if err := optionsRegistry.Register(defn); err != nil {
		panic(err)
	}

	// FIXME:
	//  * compare clusterv1.JsonSchemaProps vs kubebuilder marker if something is missing
	//   * example marker
	//  * cleanup code here
	if err := genall.RegisterOptionsMarkers(optionsRegistry); err != nil {
		panic(err)
	}

	// otherwise, set up the runtime for actually running the generators
	rt, err := genall.FromOptions(optionsRegistry, []string{"paths=./api/...", "crd"})
	if err != nil {
		fmt.Println(err)
		os.Exit(0)
	}

	run(rt.GenerationContext, gen)

	// FIXME: Current state
	// * variable go type => apiextensionsv1.CustomResourceDefinition
	// * apiextensionsv1.CustomResourceDefinition => clusterv1.JsonSchemaProps
	// * Write schema as go structs to a file
	// * Validate: existing util (clusterv1.JsonSchemaProps) => validation result

	//
	//for _, currentGen := range rt.Generators {
	//	err := (*currentGen).Generate(&rt.GenerationContext)
	//	if err != nil {
	//		fmt.Println(err)
	//	}
	//}
	//
	//if hadErrs := rt.Run(); hadErrs {
	//	// don't obscure the actual error with a bunch of usage
	//	fmt.Println("err")
	//	os.Exit(0)
	//}
}

func run(ctx genall.GenerationContext, g crd.Generator) error {

	parser := &crd.Parser{
		Collector: ctx.Collector,
		Checker:   ctx.Checker,
		// Perform defaulting here to avoid ambiguity later
		IgnoreUnexportedFields: g.IgnoreUnexportedFields != nil && *g.IgnoreUnexportedFields == true,
		AllowDangerousTypes:    g.AllowDangerousTypes != nil && *g.AllowDangerousTypes == true,
		// Indicates the parser on whether to register the ObjectMeta type or not
		GenerateEmbeddedObjectMeta: g.GenerateEmbeddedObjectMeta != nil && *g.GenerateEmbeddedObjectMeta == true,
	}

	crd.AddKnownTypes(parser)
	for _, root := range ctx.Roots {
		parser.NeedPackage(root)
	}

	metav1Pkg := crd.FindMetav1(ctx.Roots)
	if metav1Pkg == nil {
		// no objects in the roots, since nothing imported metav1
		return nil
	}

	kubeKinds := crd.FindKubeKinds(parser, metav1Pkg)
	if len(kubeKinds) == 0 {
		// no objects in the roots
		return nil
	}
	for _, groupKind := range kubeKinds {
		parser.NeedCRDFor(groupKind, g.MaxDescLen)
		crdRaw := parser.CustomResourceDefinitions[groupKind]
		fmt.Printf("%#v", crdRaw)
	}

	return nil
}

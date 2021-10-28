// +build tools

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

package main

import (
	"fmt"
	"io/fs"
	"io/ioutil"

	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

type document struct {
	Openapi    string     `json:"openapi"`
	Info       info       `json:"info"`
	Servers    []server   `json:"servers"`
	Components components `json:"components"`
}

type components struct {
	Schemas map[string]*apiextensionsv1.JSONSchemaProps `json:"schemas"`
}

type info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type server struct {
	URL string `json:"url"`
}

func main() {
	inputDirs := []string{"config/crd/bases"}
	title := "Kubernetes"
	pflag.CommandLine.StringSliceVar(&inputDirs, "input-dirs", inputDirs, "input directories to convert CRDs from")
	pflag.CommandLine.String("title", title, "Title of the API")
	pflag.Parse()

	document := document{
		Openapi: "3.0.0",
		Info: info{
			Title: title,
		},
		Servers: []server{
			{
				URL: "kubernetes",
			},
		},
		Components: components{
			Schemas: map[string]*apiextensionsv1.JSONSchemaProps{},
		},
	}

	for _, dir := range inputDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			panic(err)
		}
		document.addFiles(dir, files)
	}

	newDat, err := yaml.Marshal(document)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(newDat))
}

func (d *document) addFiles(baseDir string, files []fs.FileInfo) {
	for _, file := range files {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		dat, err := ioutil.ReadFile(baseDir + "/" + file.Name()) // nolint:gosec
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(dat, crd)
		if err != nil {
			panic(err)
		}
		d.addSchema(crd)
	}
}

func (d *document) addSchema(crd *apiextensionsv1.CustomResourceDefinition) {
	singular := crd.Spec.Names.Singular
	versions := crd.Spec.Versions

	for _, ver := range versions {
		componentName := ver.Name + "." + singular
		d.Components.Schemas[componentName] = ver.Schema.OpenAPIV3Schema
	}
}

/*
Copyright 2018 The Kubernetes Authors.

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
	"flag"

	// Make sure glide gets these dependencies
	_ "github.com/go-openapi/loads"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/cmd/server"

	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable cloud provider auth

	"sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/openapi"
)

func main() {
	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})

	version := "v0"
	server.StartApiServer("/registry/k8s.io", apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions, "Api", version)
}

// +build tools

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

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
	_ "github.com/drone/envsubst/cmd/envsubst"
	_ "github.com/go-bindata/go-bindata"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/joelanford/go-apidiff"
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "github.com/raviqqe/liche"
	_ "k8s.io/code-generator/cmd/conversion-gen"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kustomize/kustomize/v3"
	_ "sigs.k8s.io/testing_frameworks/integration"
)

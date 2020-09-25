/*
Copyright 2020 The Kubernetes Authors.

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

package kubernetesversions

//go:generate sh -c "go-bindata -nometadata -pkg kubernetesversions -o zz_generated.bindata.go.tmp data && cat ../../../hack/boilerplate/boilerplate.generatego.txt zz_generated.bindata.go.tmp > zz_generated.bindata.go && rm zz_generated.bindata.go.tmp"

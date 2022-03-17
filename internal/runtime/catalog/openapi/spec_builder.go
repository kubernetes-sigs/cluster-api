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

package openapi

import (
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

type SpecBuilder struct {
	catalog *catalog.Catalog
}

func NewSpecBuilder() *SpecBuilder {
	return &SpecBuilder{}
}

func (bld *SpecBuilder) WithCatalog(c *catalog.Catalog) *SpecBuilder {
	bld.catalog = c
	return bld
}

func (bld *SpecBuilder) Build() Spec {
	if bld.catalog == nil {

	}
	return &spec{
		catalog: bld.catalog,
	}
}

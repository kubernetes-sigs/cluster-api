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

package http

import (
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

type ClientBuilder struct {
	host     string
	basePath string
	catalog  *catalog.Catalog
}

func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{}
}

func (bld *ClientBuilder) Host(host string) *ClientBuilder {
	bld.host = host
	return bld
}

func (bld *ClientBuilder) BasePath(basePath string) *ClientBuilder {
	bld.basePath = basePath
	return bld
}

func (bld *ClientBuilder) WithCatalog(c *catalog.Catalog) *ClientBuilder {
	bld.catalog = c
	return bld
}

func (bld *ClientBuilder) Build() Client {
	if bld.catalog == nil {

	}
	return &client{
		host:     bld.host,
		basePath: bld.basePath,
		catalog:  bld.catalog,
	}
}

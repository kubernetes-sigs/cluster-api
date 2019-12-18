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

package cluster

// ObjectsClient has methods to work with provider objects in the cluster.
type ObjectsClient interface {
	//TODO: add move
}

// objectsClient implements ObjectsClient.
type objectsClient struct {
	proxy Proxy
}

// ensure objectsClient implements ObjectsClient.
var _ ObjectsClient = &objectsClient{}

// newProviderObjects returns a objectsClient.
func newObjectsClient(proxy Proxy) *objectsClient {
	return &objectsClient{
		proxy: proxy,
	}
}

/*
Copyright 2023 The Kubernetes Authors.

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

/*
Package cloud implements an in memory cloud provider.

Cloud provider objects are grouped in resource groups, similarly to resource groups in Azure.

Cloud provider objects are defined like Kubernetes objects and they can be operated with
a client inspired from the controller-runtime client.

We can't use controller-runtime directly for the following reasons:
* multi-cluster (we have resourceGroups to differentiate resources belonging to different clusters)
* data should be stored in-memory
* we would like that objects in memory behave like Kubernetes objects (garbage collection)

The Manager, is the object responsible for the lifecycle of objects; it also allows
defining controllers.
*/
package cloud

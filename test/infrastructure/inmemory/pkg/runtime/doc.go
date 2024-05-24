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
Package runtime implements an in memory runtime for handling objects grouped in resource groups,
similarly to resource groups in Azure.

In memory objects are defined like Kubernetes objects and they can be operated with
a client inspired from the controller-runtime client; they also have some behaviour
of real Kubernetes objects, like e.g. a garbage collection and owner references,
as well as informers to support watches.

NOTE: We can't use controller-runtime directly for the following reasons:
* multi-cluster (we have resourceGroups to differentiate resources belonging to different clusters)
* data should be stored in-memory
* we would like that objects in memory behave like Kubernetes objects (garbage collection).
*/
package runtime

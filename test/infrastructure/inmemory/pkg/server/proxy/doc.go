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
Package proxy is a copy of sigs.k8s.io/cluster-api//controlplane/kubeadm/internal/proxy/addr.go.

It provides utilities for calling a service via a port forwarded connection, and we are using it
to API's fake port forward implementation.

TODO: Consider re-using the copied package from KCP.
*/
package proxy

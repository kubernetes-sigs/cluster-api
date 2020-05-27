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

/*
Package equality defines equality semantics for KubeadmConfigs, and utility tools for identifying
equivalent configurations.

There are a number of distinct but not different ways to express the "same" Kubeadm configuration,
and so this package attempts to sort out what differences are likely meaningful or intentional.

It is inspired by the observation that k/k no longer relies on hashing to identify "current" versions
ReplicaSets, instead using a semantic equality check that's more amenable to field modifications and
deletions: https://github.com/kubernetes/kubernetes/blob/0bb125e731/pkg/controller/deployment/util/deployment_util.go#L630-L634
*/
package equality

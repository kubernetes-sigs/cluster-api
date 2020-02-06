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

package framework

const defaultConfigYAML = `---
components:

# Load the certificate manager and wait for all of its pods and service to
# become available.
- name:    cert-manager
  sources:
  - type:  url
    value: https://github.com/jetstack/cert-manager/releases/download/v0.11.1/cert-manager.yaml
  waiters:
  - type:  service
    value: v1beta1.webhook.cert-manager.io
  - value: cert-manager

# Load CAPI core and wait for its pods to become available.
- name:    capi
  sources:
  - value: https://github.com/kubernetes-sigs/cluster-api//config?ref=master
    replacements:
    - old: "imagePullPolicy: Always"
      new: "imagePullPolicy: IfNotPresent"
  waiters:
  - value: capi-system

# Load the CAPI kubeadm bootstrapper and wait for its pods to become available.
- name:    capi-kubeadm-bootstrap
  sources:
  - value: https://github.com/kubernetes-sigs/cluster-api//bootstrap/kubeadm/config?ref=master
    replacements:
    - old: "imagePullPolicy: Always"
      new: "imagePullPolicy: IfNotPresent"
  waiters:
  - value: capi-kubeadm-bootstrap-system

# Load the CAPI kubeadm control plane and wait for its pods to become available.
- name:    capi-kubeadm-control-plane
  sources:
  - value: https://github.com/kubernetes-sigs/cluster-api//controlplane/kubeadm/config?ref=master
    replacements:
    - old: "imagePullPolicy: Always"
      new: "imagePullPolicy: IfNotPresent"
  waiters:
  - value: capi-kubeadm-control-plane-system
`

// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cluster

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Firewall contains the configuration a user expects to be applied.
type Firewall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Identifier        string         `json:"identifier,omitempty"`
	IngressRules      []*IngressRule `json:"ingressRules,omitempty"`
	EgressRules       []*EgressRule  `json:"egressRules,omitempty"`
	Name              string         `json:"name,omitempty"`
}

// Shared object infor among rules.
type Shared struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Identifier        string `json:"identifier,omitempty"`
}

// IngressRule parameters for the firewall
type IngressRule struct {
	Shared
	IngressFromPort string `json:"ingressFromPort,omitempty"`
	IngressToPort   string `json:"ingressToPort,omitempty"` //this thing should be a string.
	IngressSource   string `json:"ingressSource,omitempty"`
	IngressProtocol string `json:"ingressProtocol,omitempty"`
}

// EgressRule parameters for the firewall
type EgressRule struct {
	Shared
	EgressToPort      string `json:"engressToPort,omitempty"` //this thing should be a string.
	EgressDestination string `json:"engressDestination,omitempty"`
	EgressProtocol    string `json:"engressProtocol,omitempty"`
}

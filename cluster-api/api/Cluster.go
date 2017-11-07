/*
Copyright 2017 The Kubernetes Authors.

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

package api

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Cluster struct {
	metav1.ObjectMeta
	Spec   ClusterSpec `json:"spec"`
	//Status ClusterStatus
}


type ClusterSpec struct {
	Cloud        string `json:"cloud"` // e.g. aws, azure, gcp
	Project      string `json:"project"`
	SSH          *SSHConfig `json:"ssh"`
}

type SSHConfig struct {
	PublicKeyPath string `json:"publicKeyPath"`
	User      string `json:"user"`
}

type ControlPlaneConfig struct {
	APIServer         APIServerConfig
	ControllerManager ControllerManagerConfig
}

type APIServerConfig struct {
	AdvertiseAddress  string // e.g. "0.0.0.0"
	Port              uint32
	CertExtraSANs     string
	AuthorizationMode string
	KubernetesVersion KubernetesVersionInfo
}

type ControllerManagerConfig struct {
	LeaderElection    bool
	Port              uint32
	KubernetesVersion KubernetesVersionInfo
}

type EtcdConfig struct {
	Version string // full semver
	API     string // e.g. v2, v3
}

type KubernetesVersionInfo struct {
	SemVer   string
	Location string // e.g. https://storageapis.google.com/...
}

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

package cloudinit

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
)

// Certificates is a template struct to hold certificate data
type Certificates struct {
	CACert           string
	CAKey            string
	EtcdCACert       string
	EtcdCAKey        string
	FrontProxyCACert string
	FrontProxyCAKey  string
	SaCert           string
	SaKey            string
}

func (cpi *Certificates) validate() error {
	if !isKeyPairValid(cpi.CACert, cpi.CAKey) {
		return errors.New("CA cert material in the ControlPlaneInput is missing cert/key")
	}

	if !isKeyPairValid(cpi.EtcdCACert, cpi.EtcdCAKey) {
		return errors.New("ETCD CA cert material in the ControlPlaneInput is  missing cert/key")
	}

	if !isKeyPairValid(cpi.FrontProxyCACert, cpi.FrontProxyCAKey) {
		return errors.New("FrontProxy CA cert material in ControlPlaneInput is  missing cert/key")
	}

	if !isKeyPairValid(cpi.SaCert, cpi.SaKey) {
		return errors.New("ServiceAccount cert material in ControlPlaneInput is  missing cert/key")
	}

	return nil
}

func certificatesToFiles(input Certificates) []v1alpha2.Files {
	return []v1alpha2.Files{
		{
			Path:        "/etc/kubernetes/pki/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     input.CACert,
		},
		{
			Path:        "/etc/kubernetes/pki/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     input.CAKey,
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     input.EtcdCACert,
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     input.EtcdCAKey,
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     input.FrontProxyCACert,
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     input.FrontProxyCAKey,
		},
		{
			Path:        "/etc/kubernetes/pki/sa.pub",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     input.SaCert,
		},
		{
			Path:        "/etc/kubernetes/pki/sa.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     input.SaKey,
		},
	}
}

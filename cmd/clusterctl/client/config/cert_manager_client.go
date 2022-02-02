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

package config

import (
	"time"

	"github.com/pkg/errors"
)

const (
	// CertManagerConfigKey defines the name of the top level config key for cert-manager configuration.
	CertManagerConfigKey = "cert-manager"

	// CertManagerDefaultVersion defines the default cert-manager version to be used by clusterctl.
	CertManagerDefaultVersion = "v1.5.3"

	// CertManagerDefaultURL defines the default cert-manager repository url to be used by clusterctl.
	// NOTE: At runtime /latest will be replaced with the CertManagerDefaultVersion or with the
	// version defined by the user in the clusterctl configuration file.
	CertManagerDefaultURL = "https://github.com/cert-manager/cert-manager/releases/latest/cert-manager.yaml"

	// CertManagerDefaultTimeout defines the default cert-manager timeout to be used by clusterctl.
	CertManagerDefaultTimeout = 10 * time.Minute
)

// CertManagerClient has methods to work with cert-manager configurations.
type CertManagerClient interface {
	// Get returns the cert-manager configuration.
	Get() (CertManager, error)
}

// certManagerClient implements CertManagerClient.
type certManagerClient struct {
	reader Reader
}

// ensure certManagerClient implements CertManagerClient.
var _ CertManagerClient = &certManagerClient{}

func newCertManagerClient(reader Reader) *certManagerClient {
	return &certManagerClient{
		reader: reader,
	}
}

// configCertManager mirrors config.CertManager interface and allows serialization of the corresponding info.
type configCertManager struct {
	URL     string `json:"url,omitempty"`
	Version string `json:"version,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

func (p *certManagerClient) Get() (CertManager, error) {
	url := CertManagerDefaultURL
	version := CertManagerDefaultVersion
	timeout := CertManagerDefaultTimeout.String()

	userCertManager := &configCertManager{}
	if err := p.reader.UnmarshalKey(CertManagerConfigKey, &userCertManager); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal certManager from the clusterctl configuration file")
	}
	if userCertManager.URL != "" {
		url = userCertManager.URL
	}
	if userCertManager.Version != "" {
		version = userCertManager.Version
	}
	if userCertManager.Timeout != "" {
		timeout = userCertManager.Timeout
	}

	return NewCertManager(url, version, timeout), nil
}

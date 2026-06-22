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

// CertManager defines cert-manager configuration.
type CertManager interface {
	// URL returns the name of the cert-manager repository.
	// If empty, "https://github.com/cert-manager/cert-manager/releases/{DefaultVersion}/cert-manager.yaml" will be used.
	URL() string

	// Version returns the cert-manager version to install.
	// If empty, a default version will be used.
	Version() string

	// Timeout returns the timeout for cert-manager to start.
	// If empty, 10m will be used.
	Timeout() string

	// WaitForCertManager returns the true/false to be used when clusterctl init is initialized.
	// wait before creating cert-manager resources.
	WaitForCertManager() string
}

// certManager implements CertManager.
type certManager struct {
	url                string
	version            string
	timeout            string
	waitForCertManager string
}

// ensure certManager implements CertManager.
var _ CertManager = &certManager{}

func (p *certManager) URL() string {
	return p.url
}

func (p *certManager) Version() string {
	return p.version
}

func (p *certManager) Timeout() string {
	return p.timeout
}

func (p *certManager) WaitForCertManager() string {
	return p.waitForCertManager
}

// NewCertManager creates a new CertManager with the given configuration.
func NewCertManager(url, version, timeout, waitForCertManager string) CertManager {
	return &certManager{
		url:                url,
		version:            version,
		timeout:            timeout,
		waitForCertManager: waitForCertManager,
	}
}

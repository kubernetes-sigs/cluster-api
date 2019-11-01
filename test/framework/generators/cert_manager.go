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

package generators

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

// Generator generates the components for cert manager.
type CertManager struct {
	// ReleaseVersion defines the release version. Must be set.
	ReleaseVersion string
}

// GetName returns the name of the components being generated.
func (g *CertManager) GetName() string {
	return fmt.Sprintf("Cert Manager version %s", g.ReleaseVersion)
}

func (g *CertManager) releaseYAMLPath() string {
	return fmt.Sprintf("https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.yaml", g.ReleaseVersion)
}

// Manifests return the generated components and any error if there is one.
func (g *CertManager) Manifests() ([]byte, error) {
	resp, err := http.Get(g.releaseYAMLPath())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	return out, nil
}

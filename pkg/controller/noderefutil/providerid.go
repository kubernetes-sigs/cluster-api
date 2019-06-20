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

package noderefutil

import (
	"errors"
	"net/url"
	"path"
)

var (
	ErrEmptyProviderID   = errors.New("providerID is empty")
	ErrInvalidProviderID = errors.New("providerID is not valid")
)

// ProviderID is a struct representation of a Kubernetes ProviderID.
// Format: cloudProvider://optional/segments/etc/id
type ProviderID struct {
	value *url.URL
}

// NewProviderID parses the input string and returns a new ProviderID.
func NewProviderID(id string) (*ProviderID, error) {
	if id == "" {
		return nil, ErrEmptyProviderID
	}

	parsed, err := url.Parse(id)
	if err != nil {
		return nil, err
	}

	res := &ProviderID{
		value: parsed,
	}

	if !res.Validate() {
		return nil, ErrInvalidProviderID
	}

	return res, nil
}

// CloudProvider returns the cloud provider portion of the ProviderID.
func (p *ProviderID) CloudProvider() string {
	return p.value.Scheme
}

// ID returns the identifier portion of the ProviderID.
func (p *ProviderID) ID() string {
	return path.Base(p.value.Path)
}

// Equals returns true if both the CloudProvider and ID match.
func (p *ProviderID) Equals(o *ProviderID) bool {
	return p.CloudProvider() == o.CloudProvider() && p.ID() == o.ID()
}

// String returns the string representation of this object.
func (p *ProviderID) String() string {
	return p.value.String()
}

// Validate returns true if the provider id is valid.
func (p *ProviderID) Validate() bool {
	return p.CloudProvider() != "" &&
		p.ID() != "" &&
		p.ID() != "/" // path.Base returns "/" if consists only of slashes.
}

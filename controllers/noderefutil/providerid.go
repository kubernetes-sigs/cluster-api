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

// Package noderefutil implements NodeRef utils.
package noderefutil

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrEmptyProviderID means that the provider id is empty.
	ErrEmptyProviderID = errors.New("providerID is empty")

	// ErrInvalidProviderID means that the provider id has an invalid form.
	ErrInvalidProviderID = errors.New("providerID must be of the form <cloudProvider>://<optional>/<segments>/<provider id>")
)

// ProviderID is a struct representation of a Kubernetes ProviderID.
// Format: cloudProvider://optional/segments/etc/id
type ProviderID struct {
	original      string
	normalized    string
	cloudProvider string
	id            string
}

/*
	- must start with at least one non-colon
	- followed by ://
	- followed by any number of characters
	- must end with a non-slash
*/
var providerIDRegex = regexp.MustCompile("^[^:]+://.*[^/]$")

// NewProviderID parses the input string and returns a new ProviderID.
func NewProviderID(id string) (*ProviderID, error) {
	if id == "" {
		return nil, ErrEmptyProviderID
	}

	if !providerIDRegex.MatchString(id) {
		return nil, ErrInvalidProviderID
	}

	colonIndex := strings.Index(id, ":")
	cloudProvider := id[0:colonIndex]

	// Strip all slashes, but keep two empty slashes in the middle of the providerID
	tokens := strings.Split(id[colonIndex+1:], "/")
	cursor := 0
	// when we find the first non-empty token, this should be true so we include interspersed empty tokens
	foundValue := false
	for i := range tokens {
		token := tokens[i]
		if token != "" {
			foundValue = true
			tokens[cursor] = token
			cursor++
		} else {
			if foundValue {
				tokens[cursor] = token
				cursor++
			}
		}
	}
	tokens = tokens[:cursor]
	normalized := strings.Join(tokens, "/")

	lastSlashIndex := strings.LastIndex(id, "/")
	instance := id[lastSlashIndex+1:]

	res := &ProviderID{
		original:      id,
		cloudProvider: cloudProvider,
		id:            instance,
		normalized:    normalized,
	}

	if !res.Validate() {
		return nil, ErrInvalidProviderID
	}

	return res, nil
}

// CloudProvider returns the cloud provider portion of the ProviderID.
func (p *ProviderID) CloudProvider() string {
	return p.cloudProvider
}

// ID returns the identifier portion of the ProviderID.
func (p *ProviderID) ID() string {
	return p.id
}

// Equals returns true if both the CloudProvider and normalized suffix of the ID match.
func (p *ProviderID) Equals(o *ProviderID) bool {
	return p.CloudProvider() == o.CloudProvider() && p.Normalized() == o.Normalized()
}

// String returns the string representation of this object.
func (p *ProviderID) String() string {
	return p.original
}

// Normalized returns the normalized string representation of this object, stripping cloud provider for separate comparison.
func (p *ProviderID) Normalized() string {
	return p.normalized
}

// Validate returns true if the provider id is valid.
func (p *ProviderID) Validate() bool {
	return p.CloudProvider() != "" && p.ID() != ""
}

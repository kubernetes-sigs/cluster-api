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
// The ProviderID type is deprecated and unused by Cluster API internally.
// It will be removed entirely in a future release.
package noderefutil

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// ErrEmptyProviderID means that the provider id is empty.
	//
	// Deprecated: This var is going to be removed in a future release.
	ErrEmptyProviderID = errors.New("providerID is empty")

	// ErrInvalidProviderID means that the provider id has an invalid form.
	//
	// Deprecated: This var is going to be removed in a future release.
	ErrInvalidProviderID = errors.New("providerID must be of the form <cloudProvider>://<optional>/<segments>/<provider id>")
)

// ProviderID is a struct representation of a Kubernetes ProviderID.
// Format: cloudProvider://optional/segments/etc/id
//
// Deprecated: This struct is going to be removed in a future release.
type ProviderID struct {
	original      string
	cloudProvider string
	id            string
}

/*
- must start with at least one non-colon
- followed by ://
- followed by any number of characters
- must end with a non-slash.
*/
var providerIDRegex = regexp.MustCompile("^[^:]+://.*[^/]$")

// NewProviderID parses the input string and returns a new ProviderID.
//
// Deprecated: This constructor is going to be removed in a future release.
func NewProviderID(id string) (*ProviderID, error) {
	if id == "" {
		return nil, ErrEmptyProviderID
	}

	if !providerIDRegex.MatchString(id) {
		return nil, ErrInvalidProviderID
	}

	colonIndex := strings.Index(id, ":")
	cloudProvider := id[0:colonIndex]

	lastSlashIndex := strings.LastIndex(id, "/")
	instance := id[lastSlashIndex+1:]

	res := &ProviderID{
		original:      id,
		cloudProvider: cloudProvider,
		id:            instance,
	}

	if !res.Validate() {
		return nil, ErrInvalidProviderID
	}

	return res, nil
}

// CloudProvider returns the cloud provider portion of the ProviderID.
//
// Deprecated: This method is going to be removed in a future release.
func (p *ProviderID) CloudProvider() string {
	return p.cloudProvider
}

// ID returns the identifier portion of the ProviderID.
//
// Deprecated: This method is going to be removed in a future release.
func (p *ProviderID) ID() string {
	return p.id
}

// Equals returns true if this ProviderID string matches another ProviderID string.
//
// Deprecated: This method is going to be removed in a future release.
func (p *ProviderID) Equals(o *ProviderID) bool {
	return p.String() == o.String()
}

// String returns the string representation of this object.
//
// Deprecated: This method is going to be removed in a future release.
func (p ProviderID) String() string {
	return p.original
}

// Validate returns true if the provider id is valid.
//
// Deprecated: This method is going to be removed in a future release.
func (p *ProviderID) Validate() bool {
	return p.CloudProvider() != "" && p.ID() != ""
}

// IndexKey returns the required level of uniqueness
// to represent and index machines uniquely from their node providerID.
//
// Deprecated: This method is going to be removed in a future release.
func (p *ProviderID) IndexKey() string {
	return p.String()
}

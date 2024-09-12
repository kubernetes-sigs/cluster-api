/*
Copyright 2024 The Kubernetes Authors.

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

package apiwarnings

import (
	"fmt"
	"regexp"

	"github.com/go-logr/logr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// DomainQualifiedFinalizerWarning is a regular expression that matches a
// warning that the API server returns when a finalizer is not domain-qualified.
func DomainQualifiedFinalizerWarning(domain string) *regexp.Regexp {
	return regexp.MustCompile(
		fmt.Sprintf("^metadata.finalizers:.*%s.*prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers$", domain),
	)
}

// DefaultHandler is a handler that discards warnings that are the result of
// decisions made by the Cluster API project, but cannot be immediately
// addressed, and are therefore not helpful to the user.
func DefaultHandler(l logr.Logger) *DiscardMatchingHandler {
	return NewDiscardMatchingHandler(
		l,
		DiscardMatchingHandlerOptions{
			Deduplicate: true,
			Expressions: []regexp.Regexp{
				*DomainQualifiedFinalizerWarning(clusterv1.GroupVersion.Group),
			},
		},
	)
}

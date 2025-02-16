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
	"regexp"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// DiscardMatchingHandler is a handler that discards API server warnings
// whose message matches any user-defined regular expressions, but otherwise
// logs warnings to the user-defined logger.
type DiscardMatchingHandler struct {
	// Logger is used to log responses with the warning header.
	Logger logr.Logger

	// Expressions is a slice of regular expressions used to discard warnings.
	// If the warning message matches any expression, it is not logged.
	Expressions []regexp.Regexp
}

// HandleWarningHeader handles logging for responses from API server that are
// warnings with code being 299 and uses a logr.Logger for its logging purposes.
func (h *DiscardMatchingHandler) HandleWarningHeader(code int, _, message string) {
	if code != 299 || message == "" {
		return
	}

	for _, exp := range h.Expressions {
		if exp.MatchString(message) {
			return
		}
	}

	h.Logger.Info(message)
}

// DefaultHandler is a handler that discards warnings that are the result of
// decisions made by the Cluster API project, but cannot be immediately
// addressed, and are therefore not helpful to the user.
func DefaultHandler(l logr.Logger) *DiscardMatchingHandler {
	return &DiscardMatchingHandler{
		Logger: l,
		Expressions: []regexp.Regexp{
			DomainQualifiedFinalizerWarning(clusterv1.GroupVersion.Group),
		},
	}
}

// DiscardAllHandler is a handler that discards all warnings.
var DiscardAllHandler = rest.NoWarnings{}

// LogAllHandler is a handler that logs all warnings.
func LogAllHandler(l logr.Logger) rest.WarningHandler {
	return &DiscardMatchingHandler{Logger: l}
}

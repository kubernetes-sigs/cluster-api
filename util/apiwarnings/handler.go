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

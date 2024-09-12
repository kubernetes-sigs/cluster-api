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
	"sync"

	"github.com/go-logr/logr"
)

// DiscardMatchingHandlerOptions configures the handler created with
// NewDiscardMatchingHandler.
type DiscardMatchingHandlerOptions struct {
	// Deduplicate indicates a given warning message should only be written once.
	// Setting this to true in a long-running process handling many warnings can
	// result in increased memory use.
	Deduplicate bool

	// Expressions is a slice of regular expressions used to discard warnings.
	// If the warning message matches any expression, it is not logged.
	Expressions []regexp.Regexp
}

// NewDiscardMatchingHandler initializes and returns a new DiscardMatchingHandler.
func NewDiscardMatchingHandler(l logr.Logger, opts DiscardMatchingHandlerOptions) *DiscardMatchingHandler {
	h := &DiscardMatchingHandler{logger: l, opts: opts}
	if opts.Deduplicate {
		h.logged = map[string]struct{}{}
	}
	return h
}

// DiscardMatchingHandler is a handler that discards API server warnings
// whose message matches any user-defined regular expressions.
type DiscardMatchingHandler struct {
	// logger is used to log responses with the warning header
	logger logr.Logger
	// opts contain options controlling warning output
	opts DiscardMatchingHandlerOptions
	// loggedLock guards logged
	loggedLock sync.Mutex
	// used to keep track of already logged messages
	// and help in de-duplication.
	logged map[string]struct{}
}

// HandleWarningHeader handles logging for responses from API server that are
// warnings with code being 299 and uses a logr.Logger for its logging purposes.
func (h *DiscardMatchingHandler) HandleWarningHeader(code int, _, message string) {
	if code != 299 || message == "" {
		return
	}

	for _, exp := range h.opts.Expressions {
		if exp.MatchString(message) {
			return
		}
	}

	if h.opts.Deduplicate {
		h.loggedLock.Lock()
		defer h.loggedLock.Unlock()

		if _, alreadyLogged := h.logged[message]; alreadyLogged {
			return
		}
		h.logged[message] = struct{}{}
	}
	h.logger.Info(message)
}

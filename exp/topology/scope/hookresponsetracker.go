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

package scope

import (
	"fmt"
	"strings"
	"time"

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

// HookResponseTracker is a helper to capture the responses of the various lifecycle hooks.
type HookResponseTracker struct {
	responses map[string]runtimehooksv1.ResponseObject
}

// NewHookResponseTracker returns a new HookResponseTracker.
func NewHookResponseTracker() *HookResponseTracker {
	return &HookResponseTracker{
		responses: map[string]runtimehooksv1.ResponseObject{},
	}
}

// Add add the response of a hook to the tracker.
func (h *HookResponseTracker) Add(hook runtimecatalog.Hook, response runtimehooksv1.ResponseObject) {
	hookName := runtimecatalog.HookName(hook)
	h.responses[hookName] = response
}

// IsBlocking returns true if the hook returned a blocking response.
// If the hook is not called or did not return a blocking response it returns false.
func (h *HookResponseTracker) IsBlocking(hook runtimecatalog.Hook) bool {
	hookName := runtimecatalog.HookName(hook)
	response, ok := h.responses[hookName]
	if !ok {
		return false
	}
	retryableResponse, ok := response.(runtimehooksv1.RetryResponseObject)
	if !ok {
		// Not a retryable response. Cannot be blocking.
		return false
	}
	if retryableResponse.GetRetryAfterSeconds() == 0 {
		// Not a blocking response.
		return false
	}
	return true
}

// AggregateRetryAfter calculates the lowest non-zero retryAfterSeconds time from all the tracked responses.
func (h *HookResponseTracker) AggregateRetryAfter() time.Duration {
	res := int32(0)
	for _, resp := range h.responses {
		if retryResponse, ok := resp.(runtimehooksv1.RetryResponseObject); ok {
			res = util.LowestNonZeroInt32(res, retryResponse.GetRetryAfterSeconds())
		}
	}
	return time.Duration(res) * time.Second
}

// AggregateMessage returns a human friendly message about the blocking status of hooks.
func (h *HookResponseTracker) AggregateMessage() string {
	blockingHooks := map[string]string{}
	for hook, resp := range h.responses {
		if retryResponse, ok := resp.(runtimehooksv1.RetryResponseObject); ok {
			if retryResponse.GetRetryAfterSeconds() != 0 {
				blockingHooks[hook] = resp.GetMessage()
			}
		}
	}
	if len(blockingHooks) == 0 {
		return ""
	}

	hookAndMessages := []string{}
	for hook, message := range blockingHooks {
		if message == "" {
			hookAndMessages = append(hookAndMessages, fmt.Sprintf("hook %q is blocking", hook))
		} else {
			hookAndMessages = append(hookAndMessages, fmt.Sprintf("hook %q is blocking: %s", hook, message))
		}
	}
	return strings.Join(hookAndMessages, "; ")
}

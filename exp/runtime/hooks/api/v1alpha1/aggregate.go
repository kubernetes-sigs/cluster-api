/*
Copyright 2022 The Kubernetes Authors.

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

package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// AggregatableResponse represents a response object that can be constructed using individual
// ExtensionHandlerResponses.
// +kubebuilder:object:generate=false
type AggregatableResponse interface {
	runtime.Object

	Aggregate(items []*ExtensionHandlerResponse) error
}

// ExtensionHandlerResponse represents ths response from a single RuntimeExtensionHandler.
// +kubebuilder:object:generate=false
type ExtensionHandlerResponse struct {
	Name     string
	Response runtime.Object
}

// aggregateExtensionResponses combines the individual ExtensionHandlerResponses into a single hook response
// object. This function currently assumes that all the ExtensionHandlerResponses are success responses.
func aggregateExtensionResponses(response runtime.Object, responses []*ExtensionHandlerResponse) error {
	msg := &strings.Builder{}
	status := ResponseStatusSuccess

	for _, item := range responses {
		m, err := GetMessage(item.Response)
		if err != nil {
			return err
		}
		msg.WriteString(fmt.Sprintf("%s: %s. ", item.Name, m))
	}

	if err := SetMessage(response, msg.String()); err != nil {
		return errors.Wrap(err, "failed to set aggregate message")
	}
	if err := SetStatus(response, status); err != nil {
		return errors.Wrap(err, "failed to set aggregate status")
	}
	return nil
}

// aggregateRetryAfterSeconds calculates the effective retryAfterSeconds by using the lowest non-zero
// value among all the ExtensionHandlerResponses.
func aggregateRetryAfterSeconds(response runtime.Object, responses []*ExtensionHandlerResponse) error {
	for _, item := range responses {
		retry, err := GetRetryAfterSeconds(item.Response)
		if err != nil {
			return errors.Wrap(err, "failed to get retryAfterSeconds from aggregate response")
		}
		current, err := GetRetryAfterSeconds(response)
		if err != nil {
			return errors.Wrap(err, "failed to get retryAfterSeconds from extension response")
		}
		if err := SetRetryAfterSeconds(
			response,
			lowestNonZeroRetryAfterSeconds(current, retry),
		); err != nil {
			return errors.Wrap(err, "failed to set aggregate retryAfterSeconds")
		}
	}
	return nil
}

func lowestNonZeroRetryAfterSeconds(i, j int) int {
	if i == 0 {
		return j
	}
	if j == 0 {
		return i
	}
	if i < j {
		return i
	}
	return j
}

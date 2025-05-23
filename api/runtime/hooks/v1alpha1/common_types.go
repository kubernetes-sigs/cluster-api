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
	"k8s.io/apimachinery/pkg/runtime"
)

// RequestObject is a runtime.Object extended with methods to handle request-specific fields.
// +kubebuilder:object:generate=false
type RequestObject interface {
	runtime.Object
	GetSettings() map[string]string
	SetSettings(settings map[string]string)
}

// CommonRequest is the data structure common to all request types.
// Note: By embedding CommonRequest in a runtime.Object the RequestObject
// interface is satisfied.
type CommonRequest struct {
	// settings defines key value pairs to be passed to the call.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// GetSettings get the Settings field from the CommonRequest.
func (r *CommonRequest) GetSettings() map[string]string {
	return r.Settings
}

// SetSettings sets the Settings field in the CommonRequest.
func (r *CommonRequest) SetSettings(settings map[string]string) {
	r.Settings = settings
}

// ResponseObject is a runtime.Object extended with methods to handle response-specific fields.
// +kubebuilder:object:generate=false
type ResponseObject interface {
	runtime.Object
	GetMessage() string
	GetStatus() ResponseStatus
	SetMessage(message string)
	SetStatus(status ResponseStatus)
}

// RetryResponseObject is a ResponseObject which additionally defines the functionality
// for a response to signal a retry.
// +kubebuilder:object:generate=false
type RetryResponseObject interface {
	ResponseObject
	GetRetryAfterSeconds() int32
	SetRetryAfterSeconds(retryAfterSeconds int32)
}

// CommonResponse is the data structure common to all response types.
// Note: By embedding CommonResponse in a runtime.Object the ResponseObject
// interface is satisfied.
type CommonResponse struct {
	// status of the call. One of "Success" or "Failure".
	// +required
	Status ResponseStatus `json:"status"`

	// message is a human-readable description of the status of the call.
	// +optional
	Message string `json:"message,omitempty"`
}

// SetMessage sets the Message field for the CommonResponse.
func (r *CommonResponse) SetMessage(message string) {
	r.Message = message
}

// SetStatus sets the Status field for the CommonResponse.
func (r *CommonResponse) SetStatus(status ResponseStatus) {
	r.Status = status
}

// GetMessage returns the Message field for the CommonResponse.
func (r *CommonResponse) GetMessage() string {
	return r.Message
}

// GetStatus returns the Status field for the CommonResponse.
func (r *CommonResponse) GetStatus() ResponseStatus {
	return r.Status
}

// ResponseStatus represents the status of the hook response.
// +enum
type ResponseStatus string

const (
	// ResponseStatusSuccess represents a success response.
	ResponseStatusSuccess ResponseStatus = "Success"

	// ResponseStatusFailure represents a failure response.
	ResponseStatusFailure ResponseStatus = "Failure"
)

// CommonRetryResponse is the data structure which contains all
// common and retry fields.
// Note: By embedding CommonRetryResponse in a runtime.Object the RetryResponseObject
// interface is satisfied.
type CommonRetryResponse struct {
	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// retryAfterSeconds when set to a non-zero value signifies that the hook
	// will be called again at a future time.
	// +required
	RetryAfterSeconds int32 `json:"retryAfterSeconds"`
}

// GetRetryAfterSeconds returns the RetryAfterSeconds field for the CommonRetryResponse.
func (r *CommonRetryResponse) GetRetryAfterSeconds() int32 {
	return r.RetryAfterSeconds
}

// SetRetryAfterSeconds sets the RetryAfterSeconds field for the CommonRetryResponse.
func (r *CommonRetryResponse) SetRetryAfterSeconds(retryAfterSeconds int32) {
	r.RetryAfterSeconds = retryAfterSeconds
}

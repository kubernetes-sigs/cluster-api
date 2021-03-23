/*
Copyright 2020 The Kubernetes Authors.

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

package external

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ReconcileOutput is a return type of the external reconciliation
// of referenced objects.
type ReconcileOutput struct {
	// RequeueAfter if greater than 0, tells the Controller to requeue the reconcile key after the Duration.
	// Implies that Requeue is true, there is no need to set Requeue to true at the same time as RequeueAfter.
	//
	// TODO(vincepri): Remove this field here and try to return a better struct that embeds ctrl.Result,
	// we can't do that today because the field would conflict with the current `Result` field,
	// which should probably be renamed to `Object` or something similar.
	RequeueAfter time.Duration
	// Details of the referenced external object.
	// +optional
	Result *unstructured.Unstructured
	// Indicates if the external object is paused.
	// +optional
	Paused bool
}

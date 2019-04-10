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

package machine

import (
	"errors"
	controllerError "github.com/openshift/cluster-api/pkg/controller/error"
	"testing"
	"time"
)

func mockDrainNode(shouldDelay bool) error {
	if shouldDelay {
		return &controllerError.RequeueAfterError{RequeueAfter: 20 * time.Second}
	}
	return errors.New("other error")
}

func TestDelayIfRequeueAfterError(t *testing.T) {
	tCases := []struct {
		shouldDelay bool
		expectedErr bool
	}{
		{
			shouldDelay: true,
			expectedErr: false,
		},
		{
			shouldDelay: false,
			expectedErr: true,
		},
	}
	for i, tc := range tCases {
		_, err := delayIfRequeueAfterError(mockDrainNode(tc.shouldDelay))
		if err != nil && tc.expectedErr == false {
			t.Fatalf("case %v received unexpected error type", i)
		}
	}
}

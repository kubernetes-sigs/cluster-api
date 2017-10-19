// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package googleSDK

import (
	"os"
	"testing"
)

func TestSdkHappy(t *testing.T) {
	err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "../../../test/resources/google_application_credentials_example.json")
	if err != nil {
		t.Fatalf("Unable to set env var: %v", err)
	}

	_, err = NewSdk()
	if err != nil {
		t.Fatalf("Unable to get Google compute SDK: %v", err)
	}
}

func TestSdkSad(t *testing.T) {
	err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "file_that_does_not_exist.json")
	if err != nil {
		t.Fatalf("Unable to set env var: %v", err)
	}

	_, err = NewSdk()
	if err == nil {
		t.Fatalf("Able to get Google compute SDK with empty variables")
	}
}

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
package fileresource

import (
	"testing"
)

func TestGithubUrl(t *testing.T) {

	testData := map[string]string{
		"bootstrap/myscript.sh": "https://raw.githubusercontent.com/kris-nova/kubicorn/master/bootstrap/myscript.sh",
	}

	for input, expectedOutput := range testData {
		actualOutput := getGitHubUrl(input)
		if expectedOutput != actualOutput {
			t.Errorf("Invalid GitHub URL actual [%s] expected [%s]", actualOutput, expectedOutput)
		}
	}
}

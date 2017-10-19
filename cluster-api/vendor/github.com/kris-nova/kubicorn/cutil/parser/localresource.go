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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// readFromFS reads file from a local path and returns as string
func readFromFS(sourcePath string) (string, error) {

	// If sourcePath starts with ~ we search for $HOME
	// and preppend it to the absolutePath overwriting the first character
	// TODO: Add Windows support
	if strings.HasPrefix(sourcePath, "~") {
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			return "", fmt.Errorf("Could not find $HOME")
		}
		sourcePath = filepath.Join(homeDir, sourcePath[1:])
	}

	bytes, err := ioutil.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

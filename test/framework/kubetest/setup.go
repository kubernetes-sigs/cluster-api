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

package kubetest

import (
	"io"
	"os"
	"path"
	"path/filepath"
)

func copyFile(srcFilePath, destFilePath string) error {
	if err := os.MkdirAll(path.Dir(destFilePath), 0o750); err != nil {
		return err
	}
	srcFile, err := os.Open(filepath.Clean(srcFilePath))
	if err != nil {
		return err
	}
	destFile, err := os.Create(destFilePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}
	return nil
}

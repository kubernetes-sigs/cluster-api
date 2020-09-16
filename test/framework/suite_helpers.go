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

package framework

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

func GatherJUnitReports(srcDir string, destDir string) error {
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		return err
	}
	return filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() && p != srcDir {
			return filepath.SkipDir
		}
		if filepath.Ext(p) != ".xml" {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, "junit") {
			newName := strings.ReplaceAll(base, "_", ".")
			if err := os.Rename(p, path.Join(destDir, newName)); err != nil {
				return err
			}
		}
		return nil
	})
}

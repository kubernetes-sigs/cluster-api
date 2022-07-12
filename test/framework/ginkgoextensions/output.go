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

// Package ginkgoextensions extends ginkgo.
package ginkgoextensions

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/pkg/errors"
)

// TestOutput can be used for writing testing output.
var TestOutput = ginkgo.GinkgoWriter

// Byf provides formatted output to the GinkgoWriter.
func Byf(format string, a ...interface{}) {
	ginkgo.By(fmt.Sprintf(format, a...))
}

// EnableFileLogging enables additional file logging.
// Logs are written to the given path with timestamps.
func EnableFileLogging(path string) (io.WriteCloser, error) {
	w, err := newFileWriter(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create fileWriter")
	}

	ginkgo.GinkgoWriter.TeeTo(w)

	return w, nil
}

func newFileWriter(path string) (io.WriteCloser, error) {
	f, err := os.Create(path) //nolint:gosec // No security issue: path is safe.
	if err != nil {
		return nil, errors.Wrap(err, "failed to create file")
	}
	return &fileWriter{
		file: f,
	}, nil
}

type fileWriter struct {
	file *os.File
}

func (w *fileWriter) Write(data []byte) (n int, err error) {
	return w.file.WriteString("[" + time.Now().Format(time.RFC3339) + "] " + string(data))
}

func (w *fileWriter) Close() error {
	return w.file.Close()
}

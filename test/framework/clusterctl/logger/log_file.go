// +build e2e

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

package logger

import (
	"bufio"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
)

// Provides a log_file that can be used to trace e2e test actions.

type LogFileMeta struct {
	LogPath string
	Name    string
}

func CreateLogFile(input LogFileMeta) *LogFile {
	filePath := filepath.Join(input.LogPath, input.Name)
	Expect(os.MkdirAll(filepath.Dir(filePath), 0755)).To(Succeed(), "Failed to create log folder %s", filepath.Dir(filePath))

	f, err := os.Create(filePath)
	Expect(err).ToNot(HaveOccurred(), "Failed to create log file %s", filePath)

	return &LogFile{
		name:   input.Name,
		file:   f,
		Writer: bufio.NewWriter(f),
	}
}

type LogFile struct {
	name string
	file *os.File
	*bufio.Writer
}

func (f *LogFile) Name() string {
	return f.name
}

func (f *LogFile) Flush() {
	Expect(f.Writer.Flush()).To(Succeed(), "Failed to flush log %s", f.name)
}

func (f *LogFile) Close() {
	f.Flush()
	Expect(f.file.Close()).To(Succeed(), "Failed to close log %s", f.name)
}

func (f *LogFile) Logger() logr.Logger {
	return &logger{writer: f}
}

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

package cloudinit

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
)

// writeFilesAction defines a cloud init action that replicates on kund the write_files module
type writeFilesAction struct {
	Files []files `json:"write_files,"`
}

type files struct {
	Path        string `json:"path,"`
	Encoding    string `json:"encoding,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Permissions string `json:"permissions,omitempty"`
	Content     string `json:"content,"`
	Append      bool   `json:"append,"`
}

var _ cloudCongfigAction = &writeFilesAction{}

func newWriteFilesAction() cloudCongfigAction {
	return &writeFilesAction{}
}

func (a *writeFilesAction) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(errors.WithStack(err), "error parsing write_files action: %s", userData)
	}
	return nil
}

func (a *writeFilesAction) Run(cmder exec.Cmder) ([]string, error) {
	var lines []string
	for _, f := range a.Files {
		// Fix attributes and apply defaults
		path := fixPath(f.Path) //NB. the real cloud init module for writes files converts path into absolute paths; this is not possible here...
		encodings := fixEncoding(f.Encoding)
		owner := fixOwner(f.Owner)
		permissions := fixPermissions(f.Permissions)
		content, err := fixContent(f.Content, encodings)
		if err != nil {
			return lines, errors.Wrapf(err, "error decoding content for %s", path)
		}

		// Writing the files
		// Add a line in the output that mimics the command being issues at the command line
		redirects := ">"
		if f.Append {
			redirects = ">>"
		}
		lines = append(lines, fmt.Sprintf("%s cat %s %s << END\n%s\nEND\n", prompt, redirects, path, content))

		if err := cmder.Command(
			"cat", redirects, path, "/dev/stdin",
		).SetStdin(
			strings.NewReader(content),
		).Run(); err != nil {
			// Add a line in the output with the error message and exit
			lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
			return lines, errors.Wrapf(errors.WithStack(err), "error writing file content to %s", path)
		}

		// if permissions is different by default ownership in kind, sets file permissions
		if permissions != "0644" {
			// Add a line in the output that mimics the command being issues at the command line
			lines = append(lines, fmt.Sprintf("%s chmod %s %s", prompt, permissions, path))
			if err := cmder.Command(
				"chmod", permissions, path,
			).Run(); err != nil {
				// Add a line in the output with the error message and exit
				lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
				return lines, errors.Wrapf(errors.WithStack(err), "error setting permissions for %s", path)
			}
		}

		// if ownership is different by default ownership in kind, sets file ownership
		if owner != "root:root" {
			// Add a line in the output that mimics the command being issues at the command line
			lines = append(lines, fmt.Sprintf("%s chown %s %s", prompt, owner, path))
			if err := cmder.Command(
				"chown", owner, path,
			).Run(); err != nil {
				// Add a line in the output with the error message and exit
				lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
				return lines, errors.Wrapf(errors.WithStack(err), "error setting ownership for %s", path)
			}
		}
	}
	return lines, nil
}

func fixPath(p string) string {
	return strings.TrimSpace(p)
}

func fixOwner(o string) string {
	o = strings.TrimSpace(o)
	if o != "" {
		return o
	}
	return "root:root"
}

func fixPermissions(p string) string {
	p = strings.TrimSpace(p)
	if p != "" {
		return p
	}
	return "0644"
}

func fixEncoding(e string) []string {
	e = strings.ToLower(e)
	e = strings.TrimSpace(e)

	if e == "gz" || e == "gzip" {
		return []string{"application/x-gzip"}
	} else if e == "gz+base64" || e == "gzip+base64" || e == "gz+b64" || e == "gzip+b64" {
		return []string{"application/base64", "application/x-gzip"}
	} else if e == "base64" || e == "b64" {
		return []string{"application/base64"}
	}

	return []string{"text/plain"}
}

func fixContent(content string, encodings []string) (string, error) {
	var contentString = content
	for _, e := range encodings {
		switch e {
		case "application/base64":
			rByte, err := base64.StdEncoding.DecodeString(contentString)
			if err != nil {
				return contentString, errors.WithStack(err)
			}
			contentString = string(rByte)
		case "application/x-gzip":
			rByte, err := gUnzipData([]byte(contentString))
			if err != nil {
				return contentString, err
			}
			contentString = string(rByte)
		case "text/plain":
			// NOP
		}
	}
	return contentString, nil
}

func gUnzipData(data []byte) ([]byte, error) {
	var r io.Reader
	var err error
	b := bytes.NewBuffer(data)
	r, err = gzip.NewReader(b)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resB.Bytes(), nil
}

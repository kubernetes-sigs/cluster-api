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
	"path/filepath"
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

func newWriteFilesAction() action {
	return &writeFilesAction{}
}

func (a *writeFilesAction) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing write_files action: %s", userData)
	}
	return nil
}

func (a *writeFilesAction) Run(cmder exec.Cmder) ([]string, error) {
	var lines []string //nolint:prealloc
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

		// Make the directory so cat + redirection will work
		directory := filepath.Dir(path)
		lines = append(lines, fmt.Sprintf("%s mkdir -p %s\n", prompt, directory))
		if err := cmder.Command("/bin/sh", "-c", fmt.Sprintf("mkdir -p %q", directory)).Run(); err != nil {
			return lines, errors.Wrapf(err, fmt.Sprintf("failed to create directory"))
		}

		redirects := ">"
		if f.Append {
			redirects = ">>"
		}

		// Add a line in the output that mimics the command being issues at the command line
		lines = append(lines, fmt.Sprintf("%s cat %s %s << END\n%s\nEND\n", prompt, redirects, path, content))
		if err := cmder.Command("/bin/sh", "-c", fmt.Sprintf("cat %s %s /dev/stdin", redirects, path)).
			SetStdin(strings.NewReader(content)).
			Run(); err != nil {
			// TODO Consider returning stdout or stderr or both instead of lines or in addition to lines
			// Add a line in the output with the error message and exit
			lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
			return lines, errors.Wrapf(err, "error writing file content to %s", path)
		}

		// if permissions is different by default ownership in kind, sets file permissions
		if permissions != "0644" {
			// Add a line in the output that mimics the command being issues at the command line
			lines = append(lines, fmt.Sprintf("%s chmod %s %s", prompt, permissions, path))
			if err := cmder.Command("chmod", permissions, path).Run(); err != nil {
				// Add a line in the output with the error message and exit
				lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
				return lines, errors.Wrapf(errors.WithStack(err), "error setting permissions for %s", path)
			}
		}

		// if ownership is different by default ownership in kind, sets file ownership
		if owner != "root:root" {
			// Add a line in the output that mimics the command being issues at the command line
			lines = append(lines, fmt.Sprintf("%s chown %s %s", prompt, owner, path))
			if err := cmder.Command("chown", owner, path).Run(); err != nil {
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

	switch e {
	case "gz", "gzip":
		return []string{"application/x-gzip"}
	case "gz+base64", "gzip+base64", "gz+b64", "gzip+b64":
		return []string{"application/base64", "application/x-gzip"}
	case "base64", "b64":
		return []string{"application/base64"}
	}

	return []string{"text/plain"}
}

func fixContent(content string, encodings []string) (string, error) {
	for _, e := range encodings {
		switch e {
		case "application/base64":
			rByte, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				return content, errors.WithStack(err)
			}
			return string(rByte), nil
		case "application/x-gzip":
			rByte, err := gUnzipData([]byte(content))
			if err != nil {
				return content, err
			}
			return string(rByte), nil
		case "text/plain":
			return content, nil
		default:
			return content, errors.Errorf("Unknown bootstrap data encoding: %q", content)
		}
	}
	return content, nil
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

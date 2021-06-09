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

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

const (
	kubeadmInitPath          = "/run/kubeadm/kubeadm.yaml"
	kubeproxyComponentConfig = `
---
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
conntrack:
# Skip setting sysctl value "net.netfilter.nf_conntrack_max"
# It is a global variable that affects other namespaces
  maxPerCore: 0
`
)

// writeFilesAction defines a list of files that should be written to a node.
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

// Commands return a list of commands to run on the node.
// Each command defines the parameters of a shell command necessary to generate a file replicating the cloud-init write_files module.
func (a *writeFilesAction) Commands() ([]Cmd, error) {
	commands := make([]Cmd, 0)
	for _, f := range a.Files {
		// Fix attributes and apply defaults
		path := fixPath(f.Path) // NB. the real cloud init module for writes files converts path into absolute paths; this is not possible here...
		encodings := fixEncoding(f.Encoding)
		owner := fixOwner(f.Owner)
		permissions := fixPermissions(f.Permissions)
		content, err := fixContent(f.Content, encodings)
		if path == kubeadmInitPath {
			content += kubeproxyComponentConfig
		}
		if err != nil {
			return commands, errors.Wrapf(err, "error decoding content for %s", path)
		}

		// Make the directory so cat + redirection will work
		directory := filepath.Dir(path)
		commands = append(commands, Cmd{Cmd: "mkdir", Args: []string{"-p", directory}})

		redirects := ">"
		if f.Append {
			redirects = ">>"
		}

		// generate a command that will create a file with the expected contents.
		commands = append(commands, Cmd{Cmd: "/bin/sh", Args: []string{"-c", fmt.Sprintf("cat %s %s /dev/stdin", redirects, path)}, Stdin: content})

		// if permissions are different than default ownership, add a command to modify the permissions.
		if permissions != "0644" {
			commands = append(commands, Cmd{Cmd: "chmod", Args: []string{permissions, path}})
		}

		// if ownership is different than default ownership, add a command to modify file ownerhsip.
		if owner != "root:root" {
			commands = append(commands, Cmd{Cmd: "chown", Args: []string{owner, path}})
		}
	}
	return commands, nil
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

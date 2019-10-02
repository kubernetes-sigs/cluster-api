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
	"fmt"
	"reflect"
	"testing"
)

func TestWriteFiles(t *testing.T) {
	var useCases = []struct {
		name          string
		w             writeFilesAction
		expectedlines []string
		expectedError bool
	}{
		{
			name: "two files pass",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar"},
					{Path: "baz", Content: "qux"},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > baz << END\nqux\nEND\n", prompt),
			},
		},
		{
			name: "first file fails",
			w: writeFilesAction{
				Files: []files{
					{Path: "fail", Content: "bar"}, // fail force fakeCmd to fail
					{Path: "baz", Content: "qux"},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > fail << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
				// there should not be a second file!
			},
			expectedError: true,
		},
		{
			name: "second file fails",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar"},
					{Path: "fail", Content: "qux"}, // fail force fakeCmd to fail
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > fail << END\nqux\nEND\n", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
			},
			expectedError: true,
		},
		{
			name: "owner different than default",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar", Owner: "baz:baz"},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s chown baz:baz foo", prompt),
			},
		},
		{
			name: "chown fail",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar", Owner: "fail:fail"}, // fail force fakeCmd to fail
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s chown fail:fail foo", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
			},
			expectedError: true,
		},
		{
			name: "permissions different than default",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar", Permissions: "755"},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s chmod 755 foo", prompt),
			},
		},
		{
			name: "chmod fail",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar", Permissions: "fail"}, // fail force fakeCmd to fail
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat > foo << END\nbar\nEND\n", prompt),
				fmt.Sprintf("%s chmod fail foo", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
			},
			expectedError: true,
		},
		{
			name: "append",
			w: writeFilesAction{
				Files: []files{
					{Path: "foo", Content: "bar", Append: true},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s mkdir -p .\n", prompt),
				fmt.Sprintf("%s cat >> foo << END\nbar\nEND\n", prompt),
			},
		},
	}

	for _, rt := range useCases {
		t.Run(rt.name, func(t *testing.T) {

			cmder := fakeCmder{t: t}
			lines, err := rt.w.Run(cmder)

			if err == nil && rt.expectedError {
				t.Error("Expected error, got nil")
			}
			if err != nil && !rt.expectedError {
				t.Errorf("Expected nil, got error %v", err)
			}

			if !reflect.DeepEqual(rt.expectedlines, lines) {
				t.Errorf("Expected %s, got %s", rt.expectedlines, lines)
			}
		})
	}
}

func TestFixContent(t *testing.T) {
	v := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	gv, _ := gZipData([]byte(v))
	var useCases = []struct {
		name            string
		content         string
		encoding        string
		expectedContent string
		expectedError   bool
	}{
		{
			name:            "plain text",
			content:         "foobar",
			expectedContent: "foobar",
		},
		{
			name:            "base64 data",
			content:         "YWJjMTIzIT8kKiYoKSctPUB+",
			encoding:        "base64",
			expectedContent: "abc123!?$*&()'-=@~",
		},
		{
			name:            "gzip data",
			content:         string(gv),
			encoding:        "gzip",
			expectedContent: v,
		},
	}

	for _, rt := range useCases {
		t.Run(rt.name, func(t *testing.T) {
			encoding := fixEncoding(rt.encoding)
			c, err := fixContent(rt.content, encoding)

			if err == nil && rt.expectedError {
				t.Error("Expected error, got nil")
			}
			if err != nil && !rt.expectedError {
				t.Errorf("Expected nil, got error %v", err)
			}

			if rt.expectedContent != c {
				t.Errorf("Expected %s, got %s", rt.expectedContent, c)
			}
		})
	}
}

func TestUnzipData(t *testing.T) {
	value := []byte("foobarbazquxfoobarbazquxfoobarbazquxfoobarbazquxfoobarbazquxfoobarbazquxfoobarbazquxfoobarbazquxfoobarbazqux")
	gvalue, _ := gZipData(value)
	dvalue, _ := gUnzipData(gvalue)
	if !bytes.Equal(value, dvalue) {
		t.Errorf("ss")
	}
}

func gZipData(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	if _, err := gz.Write(data); err != nil {
		return nil, err
	}

	if err := gz.Flush(); err != nil {
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

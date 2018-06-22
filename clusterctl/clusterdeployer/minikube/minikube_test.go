/*
Copyright 2018 The Kubernetes Authors.

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

package minikube

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestCreate(t *testing.T) {
	var testcases = []struct {
		name      string
		execError error
		expectErr bool
	}{
		{
			name: "success",
		},
		{
			name:      "exec fail",
			execError: fmt.Errorf("test error"),
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			m := New("")
			m.minikubeExec = func(env []string, args ...string) (string, error) {
				return "", testcase.execError
			}
			err := m.Create()
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	var testcases = []struct {
		name      string
		execError error
		expectErr bool
	}{
		{
			name: "success",
		},
		{
			name:      "exec fail",
			execError: fmt.Errorf("test error"),
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			m := New("")
			m.minikubeExec = func(env []string, args ...string) (string, error) {
				return "", testcase.execError
			}
			err := m.Delete()
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
		})
	}
}

func TestGetKubeconfig(t *testing.T) {
	const contents = "dfserfafaew"
	m := New("")
	f, err := createTempFile(contents)
	if err != nil {
		t.Fatal("Unable to create test file.")
	}
	defer os.Remove(f)
	t.Run("file does not exist", func(t *testing.T) {
		c, err := m.GetKubeconfig()
		if err == nil {
			t.Fatal("Able to read a file that does not exist")
		}
		if c != "" {
			t.Fatal("Able to return contents for file that does not exist.")
		}
	})
	t.Run("file exists", func(t *testing.T) {
		m.kubeconfigpath = f
		c, err := m.GetKubeconfig()
		if err != nil {
			t.Fatalf("Unexpected err. Got: %v", err)
			return
		}
		if c != contents {
			t.Fatalf("Unexpected contents. Got: %v, Want: %v", c, contents)
		}
	})
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()
	f.WriteString(contents)
	return f.Name(), nil
}

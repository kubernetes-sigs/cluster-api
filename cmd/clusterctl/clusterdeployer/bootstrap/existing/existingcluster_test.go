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

package existing

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestGetKubeconfig(t *testing.T) {
	const contents = "dfserfafaew"
	f, err := createTempFile(contents)
	if err != nil {
		t.Fatal("Unable to create test file.")
	}
	defer os.Remove(f)

	t.Run("invalid path given", func(t *testing.T) {
		_, err = NewExistingCluster("")
		if err == nil {
			t.Fatal("Should not be able create External Cluster Provisioner.")
		}
	})

	t.Run("file exists", func(t *testing.T) {

		ec, err := NewExistingCluster(f)
		if err != nil {
			t.Fatal("Should be able create External Cluster Provisioner.")
		}

		c, err := ec.GetKubeconfig()
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

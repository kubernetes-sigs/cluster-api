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

package objects

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/kubectl/scheme"
)

// DecodeCAPIObjects takes YAML and exports runtime.Objects to be submitted to an APIserver
func DecodeCAPIObjects(yaml io.Reader) ([]runtime.Object, error) {
	decoder := scheme.Codecs.UniversalDeserializer()
	objects := []runtime.Object{}
	readbuf := bufio.NewReader(yaml)
	writebuf := &bytes.Buffer{}

	for {
		line, err := readbuf.ReadBytes('\n')
		// End of an object, parse it
		if err == io.EOF || bytes.Equal(line, []byte("---\n")) {

			// Use unstructured because scheme may not know about CRDs
			if writebuf.Len() > 1 {
				obj, _, err := decoder.Decode(writebuf.Bytes(), nil, &unstructured.Unstructured{})
				if err == nil {
					objects = append(objects, obj)
				} else {
					return []runtime.Object{}, errors.Wrap(err, "couldn't decode CAPI object")
				}
			}

			// previously we didn't care if this was EOF or ---, but now we need to break the loop
			if err == io.EOF {
				break
			}

			// No matter what happened, start over
			writebuf.Reset()
		} else if err != nil {
			return []runtime.Object{}, errors.Wrap(err, "couldn't read YAML")
		} else {
			// Just an ordinary line
			writebuf.Write(line)
		}
	}

	return objects, nil
}

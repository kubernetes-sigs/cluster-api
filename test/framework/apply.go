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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HTTPGetter wraps up the Get method exposed by the net/http.Client.
type HTTPGetter interface {
	Get(url string) (resp *http.Response, err error)
}

// ApplyYAMLURLInput is the input for ApplyYAMLURL.
type ApplyYAMLURLInput struct {
	Client        client.Client
	HTTPGetter    HTTPGetter
	NetworkingURL string
	Scheme        *runtime.Scheme
}

// ApplyYAMLURL is essentially kubectl apply -f <url>.
// If the YAML in the URL contains Kinds not registered with the scheme this will fail.
func ApplyYAMLURL(ctx context.Context, input ApplyYAMLURLInput) {
	By(fmt.Sprintf("Applying networking from %s", input.NetworkingURL))
	resp, err := input.HTTPGetter.Get(input.NetworkingURL)
	Expect(err).ToNot(HaveOccurred())
	yamls, err := ioutil.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()
	yamlFiles := bytes.Split(yamls, []byte("---"))
	codecs := serializer.NewCodecFactory(input.Scheme)
	for _, f := range yamlFiles {
		f = bytes.TrimSpace(f)
		if len(f) == 0 {
			continue
		}
		decode := codecs.UniversalDeserializer().Decode
		obj, _, err := decode(f, nil, nil)
		if runtime.IsMissingKind(err) {
			continue
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(input.Client.Create(ctx, obj)).To(Succeed())
	}
}

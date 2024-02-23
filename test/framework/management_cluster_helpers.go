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
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

// WaitForProviderCAInjection tries to query a list of all CRD objects for a provider
// to ensure the webhooks are correctly setup, especially that cert-manager did inject
// the up-to-date CAs into the relevant objects.
func WaitForProviderCAInjection(ctx context.Context, lister Lister, outpath string) {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	Eventually(func() error {
		return lister.List(ctx, crdList, client.HasLabels{clusterv1.ProviderNameLabel})
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get crds of providers")

	outDir := outpath
	Expect(os.MkdirAll(outDir, 0750)).To(Succeed())

	for i := range crdList.Items {
		crd := crdList.Items[i]
		data, err := yaml.Marshal(crd)
		Expect(err).ToNot(HaveOccurred())
		outFile := filepath.Join(outDir, crd.Name+".yaml")
		log.Logf("Writing crd to %s", outpath)
		Expect(os.WriteFile(outFile, data, 0600)).To(Succeed())
		// Use all versions so we also test conversion webhooks
		for _, version := range crd.Spec.Versions {
			if !version.Served {
				continue
			}
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: version.Name,
				Kind:    crd.Spec.Names.Kind,
			}
			log.Logf("Checking crd %s - %s", gvk, time.Now())
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(gvk)
			Eventually(func() error {
				return lister.List(ctx, list)
			}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get objects for crd")
		}
	}
}

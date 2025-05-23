/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// ValidateCRDMigrationCRDFilterFunction allows filtering the CRDs that are validated.
// The function should return true for CRDs that should be validated.
type ValidateCRDMigrationCRDFilterFunction func(crd apiextensionsv1.CustomResourceDefinition) bool

// ValidateCRDMigration validates that CRD Migration is completed.
func ValidateCRDMigration(ctx context.Context, proxy ClusterProxy, namespace, _ string, crdFilterFunction ValidateCRDMigrationCRDFilterFunction, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	By("Validate CRD Migration")

	servedGroupVersionsByGK := map[schema.GroupKind]sets.Set[string]{}

	By("Validate CRDs")
	Eventually(func(g Gomega) {
		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		g.Expect(proxy.GetClient().List(ctx, crds)).To(Succeed())

		for _, crd := range crds.Items {
			if !crdFilterFunction(crd) {
				continue
			}

			// Validate the CRD has been reconciled by the CRD migrator
			g.Expect(crd.Annotations).To(HaveKeyWithValue(clusterv1.CRDMigrationObservedGenerationAnnotation, strconv.FormatInt(crd.GetGeneration(), 10)), fmt.Sprintf("CustomResourceDefinition %s should have observed generation annotation with correct value", crd.Name))

			// Validate .status.storedVersions == [storageVersion]
			g.Expect(crd.Status.StoredVersions).To(HaveLen(1), fmt.Sprintf("CustomResourceDefinition %s should have only one entry in .status.storedVersions", crd.Name))
			g.Expect(crd.Status.StoredVersions[0]).To(Equal(storageVersionForCRD(&crd)), fmt.Sprintf("CustomResourceDefinition %s .status.storedVersions[0] should be storageVersion", crd.Name))

			servedGroupVersions := sets.Set[string]{}
			for _, v := range crd.Spec.Versions {
				if v.Served {
					servedGroupVersions.Insert(fmt.Sprintf("%s/%s", crd.Spec.Group, v.Name))
				}
			}
			servedGroupVersionsByGK[schema.GroupKind{
				Group: crd.Spec.Group,
				Kind:  crd.Spec.Names.Kind,
			}] = servedGroupVersions
		}
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("Validate CRs")
	Eventually(func() error {
		graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
		if err != nil {
			return err
		}

		var allErrs []error
		for _, node := range graph {
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(node.Object.APIVersion)
			obj.SetKind(node.Object.Kind)
			obj.SetNamespace(node.Object.Namespace)
			obj.SetName(node.Object.Name)
			if err := proxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				return errors.Wrapf(err, "failed to get %s %s", node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name))
			}

			servedGroupVersions, ok := servedGroupVersionsByGK[schema.GroupKind{
				Group: obj.GroupVersionKind().Group,
				Kind:  obj.GetKind(),
			}]
			if !ok {
				// Skip validation if the CRD was filtered out by crdFilterFunction.
				continue
			}

			for _, managedField := range obj.GetManagedFields() {
				if servedGroupVersions.Has(managedField.APIVersion) {
					continue
				}

				allErrs = append(allErrs, fmt.Errorf("unexpected managedField entry using not served apiVersion %s in %s %s",
					managedField.APIVersion, node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name)))
			}
		}
		return kerrors.NewAggregate(allErrs)
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
}

func storageVersionForCRD(crd *apiextensionsv1.CustomResourceDefinition) string {
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			return v.Name
		}
	}
	return ""
}

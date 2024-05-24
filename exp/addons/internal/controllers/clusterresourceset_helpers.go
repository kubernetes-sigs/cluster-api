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

package controllers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"unicode"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	utilresource "sigs.k8s.io/cluster-api/util/resource"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

var jsonListPrefix = []byte("[")

// objsFromYamlData parses a collection of yaml documents into Unstructured objects.
// The returned objects are sorted for creation priority within the objects defined
// in the same document. The flattening of the documents preserves the original order.
func objsFromYamlData(yamlDocs [][]byte) ([]unstructured.Unstructured, error) {
	allObjs := []unstructured.Unstructured{}
	for _, data := range yamlDocs {
		isJSONList, err := isJSONList(data)
		if err != nil {
			return nil, err
		}
		objs := []unstructured.Unstructured{}
		// If it is a json list, convert each list element to an unstructured object.
		if isJSONList {
			var results []map[string]interface{}
			// Unmarshal the JSON to the interface.
			if err = json.Unmarshal(data, &results); err == nil {
				for i := range results {
					var u unstructured.Unstructured
					u.SetUnstructuredContent(results[i])
					objs = append(objs, u)
				}
			}
		} else {
			// If it is not a json list, data is either json or yaml format.
			objs, err = utilyaml.ToUnstructured(data)
			if err != nil {
				return nil, errors.Wrapf(err, "failed converting data to unstructured objects")
			}
		}

		allObjs = append(allObjs, utilresource.SortForCreate(objs)...)
	}

	return allObjs, nil
}

// isJSONList returns whether the data is in JSON list format.
func isJSONList(data []byte) (bool, error) {
	const peekSize = 32
	buffer := bufio.NewReaderSize(bytes.NewReader(data), peekSize)
	b, err := buffer.Peek(peekSize)
	if err != nil {
		return false, err
	}
	trim := bytes.TrimLeftFunc(b, unicode.IsSpace)
	return bytes.HasPrefix(trim, jsonListPrefix), nil
}

func createUnstructured(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	if err := c.Create(ctx, obj); err != nil {
		return errors.Wrapf(
			err,
			"creating object %s %s",
			obj.GroupVersionKind(),
			klog.KObj(obj),
		)
	}

	return nil
}

// getOrCreateClusterResourceSetBinding retrieves ClusterResourceSetBinding resource owned by the cluster or create a new one if not found.
func (r *ClusterResourceSetReconciler) getOrCreateClusterResourceSetBinding(ctx context.Context, cluster *clusterv1.Cluster, clusterResourceSet *addonsv1.ClusterResourceSet) (*addonsv1.ClusterResourceSetBinding, error) {
	clusterResourceSetBinding := &addonsv1.ClusterResourceSetBinding{}
	clusterResourceSetBindingKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	if err := r.Client.Get(ctx, clusterResourceSetBindingKey, clusterResourceSetBinding); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		clusterResourceSetBinding.Name = cluster.Name
		clusterResourceSetBinding.Namespace = cluster.Namespace
		clusterResourceSetBinding.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: addonsv1.GroupVersion.String(),
				Kind:       "ClusterResourceSet",
				Name:       clusterResourceSet.Name,
				UID:        clusterResourceSet.UID,
			},
		}
		clusterResourceSetBinding.Spec.Bindings = []*addonsv1.ResourceSetBinding{}
		clusterResourceSetBinding.Spec.ClusterName = cluster.Name
		if err := r.Client.Create(ctx, clusterResourceSetBinding); err != nil {
			if apierrors.IsAlreadyExists(err) {
				if err = r.Client.Get(ctx, clusterResourceSetBindingKey, clusterResourceSetBinding); err != nil {
					return nil, err
				}
				return clusterResourceSetBinding, nil
			}
			return nil, errors.Wrapf(err, "failed to create clusterResourceSetBinding for cluster: %s/%s", cluster.Namespace, cluster.Name)
		}
	}
	return clusterResourceSetBinding, nil
}

// getConfigMap retrieves any ConfigMap from the given name and namespace.
func getConfigMap(ctx context.Context, c client.Client, configmapName types.NamespacedName) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Namespace: configmapName.Namespace,
		Name:      configmapName.Name,
	}
	if err := c.Get(ctx, configMapKey, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

// getSecret retrieves any Secret from the given secret name and namespace.
func getSecret(ctx context.Context, c client.Client, secretName types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: secretName.Namespace,
		Name:      secretName.Name,
	}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func computeHash(dataArr [][]byte) string {
	hash := sha256.New()
	for i := range dataArr {
		_, err := hash.Write(dataArr[i])
		if err != nil {
			continue
		}
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// normalizeData reads content of the resource (configmap or secret) and returns
// them serialized with constant order. Secret's data is base64 decoded.
// This is useful to achieve consistent data  between runs, since the content
// of the data field is a map and its order is non-deterministic.
func normalizeData(resource *unstructured.Unstructured) ([][]byte, error) {
	// Since maps are not ordered, we need to order them to get the same hash at each reconcile.
	keys := make([]string, 0)
	data, ok := resource.UnstructuredContent()["data"]
	if !ok {
		return nil, errors.Errorf("failed to get data field from resource %s", klog.KObj(resource))
	}

	unstructuredData := data.(map[string]interface{})
	for key := range unstructuredData {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	dataList := make([][]byte, 0)
	for _, key := range keys {
		val, ok, err := unstructured.NestedString(unstructuredData, key)
		if err != nil {
			return nil, errors.Wrapf(err, "getting value for field %s in data from resource %s", key, klog.KObj(resource))
		}
		if !ok {
			return nil, errors.Errorf("value for field %s not present in data from resource %s", key, klog.KObj(resource))
		}

		byteArr := []byte(val)
		// If the resource is a Secret, data needs to be decoded.
		if resource.GetKind() == string(addonsv1.SecretClusterResourceSetResourceKind) {
			byteArr, _ = base64.StdEncoding.DecodeString(val)
		}

		dataList = append(dataList, byteArr)
	}

	return dataList, nil
}

func getClusterNameFromOwnerRef(obj metav1.ObjectMeta) (string, error) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind != "Cluster" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return "", errors.Wrap(err, "failed to find cluster name in ownerRefs")
		}

		if gv.Group != clusterv1.GroupVersion.Group {
			continue
		}
		if ref.Name == "" {
			return "", errors.New("failed to find cluster name in ownerRefs: ref name is empty")
		}
		return ref.Name, nil
	}
	return "", errors.New("failed to find cluster name in ownerRefs: no cluster ownerRef")
}

// ensureKubernetesServiceCreated ensures that the Service for Kubernetes API Server has been created.
func ensureKubernetesServiceCreated(ctx context.Context, client client.Client) error {
	err := client.Get(ctx, types.NamespacedName{
		Namespace: metav1.NamespaceDefault,
		Name:      "kubernetes",
	}, &corev1.Service{})
	if err != nil {
		return err
	}
	return nil
}

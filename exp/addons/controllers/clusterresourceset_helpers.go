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
	"encoding/json"
	"fmt"
	"unicode"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	utilresource "sigs.k8s.io/cluster-api/util/resource"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var jsonListPrefix = []byte("[")

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

func apply(ctx context.Context, c client.Client, data []byte) error {
	isJSONList, err := isJSONList(data)
	if err != nil {
		return err
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
			return errors.Wrapf(err, "failed converting data to unstructured objects")
		}
	}

	errList := []error{}
	sortedObjs := utilresource.SortForCreate(objs)
	for i := range sortedObjs {
		if err := applyUnstructured(ctx, c, &objs[i]); err != nil {
			errList = append(errList, err)
		}
	}
	return kerrors.NewAggregate(errList)
}

func applyUnstructured(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	// Create the object on the API server.
	// TODO: Errors are only logged. If needed, exponential backoff or requeuing could be used here for remedying connection glitches etc.
	if err := c.Create(ctx, obj); err != nil {
		// The create call is idempotent, so if the object already exists
		// then do not consider it to be an error.
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(
				err,
				"failed to create object %s %s/%s",
				obj.GroupVersionKind(),
				obj.GetNamespace(),
				obj.GetName())
		}
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
		clusterResourceSetBinding.OwnerReferences = util.EnsureOwnerRef(clusterResourceSetBinding.OwnerReferences, metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		})
		clusterResourceSetBinding.OwnerReferences = util.EnsureOwnerRef(clusterResourceSetBinding.OwnerReferences, *metav1.NewControllerRef(clusterResourceSet, clusterResourceSet.GroupVersionKind()))

		clusterResourceSetBinding.Spec.Bindings = []*addonsv1.ResourceSetBinding{}
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

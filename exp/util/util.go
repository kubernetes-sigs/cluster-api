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

// Package util implements utility functions.
package util

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels/format"
)

// GetOwnerMachinePool returns the MachinePool objects owning the current resource.
func GetOwnerMachinePool(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*expv1.MachinePool, error) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind != "MachinePool" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if gv.Group == expv1.GroupVersion.Group {
			return GetMachinePoolByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetMachinePoolByName finds and returns a MachinePool object using the specified params.
func GetMachinePoolByName(ctx context.Context, c client.Client, namespace, name string) (*expv1.MachinePool, error) {
	m := &expv1.MachinePool{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetMachinePoolByLabels finds and returns a MachinePool object using the value of clusterv1.MachinePoolNameLabel.
// This differs from GetMachinePoolByName as the label value can be a hash.
func GetMachinePoolByLabels(ctx context.Context, c client.Client, namespace string, labels map[string]string) (*expv1.MachinePool, error) {
	selector := map[string]string{}
	if clusterName, ok := labels[clusterv1.ClusterNameLabel]; ok {
		selector = map[string]string{clusterv1.ClusterNameLabel: clusterName}
	}

	if poolNameHash, ok := labels[clusterv1.MachinePoolNameLabel]; ok {
		machinePoolList := &expv1.MachinePoolList{}
		if err := c.List(ctx, machinePoolList, client.InNamespace(namespace), client.MatchingLabels(selector)); err != nil {
			return nil, errors.Wrapf(err, "failed to list MachinePools using labels %v", selector)
		}

		for _, mp := range machinePoolList.Items {
			if format.MustFormatValue(mp.Name) == poolNameHash {
				return &mp, nil
			}
		}
	} else {
		return nil, errors.Errorf("labels missing required key `%s`", clusterv1.MachinePoolNameLabel)
	}

	return nil, nil
}

// MachinePoolToInfrastructureMapFunc returns a handler.MapFunc that watches for
// MachinePool events and returns reconciliation requests for an infrastructure provider object.
func MachinePoolToInfrastructureMapFunc(ctx context.Context, gvk schema.GroupVersionKind) handler.MapFunc {
	log := ctrl.LoggerFrom(ctx)
	return func(_ context.Context, o client.Object) []reconcile.Request {
		m, ok := o.(*expv1.MachinePool)
		if !ok {
			log.V(4).Info("Not a machine pool", "Object", klog.KObj(o))
			return nil
		}
		log := log.WithValues("MachinePool", klog.KObj(o))

		gk := gvk.GroupKind()
		ref := m.Spec.Template.Spec.InfrastructureRef
		// Return early if the GroupKind doesn't match what we expect.
		infraGK := ref.GroupVersionKind().GroupKind()
		if gk != infraGK {
			log.V(4).Info("Infra kind doesn't match filter group kind", "infrastructureGroupKind", infraGK.String())
			return nil
		}

		log.V(4).Info("Projecting object")
		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: m.Namespace,
					Name:      ref.Name,
				},
			},
		}
	}
}

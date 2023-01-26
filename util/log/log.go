/*
Copyright 2017 The Kubernetes Authors.

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

// Package log provides log utils.
package log

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// AddOwners adds the owners of an Object based on OwnerReferences as k/v pairs to the logger in ctx.
// Note: If an owner is a MachineSet we also add the owners from the MachineSet OwnerReferences.
func AddOwners(ctx context.Context, c client.Client, obj metav1.Object) (context.Context, logr.Logger, error) {
	log := ctrl.LoggerFrom(ctx)

	owners, err := getOwners(ctx, c, obj)
	if err != nil {
		return nil, logr.Logger{}, errors.Wrapf(err, "failed to add object hierarchy to logger")
	}

	// Add owners as k/v pairs.
	keysAndValues := []interface{}{}
	addedKinds := sets.Set[string]{}
	for _, owner := range owners {
		// Don't add duplicate kinds.
		if addedKinds.Has(owner.Kind) {
			continue
		}

		keysAndValues = append(keysAndValues, owner.Kind, klog.KRef(owner.Namespace, owner.Name))
		addedKinds.Insert(owner.Kind)
	}
	log = log.WithValues(keysAndValues...)

	ctx = ctrl.LoggerInto(ctx, log)
	return ctx, log, nil
}

// owner represents an owner of an object.
type owner struct {
	Kind      string
	Name      string
	Namespace string
}

// getOwners returns owners of an Object based on OwnerReferences.
// Note: If an owner is a MachineSet we also return the owners from the MachineSet OwnerReferences.
func getOwners(ctx context.Context, c client.Client, obj metav1.Object) ([]owner, error) {
	owners := []owner{}
	for _, ownerRef := range obj.GetOwnerReferences() {
		owners = append(owners, owner{
			Kind:      ownerRef.Kind,
			Namespace: obj.GetNamespace(),
			Name:      ownerRef.Name,
		})

		// continue if the ownerRef does not point to a MachineSet.
		if ownerRef.Kind != "MachineSet" {
			continue
		}

		// get owners of the MachineSet.
		var ms clusterv1.MachineSet
		if err := c.Get(ctx, client.ObjectKey{Namespace: obj.GetNamespace(), Name: ownerRef.Name}, &ms); err != nil {
			// continue if the MachineSet doesn't exist.
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to get owners: failed to get MachineSet %s", klog.KRef(obj.GetNamespace(), ownerRef.Name))
		}

		for _, ref := range ms.GetOwnerReferences() {
			owners = append(owners, owner{
				Kind:      ref.Kind,
				Namespace: obj.GetNamespace(),
				Name:      ref.Name,
			})
		}
	}

	return owners, nil
}

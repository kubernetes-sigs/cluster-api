/*
Copyright 2021 The Kubernetes Authors.

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

package scope

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Liens tracks the intent of objects to reference other objects at
// a later stage of the reconcile process.
type Liens map[string][]client.Object

// Add a liens for an object that will be referenced by an owner at
// a later stage of the reconcile process.
func (l Liens) Add(owner, object client.Object) {
	ownerKey := LiensKey(owner)
	l[ownerKey] = append(l[ownerKey], object)
}

// Forget all the liens for an owner, recognizing that
// the owner object now is actually referencing all the object it was
// committed to use.
func (l Liens) Forget(owner client.Object) {
	ownerKey := LiensKey(owner)
	delete(l, ownerKey)
}

// Collect all the pending liens. This method should be called
// whenever the owner objects fails to reference all the object they were
// committed to use (usually due to an error).
// Objects involved in liens not satisfied are deleted from the
// cluster, thus avoiding orphan objects.
func (l Liens) Collect(ctx context.Context, c client.Client) error {
	log := tlog.LoggerFrom(ctx)

	errList := []error{}
	for ownerKey, objects := range l {
		for _, object := range objects {
			log.Infof("Cleaning up %s given that reference from %s was not set", tlog.KObj{Obj: object}, ownerKey)
			if err := c.Delete(ctx, object); err != nil {
				if !apierrors.IsNotFound(err) {
					errList = append(errList, errors.Wrapf(err, "failed to cleanup %s", tlog.KObj{Obj: object}))
				}
			}
		}
		delete(l, ownerKey)
	}
	return kerrors.NewAggregate(errList)
}

// LiensKey return the key for an object in the Liens map.
func LiensKey(obj client.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
}

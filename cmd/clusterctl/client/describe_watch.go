/*
Copyright 2026 The Kubernetes Authors.

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

package client

import (
	"context"
	"sync"
	"time"

	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	toolscache "k8s.io/client-go/tools/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
)

// WatchClusterMode defines how WatchCluster detects changes to a Cluster API cluster.
type WatchClusterMode string

const (
	// WatchClusterModeInformer refreshes the object tree when one of the objects of the cluster changes,
	// using informers on the management cluster.
	WatchClusterModeInformer WatchClusterMode = "informer"

	// WatchClusterModePoll refreshes the object tree at a fixed interval.
	WatchClusterModePoll WatchClusterMode = "poll"
)

// watchClusterDebounce is the time WatchCluster waits after a change before refreshing the object tree,
// so a burst of changes (e.g. a rollout touching many objects) results in a single refresh.
const watchClusterDebounce = 500 * time.Millisecond

// WatchClusterOptions carries the options supported by WatchCluster.
type WatchClusterOptions struct {
	DescribeClusterOptions

	// Mode defines how changes to the cluster are detected. Defaults to WatchClusterModeInformer.
	Mode WatchClusterMode

	// Interval between refreshes when using WatchClusterModePoll.
	Interval time.Duration

	// OnChange is called with the initial object tree and then with a new object tree every time it is refreshed.
	// If refreshing the object tree fails, OnChange is called with the error and the watch continues.
	// If OnChange returns an error, the watch stops and the error is returned by WatchCluster.
	OnChange func(objectTree *tree.ObjectTree, err error) error
}

// WatchCluster watches a Cluster API cluster and calls options.OnChange with an up-to-date object tree
// until the context is cancelled.
func (c *clusterctlClient) WatchCluster(ctx context.Context, options WatchClusterOptions) error {
	if options.OnChange == nil {
		return pkgerrors.New("OnChange must be set")
	}
	if options.Mode == "" {
		options.Mode = WatchClusterModeInformer
	}
	if options.Mode != WatchClusterModeInformer && options.Mode != WatchClusterModePoll {
		return pkgerrors.Errorf("invalid watch mode %q, must be one of %q or %q", options.Mode, WatchClusterModeInformer, WatchClusterModePoll)
	}
	if options.Mode == WatchClusterModePoll && options.Interval <= 0 {
		return pkgerrors.Errorf("invalid watch interval %s, must be greater than zero", options.Interval)
	}

	d, err := c.newClusterDescriber(ctx, options.DescribeClusterOptions)
	if err != nil {
		return err
	}

	// Fail fast if the cluster cannot be described at all, e.g. if it does not exist.
	objectTree, err := d.describe(ctx)
	if err != nil {
		return err
	}
	if err := options.OnChange(objectTree, nil); err != nil {
		return err
	}

	switch options.Mode {
	case WatchClusterModePoll:
		return pollCluster(ctx, options.Interval, d.describe, options.OnChange)
	default:
		return watchClusterWithInformers(ctx, d, objectTree, options.OnChange)
	}
}

type describeFunc func(ctx context.Context) (*tree.ObjectTree, error)

// pollCluster refreshes the object tree every interval.
func pollCluster(ctx context.Context, interval time.Duration, describe describeFunc, onChange func(*tree.ObjectTree, error) error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := onChange(describe(ctx)); err != nil {
			return err
		}
	}
}

// watchClusterWithInformers refreshes the object tree every time an object of the cluster changes.
func watchClusterWithInformers(ctx context.Context, d *clusterDescriber, objectTree *tree.ObjectTree, onChange func(*tree.ObjectTree, error) error) error {
	restConfig, err := d.proxy.GetConfig()
	if err != nil {
		return err
	}
	metadataClient, err := metadata.NewForConfig(restConfig)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to create metadata client")
	}

	w := newClusterWatcher(
		metadatainformer.NewFilteredSharedInformerFactory(metadataClient, 0, d.namespace, nil),
		d.client.RESTMapper(),
		d.name,
		d.options.ShowClusterResourceSets,
	)

	// Informers stop when the context is cancelled, so it must be cancelled before waiting for them to terminate;
	// this also covers the watch stopping because OnChange returned an error.
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		w.factory.Shutdown()
	}()

	if err := w.update(ctx, objectTree); err != nil {
		return err
	}

	return w.run(ctx, d.describe, onChange)
}

// clusterWatcher keeps track of the objects of a Cluster API cluster and signals when one of them changes.
type clusterWatcher struct {
	factory     metadatainformer.SharedInformerFactory
	restMapper  meta.RESTMapper
	clusterName string

	// changed is signaled when a relevant object changes; it is buffered so
	// many changes happening while the object tree is refreshed result in a single signal.
	changed chan struct{}

	lock sync.RWMutex
	// informers is the set of kinds the watcher has an informer for.
	informers sets.Set[schema.GroupKind]
	// objects are the names of the objects in the current object tree, by kind.
	objects map[schema.GroupKind]sets.Set[string]
	// clusterKinds are kinds for which all the objects belonging to the cluster are relevant,
	// e.g. a new Machine must be detected even if it is not part of the current object tree.
	clusterKinds sets.Set[schema.GroupKind]
}

func newClusterWatcher(factory metadatainformer.SharedInformerFactory, restMapper meta.RESTMapper, clusterName string, showClusterResourceSets bool) *clusterWatcher {
	clusterKinds := sets.New(
		clusterv1.GroupVersion.WithKind("Cluster").GroupKind(),
		clusterv1.GroupVersion.WithKind("Machine").GroupKind(),
		clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind(),
		clusterv1.GroupVersion.WithKind("MachineSet").GroupKind(),
		clusterv1.GroupVersion.WithKind("MachinePool").GroupKind(),
	)
	if showClusterResourceSets {
		clusterKinds.Insert(addonsv1.GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind())
	}

	return &clusterWatcher{
		factory:      factory,
		restMapper:   restMapper,
		clusterName:  clusterName,
		changed:      make(chan struct{}, 1),
		informers:    sets.New[schema.GroupKind](),
		objects:      map[schema.GroupKind]sets.Set[string]{},
		clusterKinds: clusterKinds,
	}
}

// run refreshes the object tree every time a relevant object changes, until the context is cancelled.
func (w *clusterWatcher) run(ctx context.Context, describe describeFunc, onChange func(*tree.ObjectTree, error) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.changed:
		}

		// Wait for the burst of changes to settle.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(watchClusterDebounce):
		}
		// Drain changes that happened while waiting; changes happening from now on trigger another refresh.
		select {
		case <-w.changed:
		default:
		}

		objectTree, err := describe(ctx)
		if err == nil {
			// Start watching kinds that showed up in the object tree, e.g. the MachineInfrastructure of a new Machine.
			if updateErr := w.update(ctx, objectTree); updateErr != nil {
				return updateErr
			}
		}
		if err := onChange(objectTree, err); err != nil {
			return err
		}
	}
}

// update records the objects in the object tree and ensures there is an informer for each of their kinds.
func (w *clusterWatcher) update(ctx context.Context, objectTree *tree.ObjectTree) error {
	objects := objectsInTree(objectTree)

	w.lock.Lock()
	w.objects = objects
	w.lock.Unlock()

	for gk := range w.clusterKinds.Union(sets.KeySet(objects)) {
		if err := w.ensureInformer(ctx, gk); err != nil {
			return err
		}
	}
	return nil
}

// ensureInformer starts an informer for the given kind, if not already started.
func (w *clusterWatcher) ensureInformer(ctx context.Context, gk schema.GroupKind) error {
	w.lock.RLock()
	found := w.informers.Has(gk)
	w.lock.RUnlock()
	if found {
		return nil
	}

	mapping, err := w.restMapper.RESTMapping(gk)
	if err != nil {
		if meta.IsNoMatchError(err) {
			// The kind is not served by the management cluster, e.g. an optional CRD; nothing to watch.
			return nil
		}
		return pkgerrors.Wrapf(err, "failed to get REST mapping for %s", gk)
	}

	informer := w.factory.ForResource(mapping.Resource).Informer()
	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, isInInitialList bool) {
			// Objects existing when the informer starts are already part of the object tree.
			if !isInInitialList {
				w.notify(gk, obj)
			}
		},
		UpdateFunc: func(_, newObj any) { w.notify(gk, newObj) },
		DeleteFunc: func(obj any) { w.notify(gk, obj) },
	}); err != nil {
		return pkgerrors.Wrapf(err, "failed to add event handler for %s", gk)
	}

	w.factory.Start(ctx.Done())
	w.factory.WaitForCacheSync(ctx.Done())

	w.lock.Lock()
	w.informers.Insert(gk)
	w.lock.Unlock()
	return nil
}

// notify signals a change if the object is relevant for the cluster.
func (w *clusterWatcher) notify(gk schema.GroupKind, obj any) {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	o, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	if !w.isRelevant(gk, o.GetName(), o.GetLabels()) {
		return
	}

	select {
	case w.changed <- struct{}{}:
	default:
		// A change is already pending.
	}
}

// isRelevant returns true if a change to the given object might change the object tree.
func (w *clusterWatcher) isRelevant(gk schema.GroupKind, name string, labels map[string]string) bool {
	switch gk {
	case clusterv1.GroupVersion.WithKind("Cluster").GroupKind(),
		addonsv1.GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind():
		return name == w.clusterName
	}

	w.lock.RLock()
	defer w.lock.RUnlock()

	if w.clusterKinds.Has(gk) && labels[clusterv1.ClusterNameLabel] == w.clusterName {
		return true
	}
	return w.objects[gk].Has(name)
}

// objectsInTree returns the names of the objects in the object tree, by kind.
// Virtual objects, e.g. groups, and objects only referenced by name, e.g. templates, are ignored
// because they are not read from the management cluster.
func objectsInTree(objectTree *tree.ObjectTree) map[schema.GroupKind]sets.Set[string] {
	objects := map[schema.GroupKind]sets.Set[string]{}

	var visit func(obj ctrlclient.Object)
	visit = func(obj ctrlclient.Object) {
		if _, ok := obj.(*tree.NodeObject); !ok {
			gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
			if !gk.Empty() {
				if _, ok := objects[gk]; !ok {
					objects[gk] = sets.New[string]()
				}
				objects[gk].Insert(obj.GetName())
			}
		}
		for _, child := range objectTree.GetObjectsByParent(obj.GetUID()) {
			visit(child)
		}
	}
	visit(objectTree.GetRoot())

	return objects
}

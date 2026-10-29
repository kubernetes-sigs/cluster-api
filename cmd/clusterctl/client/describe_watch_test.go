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
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	metadatafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/metadata/metadatainformer"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	fakeinfrastructure "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/infrastructure"
)

const (
	watchTestNamespace = "ns1"
	watchTestCluster   = "cluster1"
)

func watchTestObjects() []ctrlclient.Object {
	return test.NewFakeCluster(watchTestNamespace, watchTestCluster).
		WithControlPlane(
			test.NewFakeControlPlane("cp").
				WithMachines(
					test.NewFakeMachine("cp1"),
				),
		).
		WithMachineDeployments(
			test.NewFakeMachineDeployment("md1").
				WithMachineSets(
					test.NewFakeMachineSet("ms1").
						WithMachines(
							test.NewFakeMachine("m1"),
						),
				),
		).
		Objs()
}

func watchTestObjectTree(t *testing.T, objs []ctrlclient.Object) *tree.ObjectTree {
	t.Helper()
	g := NewWithT(t)

	for _, crd := range test.FakeCRDList() {
		objs = append(objs, crd)
	}
	c, err := test.NewFakeProxy().WithObjs(objs...).NewClient(t.Context())
	g.Expect(err).ToNot(HaveOccurred())

	objectTree, err := tree.Discovery(t.Context(), c, watchTestNamespace, watchTestCluster, tree.DiscoverOptions{Grouping: false, Echo: true})
	g.Expect(err).ToNot(HaveOccurred())
	return objectTree
}

func Test_objectsInTree(t *testing.T) {
	g := NewWithT(t)

	objects := objectsInTree(watchTestObjectTree(t, watchTestObjects()))

	// Only objects read from the management cluster are returned, virtual objects like the WorkerGroup are not.
	g.Expect(sets.KeySet(objects).UnsortedList()).To(ConsistOf(
		clusterv1.GroupVersion.WithKind("Cluster").GroupKind(),
		clusterv1.GroupVersion.WithKind("Machine").GroupKind(),
		clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind(),
		fakeinfrastructure.GroupVersion.WithKind("GenericInfrastructureCluster").GroupKind(),
		fakeinfrastructure.GroupVersion.WithKind("GenericInfrastructureMachine").GroupKind(),
		schema.GroupKind{Group: "controlplane.cluster.x-k8s.io", Kind: "GenericControlPlane"},
		schema.GroupKind{Group: "bootstrap.cluster.x-k8s.io", Kind: "GenericBootstrapConfig"},
	))
	g.Expect(objects[clusterv1.GroupVersion.WithKind("Cluster").GroupKind()].UnsortedList()).To(ConsistOf(watchTestCluster))
	g.Expect(objects[clusterv1.GroupVersion.WithKind("Machine").GroupKind()]).To(HaveLen(2))
}

func Test_clusterWatcher_isRelevant(t *testing.T) {
	clusterGK := clusterv1.GroupVersion.WithKind("Cluster").GroupKind()
	machineGK := clusterv1.GroupVersion.WithKind("Machine").GroupKind()
	infraMachineGK := fakeinfrastructure.GroupVersion.WithKind("GenericInfrastructureMachine").GroupKind()

	w := newClusterWatcher(nil, nil, watchTestCluster, false)
	w.objects = map[schema.GroupKind]sets.Set[string]{
		infraMachineGK: sets.New("infra-m1"),
	}

	tests := []struct {
		name   string
		gk     schema.GroupKind
		objKey string
		labels map[string]string
		want   bool
	}{
		{
			name:   "the cluster",
			gk:     clusterGK,
			objKey: watchTestCluster,
			want:   true,
		},
		{
			name:   "another cluster",
			gk:     clusterGK,
			objKey: "cluster2",
			want:   false,
		},
		{
			name:   "a new machine of the cluster",
			gk:     machineGK,
			objKey: "m2",
			labels: map[string]string{clusterv1.ClusterNameLabel: watchTestCluster},
			want:   true,
		},
		{
			name:   "a machine of another cluster",
			gk:     machineGK,
			objKey: "m2",
			labels: map[string]string{clusterv1.ClusterNameLabel: "cluster2"},
			want:   false,
		},
		{
			name:   "a provider object in the object tree",
			gk:     infraMachineGK,
			objKey: "infra-m1",
			want:   true,
		},
		{
			name:   "a provider object not in the object tree",
			gk:     infraMachineGK,
			objKey: "infra-m2",
			labels: map[string]string{clusterv1.ClusterNameLabel: watchTestCluster},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(w.isRelevant(tt.gk, tt.objKey, tt.labels)).To(Equal(tt.want))
		})
	}
}

func Test_clusterWatcher_informers(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(t.Context())

	objs := watchTestObjects()
	objectTree := watchTestObjectTree(t, objs)

	// Serve all the kinds of the test objects, plus the cluster kinds that are always watched.
	gvks := []schema.GroupVersionKind{}
	for _, kind := range []string{"Cluster", "Machine", "MachineDeployment", "MachineSet", "MachinePool"} {
		gvks = append(gvks, clusterv1.GroupVersion.WithKind(kind))
	}
	existing := []*metav1.PartialObjectMetadata{}
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		gvks = append(gvks, gvk)
		existing = append(existing, partialObjectMetadata(gvk, obj.GetName(), obj.GetLabels()))
	}
	groupVersions := sets.New[schema.GroupVersion]()
	for _, gvk := range gvks {
		groupVersions.Insert(gvk.GroupVersion())
	}
	restMapper := meta.NewDefaultRESTMapper(groupVersions.UnsortedList())
	for _, gvk := range gvks {
		restMapper.Add(gvk, meta.RESTScopeNamespace)
	}

	scheme := metadatafake.NewTestScheme()
	g.Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)
	create := func(obj *metav1.PartialObjectMetadata) {
		mapping, err := restMapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		g.Expect(err).ToNot(HaveOccurred())
		_, err = metadataClient.Resource(mapping.Resource).Namespace(watchTestNamespace).(metadatafake.MetadataClient).CreateFake(obj, metav1.CreateOptions{})
		g.Expect(err).ToNot(HaveOccurred())
	}
	for _, obj := range existing {
		create(obj)
	}

	w := newClusterWatcher(metadatainformer.NewFilteredSharedInformerFactory(metadataClient, 0, watchTestNamespace, nil), restMapper, watchTestCluster, false)
	defer func() {
		cancel()
		w.factory.Shutdown()
	}()
	g.Expect(w.update(ctx, objectTree)).To(Succeed())

	// Objects existing when the watch starts do not signal a change.
	g.Consistently(w.changed, 200*time.Millisecond).ShouldNot(Receive())

	// A new machine of the cluster signals a change.
	create(partialObjectMetadata(clusterv1.GroupVersion.WithKind("Machine"), "m2", map[string]string{clusterv1.ClusterNameLabel: watchTestCluster}))
	g.Eventually(w.changed).Should(Receive())

	// A new machine of another cluster does not signal a change.
	create(partialObjectMetadata(clusterv1.GroupVersion.WithKind("Machine"), "m3", map[string]string{clusterv1.ClusterNameLabel: "cluster2"}))
	g.Consistently(w.changed, 200*time.Millisecond).ShouldNot(Receive())

	// A change to a provider object in the object tree signals a change.
	for name := range objectsInTree(objectTree)[fakeinfrastructure.GroupVersion.WithKind("GenericInfrastructureCluster").GroupKind()] {
		obj := partialObjectMetadata(fakeinfrastructure.GroupVersion.WithKind("GenericInfrastructureCluster"), name, map[string]string{"changed": "true"})
		_, err := metadataClient.Resource(fakeinfrastructure.GroupVersion.WithResource("genericinfrastructureclusters")).Namespace(watchTestNamespace).(metadatafake.MetadataClient).UpdateFake(obj, metav1.UpdateOptions{})
		g.Expect(err).ToNot(HaveOccurred())
	}
	g.Eventually(w.changed).Should(Receive())
}

func Test_clusterWatcher_run(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objectTree := watchTestObjectTree(t, watchTestObjects())
	w := newClusterWatcher(nil, nil, watchTestCluster, false)
	// Pretend informers are already running for all the kinds, so run does not need a management cluster.
	for gk := range w.clusterKinds.Union(sets.KeySet(objectsInTree(objectTree))) {
		w.informers.Insert(gk)
	}

	var describes, changes atomic.Int32
	var describe describeFunc = func(context.Context) (*tree.ObjectTree, error) {
		describes.Add(1)
		return objectTree, nil
	}
	onChange := func(*tree.ObjectTree, error) error {
		if changes.Add(1) == 2 {
			return pkgerrors.New("stop")
		}
		return nil
	}

	done := make(chan error)
	go func() { done <- w.run(ctx, describe, onChange) }()

	// A burst of changes results in a single refresh.
	for range 10 {
		w.notify(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), partialObjectMetadata(clusterv1.GroupVersion.WithKind("Cluster"), watchTestCluster, nil))
	}
	g.Eventually(changes.Load).Should(BeEquivalentTo(1))
	g.Consistently(changes.Load, 2*watchClusterDebounce).Should(BeEquivalentTo(1))
	g.Expect(describes.Load()).To(BeEquivalentTo(1))

	// An error returned by onChange stops the watch.
	w.notify(clusterv1.GroupVersion.WithKind("Cluster").GroupKind(), partialObjectMetadata(clusterv1.GroupVersion.WithKind("Cluster"), watchTestCluster, nil))
	g.Eventually(done).Should(Receive(MatchError("stop")))
}

func Test_pollCluster(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var describes atomic.Int32
	describe := func(context.Context) (*tree.ObjectTree, error) {
		if describes.Add(1) == 1 {
			return nil, pkgerrors.New("transient error")
		}
		return &tree.ObjectTree{}, nil
	}

	var gotErr error
	var changes int
	onChange := func(_ *tree.ObjectTree, err error) error {
		changes++
		if changes == 1 {
			gotErr = err
		}
		if changes == 3 {
			cancel()
		}
		return nil
	}

	// Describe errors are reported to onChange without stopping the watch, and cancelling the context stops it cleanly.
	g.Expect(pollCluster(ctx, 10*time.Millisecond, describe, onChange)).To(Succeed())
	g.Expect(gotErr).To(MatchError("transient error"))
	g.Expect(changes).To(Equal(3))
}

func partialObjectMetadata(gvk schema.GroupVersionKind, name string, labels map[string]string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: watchTestNamespace,
			Name:      name,
			Labels:    labels,
		},
	}
}

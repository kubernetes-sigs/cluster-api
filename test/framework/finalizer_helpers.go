package framework

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/util/patch"
)

func AssertFinalizersAfterDeletion(ctx context.Context, cli client.Client, namespace, clusterName string) {
	// Check that the ownerReferences are as expected on the first iteration.
	initialFinalizers := getAllFinalizers(ctx, cli, namespace)

	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}

	// Pause the cluster by setting .spec.paused: "true"
	setClusterPause(ctx, cli, clusterKey,
		true)

	removeFinalizers(ctx, cli, namespace)

	// Unpause the cluster by setting .spec.paused: "false"
	setClusterPause(ctx, cli, clusterKey,
		false)

	// Annotate the clusterClass, if one is in use, to speed up reconciliation.
	annotateClusterClass(ctx, cli, clusterKey)

	Eventually(func() {
		afterDeleteFinalizers := getAllFinalizers(ctx, cli, namespace)

		missing := map[string][]string{}
		for k, v := range initialFinalizers {
			if _, ok := afterDeleteFinalizers[k]; !ok {
				missing[k] = v
			}
		}
		missingString := ""
		for k, v := range missing {
			missingString = fmt.Sprintf("%s\n%s %s", missingString, k, v)
		}
		Expect(len(missing)).To(Equal(0), missingString)
	})
}

func getAllFinalizers(ctx context.Context, cli client.Client, namespace string) map[string][]string {
	finalizers := map[string][]string{}
	graph, err := clusterctlcluster.GetOwnerGraph(namespace)
	Expect(err).To(BeNil())
	for _, v := range graph {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(v.Object.APIVersion)
		obj.SetKind(v.Object.Kind)
		err := cli.Get(ctx, client.ObjectKey{Namespace: v.Object.Namespace, Name: v.Object.Name}, obj)
		Expect(err).To(BeNil())
		if obj.GetFinalizers() != nil {
			finalizers[fmt.Sprintf("%s/%s", v.Object.Kind, v.Object.Name)] = obj.GetFinalizers()
		}
	}
	return finalizers
}

func setClusterPause(ctx context.Context, cli client.Client, clusterKey types.NamespacedName, value bool) {
	cluster := &clusterv1.Cluster{}
	Expect(cli.Get(ctx, clusterKey, cluster)).To(Succeed())

	unpausePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%v}}", value)))
	Expect(cli.Patch(ctx, cluster, unpausePatch)).To(Succeed())
}

func annotateClusterClass(ctx context.Context, cli client.Client, clusterKey types.NamespacedName) {
	cluster := &clusterv1.Cluster{}
	Expect(cli.Get(ctx, clusterKey, cluster)).To(Succeed())

	if cluster.Spec.Topology != nil {
		class := &clusterv1.ClusterClass{}
		Expect(cli.Get(ctx, client.ObjectKey{Namespace: clusterKey.Namespace, Name: cluster.Spec.Topology.Class}, class)).To(Succeed())
		annotationPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/modifiedAt\":\"%v\"}}}", time.Now().Format(time.RFC3339))))
		Expect(cli.Patch(ctx, class, annotationPatch)).To(Succeed())
	}
}

func removeFinalizers(ctx context.Context, cli client.Client, namespace string) {
	graph, err := clusterctlcluster.GetOwnerGraph(namespace)
	Expect(err).To(BeNil())
	for _, object := range graph {
		ref := object.Object
		// Once all Clusters are paused remove the OwnerReference from all objects in the graph.
		obj := new(unstructured.Unstructured)
		obj.SetAPIVersion(ref.APIVersion)
		obj.SetKind(ref.Kind)
		obj.SetName(ref.Name)

		Expect(cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: object.Object.Name}, obj)).To(Succeed())
		helper, err := patch.NewHelper(obj, cli)
		Expect(err).To(BeNil())
		obj.SetFinalizers([]string{})
		Expect(helper.Patch(ctx, obj)).To(Succeed())
	}
}

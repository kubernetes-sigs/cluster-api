/*
Copyright 2022 The Kubernetes Authors.

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

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/gobuffalo/flect"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster/internal/dryrun"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/feature"
	clustertopologycontroller "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster"
	"sigs.k8s.io/cluster-api/internal/webhooks"
)

const (
	maxClusterPerInput        = 1
	maxClusterClassesPerInput = 1
)

// TopologyClient has methods to work with ClusterClass and ManagedTopologies.
type TopologyClient interface {
	Plan(in *TopologyPlanInput) (*TopologyPlanOutput, error)
}

// topologyClient implements TopologyClient.
type topologyClient struct {
	proxy           Proxy
	inventoryClient InventoryClient
}

// ensure topologyClient implements TopologyClient.
var _ TopologyClient = &topologyClient{}

// newTopologyClient returns a TopologyClient.
func newTopologyClient(proxy Proxy, inventoryClient InventoryClient) TopologyClient {
	return &topologyClient{
		proxy:           proxy,
		inventoryClient: inventoryClient,
	}
}

// TopologyPlanInput defines the input for the Plan function.
type TopologyPlanInput struct {
	Objs              []*unstructured.Unstructured
	TargetClusterName string
	TargetNamespace   string
}

// PatchSummary defined the patch observed on an object.
type PatchSummary = dryrun.PatchSummary

// ChangeSummary defines all the changes detected by the plan operation.
type ChangeSummary = dryrun.ChangeSummary

// TopologyPlanOutput defines the output of the Plan function.
type TopologyPlanOutput struct {
	// Clusters is the list clusters affected by the input.
	Clusters []client.ObjectKey
	// ClusterClasses is the list of clusters affected by the input.
	ClusterClasses []client.ObjectKey
	// ReconciledCluster is the cluster on which the topology reconciler loop is executed.
	// If there is only one affected cluster then it becomes the ReconciledCluster. If not,
	// the ReconciledCluster is chosen using additional information in the TopologyPlanInput.
	// ReconciledCluster can be empty if no single target cluster is provided.
	ReconciledCluster *client.ObjectKey
	// ChangeSummary is the full list of changes (objects created, modified and deleted) observed
	// on the ReconciledCluster. ChangeSummary is empty if ReconciledCluster is empty.
	*ChangeSummary
}

// Plan performs a dry run execution of the topology reconciler using the given inputs.
// It returns a summary of the changes observed during the execution.
func (t *topologyClient) Plan(in *TopologyPlanInput) (*TopologyPlanOutput, error) {
	ctx := context.TODO()
	log := logf.Log

	// Make sure the inputs are valid.
	if err := t.validateInput(in); err != nil {
		return nil, errors.Wrap(err, "input failed validation")
	}

	// If there is a reachable apiserver with CAPI installed fetch a client for the server.
	// This client will be used as a fall back client when looking for objects that are not
	// in the input.
	// Example: This client will be used to fetch the underlying ClusterClass when the input
	// only has a Cluster object.
	var c client.Client
	if err := t.proxy.CheckClusterAvailable(); err == nil {
		if initialized, err := t.inventoryClient.CheckCAPIInstalled(); err == nil && initialized {
			c, err = t.proxy.NewClient()
			if err != nil {
				return nil, errors.Wrap(err, "failed to create a client to the cluster")
			}
			log.Info("Detected a cluster with Cluster API installed. Will use it to fetch missing objects.")
		}
	}

	// Prepare the inputs for dry running the reconciler. This includes steps like setting missing namespaces on objects
	// and adjusting cluster objects to reflect updated state.
	if err := t.prepareInput(ctx, in, c); err != nil {
		return nil, errors.Wrap(err, "failed preparing input")
	}
	// Run defaulting and validation on core CAPI objects - Cluster and ClusterClasses.
	// This mimics the defaulting and validation webhooks that will run on the objects during a real execution.
	// Running defaulting and validation on these objects helps to improve the UX of using the plan operation.
	// This is especially important when working with Clusters and ClusterClasses that use variable and patches.
	if err := t.runDefaultAndValidationWebhooks(ctx, in, c); err != nil {
		return nil, errors.Wrap(err, "failed defaulting and validation on input objects")
	}

	objs := []client.Object{}
	// Add all the objects from the input to the list used when initializing the dry run client.
	for _, o := range in.Objs {
		objs = append(objs, o)
	}
	// Add mock CRDs of all the provider objects in the input to the list used when initializing the dry run client.
	// Adding these CRDs makes sure that UpdateReferenceAPIContract calls in the reconciler can work.
	for _, o := range t.generateCRDs(in.Objs) {
		objs = append(objs, o)
	}

	dryRunClient := dryrun.NewClient(c, objs)
	// Calculate affected ClusterClasses.
	affectedClusterClasses, err := t.affectedClusterClasses(ctx, in, dryRunClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed calculating affected ClusterClasses")
	}
	// Calculate affected Clusters.
	affectedClusters, err := t.affectedClusters(ctx, in, dryRunClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed calculating affected Clusters")
	}

	res := &TopologyPlanOutput{
		Clusters:       affectedClusters,
		ClusterClasses: affectedClusterClasses,
		ChangeSummary:  &dryrun.ChangeSummary{},
	}

	// Calculate the target cluster object.
	// Full changeset is only generated for the target cluster.
	var targetCluster *client.ObjectKey
	if in.TargetClusterName != "" {
		// Check if the target cluster is among the list of affected clusters and use that.
		target := client.ObjectKey{
			Namespace: in.TargetNamespace,
			Name:      in.TargetClusterName,
		}
		if inList(affectedClusters, target) {
			targetCluster = &target
		} else {
			return nil, fmt.Errorf("target cluster %q is not among the list of affected clusters", target.String())
		}
	} else if len(affectedClusters) == 1 {
		// If no target cluster is specified and if there is only one affected cluster, use that as the target cluster.
		targetCluster = &affectedClusters[0]
	}

	if targetCluster == nil {
		// There is no target cluster, return here. We will
		// not generate a full change summary.
		return res, nil
	}

	res.ReconciledCluster = targetCluster
	reconciler := &clustertopologycontroller.Reconciler{
		Client:                    dryRunClient,
		APIReader:                 dryRunClient,
		UnstructuredCachingClient: dryRunClient,
	}
	reconciler.SetupForDryRun(&noOpRecorder{})
	request := reconcile.Request{NamespacedName: *targetCluster}
	// Run the topology reconciler.
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		return nil, errors.Wrap(err, "failed to dry run the topology controller")
	}
	// Calculate changes observed by dry run client.
	changes, err := dryRunClient.Changes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get changes made by the topology controller")
	}
	res.ChangeSummary = changes

	return res, nil
}

// validateInput checks that the topology plan input does not violate any of the below expectations:
// - no more than 1 cluster in the input.
// - no more than 1 clusterclass in the input.
func (t *topologyClient) validateInput(in *TopologyPlanInput) error {
	// Check all the objects in the input belong to the same namespace.
	// Note: It is okay if all the objects in the input do not have any namespace.
	// In such case, the list of unique namespaces will be [""].
	namespaces := uniqueNamespaces(in.Objs)
	if len(namespaces) != 1 {
		return fmt.Errorf("all the objects in the input should belong to the same namespace")
	}

	ns := namespaces[0]
	// If the objects have a non empty namespace make sure that it matches the TargetNamespace.
	if ns != "" && in.TargetNamespace != "" && ns != in.TargetNamespace {
		return fmt.Errorf("the namespace from the provided object(s) %q does not match the namespace %q", ns, in.TargetNamespace)
	}
	in.TargetNamespace = ns

	clusterCount, clusterClassCount := len(getClusters(in.Objs)), len(getClusterClasses(in.Objs))
	if clusterCount > maxClusterPerInput || clusterClassCount > maxClusterClassesPerInput {
		return fmt.Errorf(
			"input should have at most %d Cluster(s) and at most %d ClusterClass(es). Found %d Cluster(s) and %d ClusterClass(es)",
			maxClusterPerInput,
			maxClusterClassesPerInput,
			clusterCount,
			clusterClassCount,
		)
	}
	return nil
}

// prepareInput does the following on the input objects:
// - Set the target namespace on the objects if not set (this operation is generally done by kubectl)
// - Prepare cluster objects so that the state of the cluster, if modified, correctly represents
//   the expected changes.
func (t *topologyClient) prepareInput(ctx context.Context, in *TopologyPlanInput, apiReader client.Reader) error {
	if err := t.setMissingNamespaces(in.TargetNamespace, in.Objs); err != nil {
		return errors.Wrap(err, "failed to set missing namespaces")
	}

	if err := t.prepareClusters(ctx, getClusters(in.Objs), apiReader); err != nil {
		return errors.Wrap(err, "failed to prepare clusters")
	}
	return nil
}

// setMissingNamespaces sets the object to the current namespace on objects
// that are missing the namespace field.
func (t *topologyClient) setMissingNamespaces(currentNamespace string, objs []*unstructured.Unstructured) error {
	if currentNamespace == "" {
		// If TargetNamespace is not provided use "default" namespace.
		currentNamespace = metav1.NamespaceDefault
		// If a cluster is available use the current namespace as defined in its kubeconfig.
		if err := t.proxy.CheckClusterAvailable(); err == nil {
			currentNamespace, err = t.proxy.CurrentNamespace()
			if err != nil {
				return errors.Wrap(err, "failed to get current namespace")
			}
		}
	}
	// Set namespace on objects that do not have namespace value.
	// Skip Namespace objects, as they are non-namespaced.
	for i := range objs {
		isNamespace := objs[i].GroupVersionKind().Kind == namespaceKind
		if objs[i].GetNamespace() == "" && !isNamespace {
			objs[i].SetNamespace(currentNamespace)
		}
	}
	return nil
}

// prepareClusters does the following operations on each Cluster in the input.
// - Check if the Cluster exists in the real apiserver.
// - If the Cluster exists in the real apiserver we merge the object from the
//   server with the object from the input. This final object correctly represents the
//   modified cluster object.
//   Note: We are using a simple 2-way merge to calculate the final object in this function
//   to keep the function simple. In reality kubectl does a lot more. This function does not behave exactly
//   the same way as kubectl does.
// *Important note*: We do this above operation because the topology reconciler in a
//   real run takes as input a cluster object from the apiserver that has merged spec of
//   the changes in the input and the one stored in the server. For example: the cluster
//   object in the input will not have cluster.spec.infrastructureRef and cluster.spec.controlPlaneRef
//   but the merged object will have these fields set.
func (t *topologyClient) prepareClusters(ctx context.Context, clusters []*unstructured.Unstructured, apiReader client.Reader) error {
	if apiReader == nil {
		// If there is no backing server there is nothing more to do here.
		// Return early.
		return nil
	}

	// For a Cluster check if it already exists in the server. If it does, get the object from the server
	// and merge it with the Cluster from the file to get effective 'modified' Cluster.
	for _, cluster := range clusters {
		storedCluster := &clusterv1.Cluster{}
		if err := apiReader.Get(
			ctx,
			client.ObjectKey{Namespace: cluster.GetNamespace(), Name: cluster.GetName()},
			storedCluster,
		); err != nil {
			if apierrors.IsNotFound(err) {
				// The Cluster does not exist in the server. Nothing more to do here.
				continue
			}
			return errors.Wrapf(err, "failed to get Cluster %s/%s", cluster.GetNamespace(), cluster.GetName())
		}
		originalJSON, err := json.Marshal(storedCluster)
		if err != nil {
			return errors.Wrapf(err, "failed to convert Cluster %s/%s to json", storedCluster.Namespace, storedCluster.Name)
		}
		modifiedJSON, err := json.Marshal(cluster)
		if err != nil {
			return errors.Wrapf(err, "failed to convert Cluster %s/%s", cluster.GetNamespace(), cluster.GetName())
		}
		// Apply the modified object to the original one, merging the values of both;
		// in case of conflicts, values from the modified object are preserved.
		originalWithModifiedJSON, err := jsonpatch.MergePatch(originalJSON, modifiedJSON)
		if err != nil {
			return errors.Wrap(err, "failed to apply modified json to original json")
		}
		if err := json.Unmarshal(originalWithModifiedJSON, &cluster.Object); err != nil {
			return errors.Wrap(err, "failed to convert modified json to object")
		}
	}
	return nil
}

// runDefaultAndValidationWebhooks runs the defaulting and validation webhooks on the
// ClusterClass and Cluster objects in the input thus replicating the real kube-apiserver flow
// when applied.
// Nb. Perform ValidateUpdate only if the object is already in the cluster. In all other cases,
// ValidateCreate is performed.
// *Important Note*: We cannot perform defaulting and validation on provider objects as we do not have access to
// that code.
func (t *topologyClient) runDefaultAndValidationWebhooks(ctx context.Context, in *TopologyPlanInput, apiReader client.Reader) error {
	// Enable the ClusterTopology feature gate so that the defaulter and validators do not complain.
	// Note: We don't need to disable it later because the CLI is short lived.
	if err := feature.Gates.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", feature.ClusterTopology, true)); err != nil {
		return errors.Wrapf(err, "failed to enable %s feature gate", feature.ClusterTopology)
	}

	// From the inputs gather all the objects that are not Clusters or ClusterClasses.
	// These objects will be used when initializing a dryrun client to use in the webhooks.
	filteredObjs := filterObjects(
		in.Objs,
		clusterv1.GroupVersion.WithKind("ClusterClass"),
		clusterv1.GroupVersion.WithKind("Cluster"),
	)
	objs := []client.Object{}
	for _, o := range filteredObjs {
		objs = append(objs, o)
	}
	// Creating a dryrun client will a fall back to the apiReader client (client to the underlying Kubernetes cluster)
	// allows the defaulting and validation webhooks to complete actions to could potentially depend on other objects in the cluster.
	// Example: Validation of cluster objects will use the client to read ClusterClasses.
	webhookClient := dryrun.NewClient(apiReader, objs)

	// Run defaulting and validation on ClusterClasses.
	ccWebhook := &webhooks.ClusterClass{Client: webhookClient}
	if err := t.defaultAndValidateObjs(
		ctx,
		getClusterClasses(in.Objs),
		&clusterv1.ClusterClass{},
		ccWebhook,
		ccWebhook,
		apiReader,
	); err != nil {
		return errors.Wrap(err, "failed to run defaulting and validation on ClusterClasses")
	}

	// From the inputs gather all the objects that are not Clusters.
	// These objects will be used when initializing a dryrun client to use in the webhooks.
	// We want to keep ClusterClasses in the webhook client. This is because validation of
	// Cluster objects might need access to ClusterClass objects that are in the input.
	filteredObjs = filterObjects(
		in.Objs,
		clusterv1.GroupVersion.WithKind("Cluster"),
	)
	objs = []client.Object{}
	for _, o := range filteredObjs {
		objs = append(objs, o)
	}
	webhookClient = dryrun.NewClient(apiReader, objs)

	// Run defaulting and validation on Clusters.
	clusterWebhook := &webhooks.Cluster{Client: webhookClient}
	if err := t.defaultAndValidateObjs(
		ctx,
		getClusters(in.Objs),
		&clusterv1.Cluster{},
		clusterWebhook,
		clusterWebhook,
		apiReader,
	); err != nil {
		return errors.Wrap(err, "failed to run defaulting and validation on Clusters")
	}

	return nil
}

func (t *topologyClient) defaultAndValidateObjs(ctx context.Context, objs []*unstructured.Unstructured, o client.Object, defaulter crwebhook.CustomDefaulter, validator crwebhook.CustomValidator, apiReader client.Reader) error {
	for _, obj := range objs {
		// The defaulter and validator need a typed object. Convert the unstructured obj to a typed object.
		object := o.DeepCopyObject().(client.Object) // object here is a typed object.
		if err := localScheme.Convert(obj, object, nil); err != nil {
			return errors.Wrapf(err, "failed to convert object to %s", obj.GetKind())
		}

		// Perform Defaulting
		if err := defaulter.Default(ctx, object); err != nil {
			return errors.Wrapf(err, "failed defaulting of %s %s/%s", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName())
		}

		var oldObject client.Object
		if apiReader != nil {
			tmpObj := o.DeepCopyObject().(client.Object)
			if err := apiReader.Get(ctx, client.ObjectKeyFromObject(obj), tmpObj); err != nil {
				if !apierrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to get object %s %s/%s", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName())
				}
			} else {
				oldObject = tmpObj
			}
		}
		if oldObject != nil {
			if err := validator.ValidateUpdate(ctx, oldObject, object); err != nil {
				return errors.Wrapf(err, "failed validation of %s %s/%s", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName())
			}
		} else {
			if err := validator.ValidateCreate(ctx, object); err != nil {
				return errors.Wrapf(err, "failed validation of %s %s/%s", obj.GroupVersionKind().String(), obj.GetNamespace(), obj.GetName())
			}
		}

		// Converted the defaulted and validated object back into unstructured.
		// Note: This step also makes sure that modified object is updated into the
		// original unstructured object.
		if err := localScheme.Convert(object, obj, nil); err != nil {
			return errors.Wrapf(err, "failed to convert %s to object", obj.GetKind())
		}
	}

	return nil
}

func getClusterClasses(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	res := make([]*unstructured.Unstructured, 0)
	for _, obj := range objs {
		if obj.GroupVersionKind() == clusterv1.GroupVersion.WithKind("ClusterClass") {
			res = append(res, obj)
		}
	}
	return res
}

func getClusters(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	res := make([]*unstructured.Unstructured, 0)
	for _, obj := range objs {
		if obj.GroupVersionKind() == clusterv1.GroupVersion.WithKind("Cluster") {
			res = append(res, obj)
		}
	}
	return res
}

func getTemplates(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	res := make([]*unstructured.Unstructured, 0)
	for _, obj := range objs {
		if strings.HasSuffix(obj.GetKind(), clusterv1.TemplateSuffix) {
			res = append(res, obj)
		}
	}
	return res
}

// generateCRDs creates mock CRD objects for all the provider specific objects in the input.
// These CRD objects will be added to the dry run client for UpdateReferenceAPIContract
// to work as expected.
func (t *topologyClient) generateCRDs(objs []*unstructured.Unstructured) []*apiextensionsv1.CustomResourceDefinition {
	crds := []*apiextensionsv1.CustomResourceDefinition{}
	crdMap := map[string]bool{}
	var gvk schema.GroupVersionKind
	for _, obj := range objs {
		gvk = obj.GroupVersionKind()
		if strings.HasSuffix(gvk.Group, ".cluster.x-k8s.io") && !crdMap[gvk.String()] {
			crd := &apiextensionsv1.CustomResourceDefinition{
				TypeMeta: metav1.TypeMeta{
					APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
					Kind:       "CustomResourceDefinition",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s.%s", flect.Pluralize(strings.ToLower(gvk.Kind)), gvk.Group),
					Labels: map[string]string{
						clusterv1.GroupVersion.String(): clusterv1.GroupVersion.Version,
					},
				},
			}
			crds = append(crds, crd)
			crdMap[gvk.String()] = true
		}
	}
	return crds
}

func (t *topologyClient) affectedClusterClasses(ctx context.Context, in *TopologyPlanInput, c client.Reader) ([]client.ObjectKey, error) {
	affectedClusterClasses := map[client.ObjectKey]bool{}
	ccList := &clusterv1.ClusterClassList{}
	if err := c.List(
		ctx,
		ccList,
		client.InNamespace(in.TargetNamespace),
	); err != nil {
		return nil, errors.Wrapf(err, "failed to list ClusterClasses in namespace %s", in.TargetNamespace)
	}

	// Each of the ClusterClass that uses any of the Templates in the input is an affected ClusterClass.
	for _, template := range getTemplates(in.Objs) {
		for i := range ccList.Items {
			if clusterClassUsesTemplate(&ccList.Items[i], objToRef(template)) {
				affectedClusterClasses[client.ObjectKeyFromObject(&ccList.Items[i])] = true
			}
		}
	}

	// All the ClusterClasses in the input are considered affected ClusterClasses.
	for _, cc := range getClusterClasses(in.Objs) {
		affectedClusterClasses[client.ObjectKeyFromObject(cc)] = true
	}

	affectedClusterClassesList := []client.ObjectKey{}
	for k := range affectedClusterClasses {
		affectedClusterClassesList = append(affectedClusterClassesList, k)
	}
	return affectedClusterClassesList, nil
}

func (t *topologyClient) affectedClusters(ctx context.Context, in *TopologyPlanInput, c client.Reader) ([]client.ObjectKey, error) {
	affectedClusters := map[client.ObjectKey]bool{}
	affectedClusterClasses, err := t.affectedClusterClasses(ctx, in, c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get list of affected ClusterClasses")
	}
	clusterList := &clusterv1.ClusterList{}
	if err := c.List(
		ctx,
		clusterList,
		client.InNamespace(in.TargetNamespace),
	); err != nil {
		return nil, errors.Wrapf(err, "failed to list Clusters in namespace %s", in.TargetNamespace)
	}

	// Each of the Cluster that uses the ClusterClass in the input is an affected cluster.
	for _, cc := range affectedClusterClasses {
		for i := range clusterList.Items {
			if clusterList.Items[i].Spec.Topology != nil && clusterList.Items[i].Spec.Topology.Class == cc.Name {
				affectedClusters[client.ObjectKeyFromObject(&clusterList.Items[i])] = true
			}
		}
	}

	// All the Clusters in the input are considered affected Clusters.
	for _, cluster := range getClusters(in.Objs) {
		affectedClusters[client.ObjectKeyFromObject(cluster)] = true
	}

	affectedClustersList := []client.ObjectKey{}
	for k := range affectedClusters {
		affectedClustersList = append(affectedClustersList, k)
	}
	return affectedClustersList, nil
}

func inList(list []client.ObjectKey, target client.ObjectKey) bool {
	for _, i := range list {
		if i == target {
			return true
		}
	}
	return false
}

// filterObjects returns a new list of objects after dropping all the objects that match any of the given GVKs.
func filterObjects(objs []*unstructured.Unstructured, gvks ...schema.GroupVersionKind) []*unstructured.Unstructured {
	res := []*unstructured.Unstructured{}
	for _, o := range objs {
		skip := false
		for _, gvk := range gvks {
			if o.GroupVersionKind() == gvk {
				skip = true
				break
			}
		}
		if !skip {
			res = append(res, o)
		}
	}
	return res
}

type noOpRecorder struct{}

func (nr *noOpRecorder) Event(_ runtime.Object, _, _, _ string)                       {}
func (nr *noOpRecorder) Eventf(_ runtime.Object, _, _, _ string, args ...interface{}) {}
func (nr *noOpRecorder) AnnotatedEventf(_ runtime.Object, _ map[string]string, _, _, _ string, args ...interface{}) {
}

func objToRef(o *unstructured.Unstructured) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind:       o.GetKind(),
		Namespace:  o.GetNamespace(),
		Name:       o.GetName(),
		APIVersion: o.GetAPIVersion(),
	}
}

func equalRef(a, b *corev1.ObjectReference) bool {
	if a.APIVersion != b.APIVersion {
		return false
	}
	if a.Namespace != b.Namespace {
		return false
	}
	if a.Name != b.Name {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	return true
}

func clusterClassUsesTemplate(cc *clusterv1.ClusterClass, templateRef *corev1.ObjectReference) bool {
	// Check infrastructure ref.
	if equalRef(cc.Spec.Infrastructure.Ref, templateRef) {
		return true
	}
	// Check control plane ref.
	if equalRef(cc.Spec.ControlPlane.Ref, templateRef) {
		return true
	}
	// If control plane uses machine, check it.
	if cc.Spec.ControlPlane.MachineInfrastructure != nil && cc.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		if equalRef(cc.Spec.ControlPlane.MachineInfrastructure.Ref, templateRef) {
			return true
		}
	}

	for _, mdClass := range cc.Spec.Workers.MachineDeployments {
		// Check bootstrap template ref.
		if equalRef(mdClass.Template.Bootstrap.Ref, templateRef) {
			return true
		}
		// Check the infrastructure ref.
		if equalRef(mdClass.Template.Infrastructure.Ref, templateRef) {
			return true
		}
	}

	return false
}

func uniqueNamespaces(objs []*unstructured.Unstructured) []string {
	ns := sets.NewString()
	for _, obj := range objs {
		// Namespace objects do not have metadata.namespace set, but we can add the
		// name of the obj to the namespace list, as it is another unique namespace.
		isNamespace := obj.GroupVersionKind().Kind == namespaceKind
		if isNamespace {
			ns.Insert(obj.GetName())
			continue
		}

		// Note: treat empty namespace (namespace not set) as a unique namespace.
		// If some have a namespace set and some do not. It is safer to consider them as
		// objects from different namespaces.
		ns.Insert(obj.GetNamespace())
	}
	return ns.List()
}

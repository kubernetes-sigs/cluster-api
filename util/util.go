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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// CharSet defines the alphanumeric set for random string generation
	CharSet = "0123456789abcdefghijklmnopqrstuvwxyz"
	// MachineListFormatDeprecationMessage notifies the user that the old
	// MachineList format is no longer supported
	MachineListFormatDeprecationMessage = "Your MachineList items must include Kind and APIVersion"
)

var (
	rnd                          = rand.New(rand.NewSource(time.Now().UnixNano()))
	ErrNoCluster                 = fmt.Errorf("no %q label present", clusterv1.ClusterLabelName)
	ErrUnstructuredFieldNotFound = fmt.Errorf("field not found")
)

// RandomString returns a random alphanumeric string.
func RandomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = CharSet[rnd.Intn(len(CharSet))]
	}
	return string(result)
}

// GetMachinesForCluster returns a list of machines associated with the cluster.
func GetMachinesForCluster(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (*clusterv1.MachineList, error) {
	var machines clusterv1.MachineList
	if err := c.List(
		ctx,
		&machines,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		},
	); err != nil {
		return nil, err
	}
	return &machines, nil
}

// GetControlPlaneMachines returns a slice containing control plane machines.
func GetControlPlaneMachines(machines []*clusterv1.Machine) (res []*clusterv1.Machine) {
	for _, machine := range machines {
		if IsControlPlaneMachine(machine) {
			res = append(res, machine)
		}
	}
	return
}

// GetControlPlaneMachinesFromList returns a slice containing control plane machines.
func GetControlPlaneMachinesFromList(machineList *clusterv1.MachineList) (res []*clusterv1.Machine) {
	for i := 0; i < len(machineList.Items); i++ {
		machine := machineList.Items[i]
		if IsControlPlaneMachine(&machine) {
			res = append(res, &machine)
		}
	}
	return
}

// GetMachineIfExists gets a machine from the API server if it exists
func GetMachineIfExists(c client.Client, namespace, name string) (*clusterv1.Machine, error) {
	if c == nil {
		// Being called before k8s is setup as part of control plane VM creation
		return nil, nil
	}

	// Machines are identified by name
	machine := &clusterv1.Machine{}
	err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return machine, nil
}

// IsControlPlaneMachine checks machine is a control plane node.
func IsControlPlaneMachine(machine *clusterv1.Machine) bool {
	_, ok := machine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName]
	return ok
}

// IsNodeReady returns true if a node is ready.
func IsNodeReady(node *v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return condition.Status == v1.ConditionTrue
		}
	}

	return false
}

// GetClusterFromMetadata returns the Cluster object (if present) using the object metadata.
func GetClusterFromMetadata(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Cluster, error) {
	if obj.Labels[clusterv1.ClusterLabelName] == "" {
		return nil, errors.WithStack(ErrNoCluster)
	}
	return GetClusterByName(ctx, c, obj.Namespace, obj.Labels[clusterv1.ClusterLabelName])
}

// GetOwnerCluster returns the Cluster object owning the current resource.
func GetOwnerCluster(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Cluster, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind == "Cluster" && ref.APIVersion == clusterv1.GroupVersion.String() {
			return GetClusterByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetClusterByName finds and return a Cluster object using the specified params.
func GetClusterByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Cluster, error) {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}

	if err := c.Get(ctx, key, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}

// ObjectKey returns client.ObjectKey for the object
func ObjectKey(object metav1.Object) client.ObjectKey {
	return client.ObjectKey{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

// ClusterToInfrastructureMapFunc returns a handler.ToRequestsFunc that watches for
// Cluster events and returns reconciliation requests for an infrastructure provider object.
func ClusterToInfrastructureMapFunc(gvk schema.GroupVersionKind) handler.ToRequestsFunc {
	return func(o handler.MapObject) []reconcile.Request {
		c, ok := o.Object.(*clusterv1.Cluster)
		if !ok {
			return nil
		}

		// Return early if the InfrastructureRef is nil.
		if c.Spec.InfrastructureRef == nil {
			return nil
		}

		// Return early if the GroupVersionKind doesn't match what we expect.
		infraGVK := c.Spec.InfrastructureRef.GroupVersionKind()
		if gvk != infraGVK {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: c.Namespace,
					Name:      c.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// GetOwnerMachine returns the Machine object owning the current resource.
func GetOwnerMachine(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*clusterv1.Machine, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind == "Machine" && ref.APIVersion == clusterv1.GroupVersion.String() {
			return GetMachineByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// GetMachineByName finds and return a Machine object using the specified params.
func GetMachineByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Machine, error) {
	m := &clusterv1.Machine{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// MachineToInfrastructureMapFunc returns a handler.ToRequestsFunc that watches for
// Machine events and returns reconciliation requests for an infrastructure provider object.
func MachineToInfrastructureMapFunc(gvk schema.GroupVersionKind) handler.ToRequestsFunc {
	return func(o handler.MapObject) []reconcile.Request {
		m, ok := o.Object.(*clusterv1.Machine)
		if !ok {
			return nil
		}

		// Return early if the GroupVersionKind doesn't match what we expect.
		infraGVK := m.Spec.InfrastructureRef.GroupVersionKind()
		if gvk != infraGVK {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: m.Namespace,
					Name:      m.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// HasOwnerRef returns true if the OwnerReference is already in the slice.
func HasOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) bool {
	return indexOwnerRef(ownerReferences, ref) > -1
}

// EnsureOwnerRef makes sure the slice contains the OwnerReference.
func EnsureOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	idx := indexOwnerRef(ownerReferences, ref)
	if idx == -1 {
		return append(ownerReferences, ref)
	}
	ownerReferences[idx] = ref
	return ownerReferences
}

// indexOwnerRef returns the index of the owner reference in the slice if found, or -1.
func indexOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) int {
	for index, r := range ownerReferences {
		if referSameObject(r, ref) {
			return index
		}
	}
	return -1
}

// Returns true if a and b point to the same object.
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV.Group == bGV.Group && a.Kind == b.Kind && a.Name == b.Name
}

// PointsTo returns true if any of the owner references point to the given target
func PointsTo(refs []metav1.OwnerReference, target *metav1.ObjectMeta) bool {
	for _, ref := range refs {
		if ref.UID == target.UID {
			return true
		}
	}

	return false
}

// UnstructuredUnmarshalField is a wrapper around json and unstructured objects to decode and copy a specific field
// value into an object.
func UnstructuredUnmarshalField(obj *unstructured.Unstructured, v interface{}, fields ...string) error {
	value, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve field %q from %q", strings.Join(fields, "."), obj.GroupVersionKind())
	}
	if !found || value == nil {
		return ErrUnstructuredFieldNotFound
	}
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return errors.Wrapf(err, "failed to json-encode field %q value from %q", strings.Join(fields, "."), obj.GroupVersionKind())
	}
	if err := json.Unmarshal(valueBytes, v); err != nil {
		return errors.Wrapf(err, "failed to json-decode field %q value from %q", strings.Join(fields, "."), obj.GroupVersionKind())
	}
	return nil
}

// HasOwner checks if any of the references in the passed list match the given apiVersion and one of the given kinds
func HasOwner(refList []metav1.OwnerReference, apiVersion string, kinds []string) bool {
	kMap := make(map[string]bool)
	for _, kind := range kinds {
		kMap[kind] = true
	}

	for _, mr := range refList {
		if mr.APIVersion == apiVersion && kMap[mr.Kind] {
			return true
		}
	}

	return false
}

// IsPaused returns true if the Cluster is paused or the object has the `paused` annotation.
func IsPaused(cluster *clusterv1.Cluster, v metav1.Object) bool {
	if cluster.Spec.Paused {
		return true
	}

	annotations := v.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, ok := annotations[clusterv1.PausedAnnotation]
	return ok
}

// GetCRDWithContract retrieves a list of CustomResourceDefinitions from using controller-runtime Client,
// filtering with the `contract` label passed in.
// Returns the first CRD in the list that matches the GroupVersionKind, otherwise returns an error.
func GetCRDWithContract(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, contract string) (*apiextensionsv1.CustomResourceDefinition, error) {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	for {
		if err := c.List(ctx, crdList, client.Continue(crdList.Continue), client.HasLabels{contract}); err != nil {
			return nil, errors.Wrapf(err, "failed to list CustomResourceDefinitions for %v", gvk)
		}

		for _, crd := range crdList.Items {
			if crd.Spec.Group == gvk.Group &&
				crd.Spec.Names.Kind == gvk.Kind {
				return crd.DeepCopy(), nil
			}
		}

		if crdList.Continue == "" {
			break
		}
	}

	return nil, errors.Errorf("failed to find a CustomResourceDefinition for %v with contract %q", gvk, contract)
}

// KubeAwareAPIVersions is a sortable slice of kube-like version strings.
//
// Kube-like version strings are starting with a v, followed by a major version,
// optional "alpha" or "beta" strings followed by a minor version (e.g. v1, v2beta1).
// Versions will be sorted based on GA/alpha/beta first and then major and minor
// versions. e.g. v2, v1, v1beta2, v1beta1, v1alpha1.
type KubeAwareAPIVersions []string

func (k KubeAwareAPIVersions) Len() int      { return len(k) }
func (k KubeAwareAPIVersions) Swap(i, j int) { k[i], k[j] = k[j], k[i] }
func (k KubeAwareAPIVersions) Less(i, j int) bool {
	return version.CompareKubeAwareVersionStrings(k[i], k[j]) < 0
}

// MachinesByCreationTimestamp sorts a list of Machine by creation timestamp, using their names as a tie breaker.
type MachinesByCreationTimestamp []*clusterv1.Machine

func (o MachinesByCreationTimestamp) Len() int      { return len(o) }
func (o MachinesByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o MachinesByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
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
	ErrNoCluster                 = fmt.Errorf("no %q label present", clusterv1.MachineClusterLabelName)
	ErrUnstructuredFieldNotFound = fmt.Errorf("field not found")
)

func init() {
	clusterv1.AddToScheme(scheme.Scheme)
}

// RandomToken returns a random token.
func RandomToken() string {
	return fmt.Sprintf("%s.%s", RandomString(6), RandomString(16))
}

// RandomString returns a random alphanumeric string.
func RandomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = CharSet[rnd.Intn(len(CharSet))]
	}
	return string(result)
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
	for _, machine := range machineList.Items {
		if IsControlPlaneMachine(&machine) {
			res = append(res, &machine)
		}
	}
	return
}

// Home returns the user home directory.
func Home() string {
	home := os.Getenv("HOME")
	if strings.Contains(home, "root") {
		return "/root"
	}

	usr, err := user.Current()
	if err != nil {
		klog.Warningf("unable to find user: %v", err)
		return ""
	}
	return usr.HomeDir
}

// GetDefaultKubeConfigPath returns the standard user kubeconfig
func GetDefaultKubeConfigPath() string {
	localDir := fmt.Sprintf("%s/.kube", Home())
	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		if err := os.Mkdir(localDir, 0777); err != nil {
			klog.Fatal(err)
		}
	}
	return fmt.Sprintf("%s/config", localDir)
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
	return machine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName] != ""
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
	if obj.Labels[clusterv1.MachineClusterLabelName] == "" {
		return nil, errors.WithStack(ErrNoCluster)
	}
	return GetClusterByName(ctx, c, obj.Namespace, obj.Labels[clusterv1.MachineClusterLabelName])
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
	for _, r := range ownerReferences {
		if reflect.DeepEqual(ref, r) {
			return true
		}
	}
	return false
}

// EnsureOwnerRef makes sure the slice contains the OwnerReference.
func EnsureOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	if !HasOwnerRef(ownerReferences, ref) {
		return append(ownerReferences, ref)
	}
	return ownerReferences
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

// Copy deep copies a Machine object.
func Copy(m *clusterv1.Machine) *clusterv1.Machine {
	ret := &clusterv1.Machine{}
	ret.APIVersion = m.APIVersion
	ret.Kind = m.Kind
	ret.ClusterName = m.ClusterName
	ret.GenerateName = m.GenerateName
	ret.Name = m.Name
	ret.Namespace = m.Namespace
	m.Spec.DeepCopyInto(&ret.Spec)
	return ret
}

// ExecCommand Executes a local command in the current shell.
func ExecCommand(name string, args ...string) (string, error) {
	cmdOut, err := exec.Command(name, args...).Output()
	if err != nil {
		s := strings.Join(append([]string{name}, args...), " ")
		klog.Infof("Executing command %q: %v", s, err)
		return string(""), err
	}
	return string(cmdOut), nil
}

// Filter filters a list for a string.
func Filter(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}

// Contains returns true if a list contains a string.
func Contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}

// GetNamespaceOrDefault returns the default namespace if given empty
// output.
func GetNamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return v1.NamespaceDefault
	}
	return namespace
}

// ParseClusterYaml parses a YAML file for cluster objects.
func ParseClusterYaml(file string) (*clusterv1.Cluster, error) {
	reader, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	decoder := NewYAMLDecoder(reader)

	for {
		var cluster clusterv1.Cluster
		_, gvk, err := decoder.Decode(nil, &cluster)
		if err == io.EOF {
			break
		}

		if runtime.IsNotRegisteredError(err) {
			continue
		}

		if err != nil {
			return nil, err
		}

		if gvk.Kind == "Cluster" {
			return &cluster, nil
		}
	}

	return nil, errors.New("Did not find a cluster")

}

// ParseMachinesYaml extracts machine objects from a file.
func ParseMachinesYaml(file string) ([]*clusterv1.Machine, error) {
	reader, err := os.Open(file)

	if err != nil {
		return nil, err
	}

	var (
		machines []*clusterv1.Machine
	)

	decoder := NewYAMLDecoder(reader)
	defer decoder.Close()

	for {
		obj, gvk, err := decoder.Decode(nil, nil)

		if err == io.EOF {
			break
		}

		if runtime.IsNotRegisteredError(err) {
			continue
		}

		if err != nil {
			return nil, err
		}

		switch gvk.Kind {

		case "MachineList":
			ml, ok := obj.(*clusterv1.MachineList)
			if !ok {
				return nil, fmt.Errorf("Expected MachineList, got %t", obj)
			}

			for i := range ml.Items {
				machine := &ml.Items[i]
				if machine.APIVersion == "" || machine.Kind == "" {
					return nil, errors.New(MachineListFormatDeprecationMessage)
				}
				machines = append(machines, machine)
			}
		case "Machine":
			m, ok := obj.(*clusterv1.Machine)
			if !ok {
				return nil, fmt.Errorf("Expected Machine, got %t", obj)
			}

			machines = append(machines, m)
		}

	}

	return machines, nil
}

// isMissingKind reimplements runtime.IsMissingKind as the YAMLOrJSONDecoder
// hides the error type.
func isMissingKind(err error) bool {
	return strings.Contains(err.Error(), "Object 'Kind' is missing in")
}

type yamlDecoder struct {
	reader  *yaml.YAMLReader
	decoder runtime.Decoder
	close   func() error
}

func (d *yamlDecoder) Decode(defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	for {
		doc, err := d.reader.Read()
		if err != nil {
			return nil, nil, err
		}

		//  Skip over empty documents, i.e. a leading `---`
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		return d.decoder.Decode(doc, defaults, into)
	}

}

func (d *yamlDecoder) Close() error {
	return d.close()
}

func NewYAMLDecoder(r io.ReadCloser) streaming.Decoder {
	return &yamlDecoder{
		reader:  yaml.NewYAMLReader(bufio.NewReader(r)),
		decoder: scheme.Codecs.UniversalDeserializer(),
		close:   r.Close,
	}
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

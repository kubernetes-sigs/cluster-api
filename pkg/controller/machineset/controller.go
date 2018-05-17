/*
Copyright 2018 The Kubernetes Authors.

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

package machineset

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterapiclientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	machinesetclientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
)

const (
	// The number of times we retry updating a ReplicaSet's status.
	statusUpdateRetries = 1
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = v1alpha1.SchemeGroupVersion.WithKind("MachineSet")

// +controller:group=cluster,version=v1alpha1,kind=MachineSet,resource=machinesets
type MachineSetControllerImpl struct {
	builders.DefaultControllerFns

	// kubernetesClient a client that knows how to consume Node resources
	kubernetesClient kubernetes.Interface

	// clusterAPIClient a client that knows how to consume Cluster API resources
	clusterAPIClient clusterapiclientset.Interface

	// machineSetsLister indexes properties about MachineSet
	machineSetsLister listers.MachineSetLister

	// machineLister holds a lister that knows how to list Machines from a cache
	machineLister listers.MachineLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineSetControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	c.kubernetesClient = arguments.GetSharedInformers().KubernetesClientSet

	c.machineSetsLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineSets().Lister()
	c.machineLister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()

	var err error
	c.clusterAPIClient, err = clusterapiclientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error building clientset for clusterAPIClient: %v", err)
	}
}

// Reconcile holds the controller's business logic.
// it makes sure that the current state is equal to the desired state.
// note that the current state of the cluster is calculated based on the number of machines
// that are owned by the given machineSet (key).
func (c *MachineSetControllerImpl) Reconcile(machineSet *v1alpha1.MachineSet) error {
	filteredMachines, err := c.getMachines(machineSet)
	if err != nil {
		return err
	}

	// Take ownership of machines if not already owned.
	for _, machine := range filteredMachines {
		if shouldAdopt(machineSet, machine) {
			c.adoptOrphan(machineSet, machine)
		}
	}

	var manageReplicasErr error
	if machineSet.DeletionTimestamp == nil {
		manageReplicasErr = c.manageReplicas(filteredMachines, machineSet)
	}
	ms := machineSet.DeepCopy()
	newStatus := c.calculateStatus(ms, filteredMachines)

	// Always updates status as machines come up or die.
	updatedMS, err := updateMachineSetStatus(c.clusterAPIClient.ClusterV1alpha1().MachineSets(machineSet.Namespace), machineSet, newStatus)
	if err != nil {
		// Multiple things could lead to this update failing. Requeuing the replica set ensures
		// Returning an error causes a requeue without forcing a hotloop
		return fmt.Errorf("failed to update machine set status, %v", err)
	}
	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if manageReplicasErr == nil && updatedMS.Spec.MinReadySeconds > 0 &&
		updatedMS.Status.ReadyReplicas == *(updatedMS.Spec.Replicas) &&
		updatedMS.Status.AvailableReplicas != *(updatedMS.Spec.Replicas) {
		c.reconcileAfter(updatedMS, time.Duration(updatedMS.Spec.MinReadySeconds)*time.Second)
	}
	return manageReplicasErr
}

func (c *MachineSetControllerImpl) Get(namespace, name string) (*v1alpha1.MachineSet, error) {
	return c.machineSetsLister.MachineSets(namespace).Get(name)
}

// createMachine creates a machine resource.
// the name of the newly created resource is going to be created by the API server, we set the generateName field
func (c *MachineSetControllerImpl) createMachine(machineSet *v1alpha1.MachineSet) *v1alpha1.Machine {
	gv := v1alpha1.SchemeGroupVersion
	machine := &v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       gv.WithKind("Machine").Kind,
			APIVersion: gv.String(),
		},
		ObjectMeta: machineSet.Spec.Template.ObjectMeta,
		Spec:       machineSet.Spec.Template.Spec,
	}
	machine.ObjectMeta.GenerateName = fmt.Sprintf("%s-", machineSet.Name)
	machine.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(machineSet, controllerKind)}

	return machine
}

// getMachines returns a list of machines that match on machineSet.Spec.Selector
func (c *MachineSetControllerImpl) getMachines(machineSet *v1alpha1.MachineSet) ([]*v1alpha1.Machine, error) {
	// list all machines to include the machines that don't match the machineSet`s selector
	// anymore but has the stale controller ref.
	// TODO: Do the List and Filter in a single pass, or use an index.
	allMachines, err := c.machineLister.Machines(machineSet.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var filteredMachines []*v1alpha1.Machine
	for _, machine := range allMachines {
		// Ignore inactive machines.
		if machine.DeletionTimestamp != nil {
			continue
		}

		if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
			glog.V(4).Infof("%s not controlled by %v", machine.Name, machineSet.Name)
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
		if err != nil {
			glog.Warningf("unable to convert selector: %v", err)
			continue
		}
		// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(machine.Labels)) {
			continue
		}
		filteredMachines = append(filteredMachines, machine)
	}
	return filteredMachines, err
}

func shouldAdopt(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	// Do nothing if the machine is being deleted.
	if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
		glog.V(2).Infof("Skipping machine (%v), as it is being deleted.", machine.Name)
		return false
	}

	// Machine owned by another controller.
	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		glog.Warningf("Skipping machine (%v), as it is owned by someone else.", machine.Name)
		return false
	}

	// Machine we control.
	if metav1.IsControlledBy(machine, machineSet) {
		return false
	}

	return true
}

func (c *MachineSetControllerImpl) adoptOrphan(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) {
	// Add controller reference.
	ownerRefs := machine.ObjectMeta.GetOwnerReferences()
	if ownerRefs == nil {
		ownerRefs = []metav1.OwnerReference{}
	}

	newRef := *metav1.NewControllerRef(machineSet, controllerKind)
	ownerRefs = append(ownerRefs, newRef)
	machine.ObjectMeta.SetOwnerReferences(ownerRefs)

	if _, err := c.clusterAPIClient.ClusterV1alpha1().Machines(machineSet.Namespace).Update(machine); err != nil {
		glog.Warningf("Failed to update machine owner reference. %v", err)
	}
}

// manageReplicas checks and updates replicas for the given MachineSet.
// Does NOT modify <filteredMachines>.
// It will requeue the replica set in case of an error while creating/deleting machines.
func (c *MachineSetControllerImpl) manageReplicas(filteredMachines []*v1alpha1.Machine, ms *v1alpha1.MachineSet) error {
	diff := len(filteredMachines) - int(*(ms.Spec.Replicas))
	if diff < 0 {
		diff *= -1
		glog.V(4).Infof("Too few replicas for %v %s/%s, need %d, creating %d", controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		var errstrings []string
		for i := 0; i < diff; i++ {
			glog.V(4).Infof("creating a machine ( spec.replicas(%d) > currentMachineCount(%d) )", *(ms.Spec.Replicas), len(filteredMachines))
			machine := c.createMachine(ms)
			_, err := c.clusterAPIClient.ClusterV1alpha1().Machines(ms.Namespace).Create(machine)
			if err != nil {
				glog.Errorf("unable to create a machine = %s, due to %v", machine.Name, err)
				errstrings = append(errstrings, err.Error())
			}
		}

		if errstrings != nil {
			return fmt.Errorf(strings.Join(errstrings, "; "))
		}

		return nil
	} else if diff > 0 {
		glog.V(4).Infof("Too many replicas for %v %s/%s, need %d, deleting %d", controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		// Choose which Machines to delete.
		machinesToDelete := getMachinesToDelete(filteredMachines, diff)

		errCh := make(chan error, diff)
		var wg sync.WaitGroup
		wg.Add(diff)
		for _, machine := range machinesToDelete {
			go func(targetMachine *v1alpha1.Machine) {
				defer wg.Done()
				err := c.clusterAPIClient.ClusterV1alpha1().Machines(ms.Namespace).Delete(targetMachine.Name, &metav1.DeleteOptions{})
				if err != nil {
					glog.Errorf("unable to delete a machine = %s, due to %v", machine.Name, err)
					errCh <- err
				}
			}(machine)
		}
		wg.Wait()

		select {
		case err := <-errCh:
			// all errors have been reported before and they're likely to be the same, so we'll only return the first one we hit.
			if err != nil {
				return err
			}
		default:
		}
	}

	return nil
}

func (c *MachineSetControllerImpl) reconcileAfter(machineSet *v1alpha1.MachineSet, after time.Duration) {
	timer := time.NewTimer(after)
	go func(newMS v1alpha1.MachineSet) {
		<-timer.C
		c.Reconcile(&newMS)
	}(*machineSet)
}

// updateMachineSetStatus attempts to update the Status.Replicas of the given MachineSet, with a single GET/PUT retry.
func updateMachineSetStatus(c machinesetclientset.MachineSetInterface, ms *v1alpha1.MachineSet, newStatus v1alpha1.MachineSetStatus) (*v1alpha1.MachineSet, error) {
	// This is the steady state. It happens when the MachineSet doesn't have any expectations, since
	// we do a periodic relist every 30s. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if ms.Status.Replicas == newStatus.Replicas &&
		ms.Status.FullyLabeledReplicas == newStatus.FullyLabeledReplicas &&
		ms.Status.ReadyReplicas == newStatus.ReadyReplicas &&
		ms.Status.AvailableReplicas == newStatus.AvailableReplicas &&
		ms.Generation == ms.Status.ObservedGeneration {
		return ms, nil
	}

	// Save the generation number we acted on, otherwise we might wrongfully indicate
	// that we've seen a spec update when we retry.
	// TODO: This can clobber an update if we allow multiple agents to write to the
	// same status.
	newStatus.ObservedGeneration = ms.Generation

	var getErr, updateErr error
	var updatedMS *v1alpha1.MachineSet
	for i := 0; ; i++ {
		glog.V(4).Infof(fmt.Sprintf("Updating status for %v: %s/%s, ", ms.Kind, ms.Namespace, ms.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", ms.Status.Replicas, newStatus.Replicas, *(ms.Spec.Replicas)) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", ms.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", ms.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", ms.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", ms.Status.ObservedGeneration, newStatus.ObservedGeneration))

		ms.Status = newStatus
		updatedMS, updateErr = c.UpdateStatus(ms)
		if updateErr == nil {
			return updatedMS, nil
		}
		// Stop retrying if we exceed statusUpdateRetries - the machineSet will be requeued with a rate limit.
		if i >= statusUpdateRetries {
			break
		}
		// Update the MachineSet with the latest resource version for the next poll
		if ms, getErr = c.Get(ms.Name, metav1.GetOptions{}); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			return nil, getErr
		}
	}

	return nil, updateErr
}

func (c *MachineSetControllerImpl) calculateStatus(ms *v1alpha1.MachineSet, filteredMachines []*v1alpha1.Machine) v1alpha1.MachineSetStatus {
	newStatus := ms.Status
	// Count the number of machines that have labels matching the labels of the machine
	// template of the replica set, the matching machines may have more
	// labels than are in the template. Because the label of machineTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching machines must be part of the filteredMachines.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	availableReplicasCount := 0
	templateLabel := labels.Set(ms.Spec.Template.Labels).AsSelectorPreValidated()
	for _, machine := range filteredMachines {
		if templateLabel.Matches(labels.Set(machine.Labels)) {
			fullyLabeledReplicasCount++
		}
		node, err := c.getMachineNode(machine)
		if err != nil {
			glog.Warningf("Unable to get node for machine %v, %v", machine.Name, err)
			continue
		}
		if isNodeReady(node) {
			readyReplicasCount++
			if isNodeAvailable(node, ms.Spec.MinReadySeconds, metav1.Now()) {
				availableReplicasCount++
			}
		}
	}

	newStatus.Replicas = int32(len(filteredMachines))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)
	return newStatus
}

func isNodeAvailable(node *corev1.Node, minReadySeconds int32, now metav1.Time) bool {
	if !isNodeReady(node) {
		return false
	}

	if minReadySeconds == 0 {
		return true
	}

	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	_, readyCondition := getNodeCondition(&node.Status, corev1.NodeReady)

	if !readyCondition.LastTransitionTime.IsZero() &&
		readyCondition.LastTransitionTime.Add(minReadySecondsDuration).Before(now.Time) {
		return true
	}

	return false
}

func (c *MachineSetControllerImpl) getMachineNode(machine *v1alpha1.Machine) (*corev1.Node, error) {
	nodeRef := machine.Status.NodeRef
	if nodeRef == nil {
		return nil, fmt.Errorf("machine has no node ref")
	}

	return c.kubernetesClient.CoreV1().Nodes().Get(nodeRef.Name, metav1.GetOptions{})
}

// getNodeCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getNodeCondition(status *corev1.NodeStatus, conditionType corev1.NodeConditionType) (int, *corev1.NodeCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// isNodeReady returns true if a node is ready; false otherwise.
func isNodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func getMachinesToDelete(filteredMachines []*v1alpha1.Machine, diff int) []*v1alpha1.Machine {
	// TODO: Define machines deletion policies.
	// see: https://github.com/kubernetes/kube-deploy/issues/625
	return filteredMachines[:diff]
}

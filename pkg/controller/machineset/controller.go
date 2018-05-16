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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterapiclientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
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

	queue workqueue.RateLimitingInterface
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

	c.queue = arguments.GetSharedInformers().WorkerQueues["MachineSet"].Queue
}

// Reconcile holds the controller's business logic.
// it makes sure that the current state is equal to the desired state.
// note that the current state of the cluster is calculated based on the number of machines
// that are owned by the given machineSet (key).
func (c *MachineSetControllerImpl) Reconcile(machineSet *v1alpha1.MachineSet) error {
	allMachines, err := c.machineLister.Machines(machineSet.Namespace).List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list machines, %v", err)
	}

	// Filter out irrelevant machines (deleting/mismatch labels) and claim orphaned machines.
	var filteredMachines []*v1alpha1.Machine
	for _, machine := range allMachines {
		if c.shouldExcludeMachine(machineSet, machine) {
			continue
		}
		// Attempt to adopt machine if it meets previous conditions and it has no controller ref.
		if metav1.GetControllerOf(machine) == nil {
			if err := c.adoptOrphan(machineSet, machine); err != nil {
				glog.V(4).Infof("failed to adopt machine %v into machineset %v. %v", machine.Name, machineSet.Name, err)
				continue
			}
		}
		filteredMachines = append(filteredMachines, machine)
	}

	syncErr := c.syncReplicas(machineSet, filteredMachines)

	ms := machineSet.DeepCopy()
	newStatus := c.calculateStatus(ms, filteredMachines)

	// Always updates status as machines come up or die.
	updatedMS, err := updateMachineSetStatus(c.clusterAPIClient.ClusterV1alpha1().MachineSets(machineSet.Namespace), machineSet, newStatus)
	if err != nil {
		if syncErr != nil {
			return fmt.Errorf("failed to sync machines. %v. failed to update machine set status. %v", syncErr, err)
		}
		return fmt.Errorf("failed to update machine set status. %v", err)
	}
	if updatedMS.Spec.Replicas == nil {
		return fmt.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed.", ms.Name)
	}

	// Resync the MachineSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	// Clock-skew is an issue as it may impact whether an available replica is counted as a ready replica.
	// A replica is available if the amount of time since last transition exceeds MinReadySeconds.
	// If there was a clock skew, checking whether the amount of time since last transition to ready state
	// exceeds MinReadySeconds could be incorrect.
	// To avoid an available replica stuck in the ready state, we force a reconcile after MinReadySeconds,
	// at which point it should confirm any available replica to be available.
	if syncErr == nil && updatedMS.Spec.MinReadySeconds > 0 &&
		updatedMS.Status.ReadyReplicas == *(updatedMS.Spec.Replicas) &&
		updatedMS.Status.AvailableReplicas != *(updatedMS.Spec.Replicas) {

		if err := c.enqueueAfter(updatedMS, time.Duration(updatedMS.Spec.MinReadySeconds)*time.Second); err != nil {
			return fmt.Errorf("failed to enqueue %v machineset for later. %v", updatedMS.Name, err)
		}
	}
	return syncErr
}

func (c *MachineSetControllerImpl) Get(namespace, name string) (*v1alpha1.MachineSet, error) {
	return c.machineSetsLister.MachineSets(namespace).Get(name)
}

// syncReplicas essentially scales machine resources up and down.
func (c *MachineSetControllerImpl) syncReplicas(ms *v1alpha1.MachineSet, machines []*v1alpha1.Machine) error {
	if ms.Spec.Replicas == nil {
		return fmt.Errorf("the Replicas field in Spec for machineset %v is nil, this should not be allowed.", ms.Name)
	}
	diff := len(machines) - int(*(ms.Spec.Replicas))
	if diff < 0 {
		diff *= -1
		glog.V(4).Infof("Too few replicas for %v %s/%s, need %d, creating %d", controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		var errstrings []string
		for i := 0; i < diff; i++ {
			glog.V(2).Infof("creating a machine ( spec.replicas(%d) > currentMachineCount(%d) )", *(ms.Spec.Replicas), len(machines))
			machine := c.createMachine(ms)
			_, err := c.clusterAPIClient.ClusterV1alpha1().Machines(ms.Namespace).Create(machine)
			if err != nil {
				glog.Errorf("unable to create a machine = %s, due to %v", machine.Name, err)
				errstrings = append(errstrings, err.Error())
			}
		}

		if len(errstrings) > 0 {
			return fmt.Errorf(strings.Join(errstrings, "; "))
		}

		return nil
	} else if diff > 0 {
		glog.V(4).Infof("Too many replicas for %v %s/%s, need %d, deleting %d", controllerKind, ms.Namespace, ms.Name, *(ms.Spec.Replicas), diff)

		// Choose which Machines to delete.
		machinesToDelete := getMachinesToDelete(machines, diff)

		// TODO: Add cap to limit concurrent delete calls.
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

// shoudExcludeMachine returns true if the machine should be filtered out, false otherwise.
func (c *MachineSetControllerImpl) shouldExcludeMachine(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) bool {
	// Ignore inactive machines.
	if machine.DeletionTimestamp != nil || !machine.DeletionTimestamp.IsZero() {
		glog.V(2).Infof("Skipping machine (%v), as it is being deleted.", machine.Name)
		return true
	}

	if metav1.GetControllerOf(machine) != nil && !metav1.IsControlledBy(machine, machineSet) {
		glog.V(4).Infof("%s not controlled by %v", machine.Name, machineSet.Name)
		return true
	}
	selector, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		glog.Warningf("unable to convert selector: %v", err)
		return true
	}
	// If a deployment with a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		glog.V(4).Infof("%v machineset has empty selector", machineSet.Name)
		return true
	}
	if !selector.Matches(labels.Set(machine.Labels)) {
		glog.V(4).Infof("%v machine has mismatch labels", machine.Name)
		return true
	}
	return false
}

func (c *MachineSetControllerImpl) adoptOrphan(machineSet *v1alpha1.MachineSet, machine *v1alpha1.Machine) error {
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
		return err
	}
	return nil
}

func (c *MachineSetControllerImpl) enqueueAfter(machineSet *v1alpha1.MachineSet, after time.Duration) error {
	key, err := cache.MetaNamespaceKeyFunc(machineSet)
	if err != nil {
		glog.Errorf("Couldn't get key for object %+v: %v", machineSet, after)
		return err
	}
	c.queue.AddAfter(key, after)
	return nil
}

func getMachinesToDelete(filteredMachines []*v1alpha1.Machine, diff int) []*v1alpha1.Machine {
	// TODO: Define machines deletion policies.
	// see: https://github.com/kubernetes/kube-deploy/issues/625
	return filteredMachines[:diff]
}

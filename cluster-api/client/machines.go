package client

import (
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
)

type MachinesGetter interface {
	Machines() MachinesInterface
}

// MachinesInterface has methods to work with Machine resources.
type MachinesInterface interface {
	Create(*machinesv1.Machine) (*machinesv1.Machine, error)
	Update(*machinesv1.Machine) (*machinesv1.Machine, error)
	Delete(string, *metav1.DeleteOptions) error
	List(metav1.ListOptions) (*machinesv1.MachineList, error)
	Get(string, metav1.GetOptions) (*machinesv1.Machine, error)
}

// machines implements MachinesInterface
type machines struct {
	client rest.Interface
}

// newMachines returns a machines
func newMachines(c *ClusterAPIV1Alpha1Client) *machines {
	return &machines{
		client: c.RESTClient(),
	}
}

func (c *machines) Create(machine *machinesv1.Machine) (result *machinesv1.Machine, err error) {
	result = &machinesv1.Machine{}
	err = c.client.Post().
		Namespace(apiv1.NamespaceDefault).
		Resource(machinesv1.MachinesCRDPlural).
		Body(machine).
		Do().
		Into(result)
	return
}

func (c *machines) Update(machine *machinesv1.Machine) (result *machinesv1.Machine, err error) {
	result = &machinesv1.Machine{}
	err = c.client.Put().
		Namespace(apiv1.NamespaceDefault).
		Resource(machinesv1.MachinesCRDPlural).
		Name(machine.Name).
		Body(machine).
		Do().
		Into(result)
	return
}

func (c *machines) Delete(name string, options *metav1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(apiv1.NamespaceDefault).
		Resource(machinesv1.MachinesCRDPlural).
		Name(name).
		Body(options).
		Do().
		Error()
}

// List takes label and field selectors, and returns the list of machines that match those selectors.
func (c *machines) List(opts metav1.ListOptions) (result *machinesv1.MachineList, err error) {
	result = &machinesv1.MachineList{}
	err = c.client.Get().
		Namespace(apiv1.NamespaceDefault).
		Resource(machinesv1.MachinesCRDPlural).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

func (c *machines) Get(name string, options metav1.GetOptions) (result *machinesv1.Machine, err error) {
	result = &machinesv1.Machine{}
	err = c.client.Get().
		Namespace(apiv1.NamespaceDefault).
		Resource(machinesv1.MachinesCRDPlural).
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

package google

import (
	"fmt"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	gceconfig "k8s.io/kube-deploy/cluster-api/machinecontroller/cloud/google/gceproviderconfig"
	gceconfigv1 "k8s.io/kube-deploy/cluster-api/machinecontroller/cloud/google/gceproviderconfig/v1alpha1"
	"path"
)

type GCEClient struct {
	service      *compute.Service
	scheme       *runtime.Scheme
	codecFactory *serializer.CodecFactory
}

func NewMachineActuator() (*GCEClient, error) {
	// The default GCP client expects the environment variable
	// GOOGLE_APPLICATION_CREDENTIALS to point to a file with service credentials.
	client, err := google.DefaultClient(context.TODO(), compute.ComputeScope)
	if err != nil {
		return nil, err
	}

	service, err := compute.New(client)
	if err != nil {
		return nil, err
	}

	scheme, codecFactory, err := gceconfigv1.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}

	return &GCEClient{
		service:      service,
		scheme:       scheme,
		codecFactory: codecFactory,
	}, nil
}

func (gce *GCEClient) Create(machine *machinesv1.Machine) error {
	config, err := gce.providerconfig(machine)
	if err != nil {
		return err
	}

	startupScript := nodeStartupTemplate

	op, err := gce.service.Instances.Insert(config.Project, config.Zone, &compute.Instance{
		Name:        machine.ObjectMeta.Name,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", config.Zone, config.MachineType),
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				Network: "global/networks/default",
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "External NAT",
					},
				},
			},
		},
		Disks: []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: config.Image,
					DiskSizeGb:  10,
				},
			},
		},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				&compute.MetadataItems{
					Key: "startup-script",
					Value: &startupScript,
				},
			},
		},
	}).Do()

	gce.waitForOperation(config, op)

	return err
}

func (gce *GCEClient) Delete(machine *machinesv1.Machine) error {
	config, err := gce.providerconfig(machine)
	if err != nil {
		return err
	}

	op, err := gce.service.Instances.Delete(config.Project, config.Zone, machine.ObjectMeta.Name).Do()
	gce.waitForOperation(config, op)
	return err
}

func (gce *GCEClient) Get(name string) (*machinesv1.Machine, error){
	return nil, fmt.Errorf("Get machine is not implemented on Google")
}

func (gce *GCEClient) providerconfig(machine *machinesv1.Machine) (*gceconfig.GCEProviderConfig, error) {
	obj, gvk, err := gce.codecFactory.UniversalDecoder().Decode([]byte(machine.Spec.ProviderConfig), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}

	config, ok := obj.(*gceconfig.GCEProviderConfig)
	if !ok {
		return nil, fmt.Errorf("failure to cast to gce; type: %v", gvk)
	}

	return config, nil
}

func (gce *GCEClient) waitForOperation(c *gceconfig.GCEProviderConfig, op *compute.Operation) error {
	for op != nil && op.Status != "DONE" {
		var err error
		op, err = gce.getOp(c, op)
		if err != nil {
			return err
		}
	}
	return nil
}

// getOp returns an updated operation.
func (gce *GCEClient) getOp(c *gceconfig.GCEProviderConfig, op *compute.Operation) (*compute.Operation, error) {
	return gce.service.ZoneOperations.Get(c.Project, path.Base(op.Zone), op.Name).Do()
}
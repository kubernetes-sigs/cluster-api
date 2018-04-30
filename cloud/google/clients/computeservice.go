package clients

import (
	compute "google.golang.org/api/compute/v1"
	"net/http"
	"net/url"
)

// ComputeService is a pass through wrapper for google.golang.org/api/compute/v1/compute
// The purpose of the ComputeService's wrap of the GCE client is to enable tests to mock this struct and control behavior.
type ComputeService struct {
	service *compute.Service
}

func NewComputeService(client *http.Client) (*ComputeService, error) {
	service, err := compute.New(client)
	if err != nil {
		return nil, err
	}
	return &ComputeService{
		service: service,
	}, nil
}

func NewComputeServiceForURL(client *http.Client, baseURL string) (*ComputeService, error) {
	computeService, err := NewComputeService(client)
	if err != nil {
		return nil, err
	}
	url, err := url.Parse(computeService.service.BasePath)
	if err != nil {
		return nil, err
	}
	computeService.service.BasePath = baseURL + url.Path
	return computeService, err
}

// A pass through wrapper for compute.Service.Images.Get(...)
func (c *ComputeService) ImagesGet(project string, image string) (*compute.Image, error) {
	return c.service.Images.Get(project, image).Do()
}

// A pass through wrapper for compute.Service.Images.GetFromFamily(...)
func (c *ComputeService) ImagesGetFromFamily(project string, family string) (*compute.Image, error) {
	return c.service.Images.GetFromFamily(project, family).Do()
}

// A pass through wrapper for compute.Service.Instances.Delete(...)
func (c *ComputeService) InstancesDelete(project string, zone string, targetInstance string) (*compute.Operation, error) {
	return c.service.Instances.Delete(project, zone, targetInstance).Do()
}

// A pass through wrapper for compute.Service.Instances.Get(...)
func (c *ComputeService) InstancesGet(project string, zone string, instance string) (*compute.Instance, error) {
	return c.service.Instances.Get(project, zone, instance).Do()
}

// A pass through wrapper for compute.Service.Instances.Insert(...)
func (c *ComputeService) InstancesInsert(project string, zone string, instance *compute.Instance) (*compute.Operation, error) {
	return c.service.Instances.Insert(project, zone, instance).Do()
}

// A pass through wrapper for compute.Service.ZoneOperations.Get(...)
func (c *ComputeService) ZoneOperationsGet(project string, zone string, operation string) (*compute.Operation, error) {
	return c.service.ZoneOperations.Get(project, zone, operation).Do()
}

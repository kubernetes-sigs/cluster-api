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

package google_test

import compute "google.golang.org/api/compute/v1"

type GCEClientComputeServiceMock struct {
	mockImagesGet           func(project string, image string) (*compute.Image, error)
	mockImagesGetFromFamily func(project string, family string) (*compute.Image, error)
	mockInstancesDelete     func(project string, zone string, targetInstance string) (*compute.Operation, error)
	mockInstancesGet        func(project string, zone string, instance string) (*compute.Instance, error)
	mockInstancesInsert     func(project string, zone string, instance *compute.Instance) (*compute.Operation, error)
	mockZoneOperationsGet   func(project string, zone string, operation string) (*compute.Operation, error)
	mockGlobalOperationsGet func(project string, operation string) (*compute.Operation, error)
	mockFirewallsGet        func(project string) (*compute.FirewallList, error)
	mockFirewallsInsert     func(project string, firewallRule *compute.Firewall) (*compute.Operation, error)
	mockFirewallsDelete     func(project string, name string) (*compute.Operation, error)
	mockWaitForOperation    func(project string, op *compute.Operation) error
}

func (c *GCEClientComputeServiceMock) ImagesGet(project string, image string) (*compute.Image, error) {
	if c.mockImagesGet == nil {
		return nil, nil
	}
	return c.mockImagesGet(project, image)
}

func (c *GCEClientComputeServiceMock) ImagesGetFromFamily(project string, family string) (*compute.Image, error) {
	if c.mockImagesGetFromFamily == nil {
		return nil, nil
	}
	return c.mockImagesGetFromFamily(project, family)
}

func (c *GCEClientComputeServiceMock) InstancesDelete(project string, zone string, targetInstance string) (*compute.Operation, error) {
	if c.mockInstancesDelete == nil {
		return nil, nil
	}
	return c.mockInstancesDelete(project, zone, targetInstance)
}

func (c *GCEClientComputeServiceMock) InstancesGet(project string, zone string, instance string) (*compute.Instance, error) {
	if c.mockInstancesGet == nil {
		return nil, nil
	}
	return c.mockInstancesGet(project, zone, instance)
}

func (c *GCEClientComputeServiceMock) InstancesInsert(project string, zone string, instance *compute.Instance) (*compute.Operation, error) {
	if c.mockInstancesInsert == nil {
		return nil, nil
	}
	return c.mockInstancesInsert(project, zone, instance)
}

func (c *GCEClientComputeServiceMock) ZoneOperationsGet(project string, zone string, operation string) (*compute.Operation, error) {
	if c.mockZoneOperationsGet == nil {
		return nil, nil
	}
	return c.mockZoneOperationsGet(project, zone, operation)
}

func (c *GCEClientComputeServiceMock) GlobalOperationsGet(project string, operation string) (*compute.Operation, error) {
	if c.mockGlobalOperationsGet == nil {
		return nil, nil
	}
	return c.mockGlobalOperationsGet(project, operation)
}

func (c *GCEClientComputeServiceMock) FirewallsGet(project string) (*compute.FirewallList, error) {
	if c.mockFirewallsGet == nil {
		return nil, nil
	}
	return c.mockFirewallsGet(project)
}

func (c *GCEClientComputeServiceMock) FirewallsInsert(project string, firewallRule *compute.Firewall) (*compute.Operation, error) {
	if c.mockFirewallsInsert == nil {
		return nil, nil
	}
	return c.mockFirewallsInsert(project, firewallRule)
}

func (c *GCEClientComputeServiceMock) FirewallsDelete(project string, name string) (*compute.Operation, error) {
	if c.mockFirewallsDelete == nil {
		return nil, nil
	}
	return c.mockFirewallsDelete(project, name)
}

func (c *GCEClientComputeServiceMock) WaitForOperation(project string, op *compute.Operation) error {
	if c.mockWaitForOperation == nil {
		return nil
	}
	return c.mockWaitForOperation(project, op)
}
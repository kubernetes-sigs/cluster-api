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

package google

import (
	compute "google.golang.org/api/compute/v1"
)

type GCEClientComputeService interface {
	ImagesGet(project string, image string) (*compute.Image, error)
	ImagesGetFromFamily(project string, family string) (*compute.Image, error)
	InstancesDelete(project string, zone string, targetInstance string) (*compute.Operation, error)
	InstancesGet(project string, zone string, instance string) (*compute.Instance, error)
	InstancesInsert(project string, zone string, instance *compute.Instance) (*compute.Operation, error)
	ZoneOperationsGet(project string, zone string, operation string) (*compute.Operation, error)
	GlobalOperationsGet(project string, operation string) (*compute.Operation, error)
	FirewallsGet(project string) (*compute.FirewallList, error)
	FirewallsInsert(project string, firewallRule *compute.Firewall) (*compute.Operation, error)
	FirewallsDelete(project string, name string) (*compute.Operation, error)
	WaitForOperation(project string, op *compute.Operation) error
}

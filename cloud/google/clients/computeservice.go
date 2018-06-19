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

package clients

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	compute "google.golang.org/api/compute/v1"
)

const (
	gceTimeout   = time.Minute * 10
	gceWaitSleep = time.Second * 5
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

// A pass through wrapper for compute.Service.GlobalOperations.Get(...)
func (c *ComputeService) GlobalOperationsGet(project string, operation string) (*compute.Operation, error) {
	return c.service.GlobalOperations.Get(project, operation).Do()
}

// A pass through wrapper for compute.Service.Firewalls.List(...)
func (c *ComputeService) FirewallsGet(project string) (*compute.FirewallList, error) {
	return c.service.Firewalls.List(project).Do()
}

// A pass through wrapper for compute.Service.Firewalls.Insert(...)
func (c *ComputeService) FirewallsInsert(project string, firewallRule *compute.Firewall) (*compute.Operation, error) {
	return c.service.Firewalls.Insert(project, firewallRule).Do()
}

// A pass through wrapper for compute.Service.Firewalls.Delete(...)
func (c *ComputeService) FirewallsDelete(project string, name string) (*compute.Operation, error) {
	return c.service.Firewalls.Delete(project, name).Do()
}

func (c *ComputeService) WaitForOperation(project string, op *compute.Operation) error {
	glog.Infof("Wait for %v %q...", op.OperationType, op.Name)
	defer glog.Infof("Finish wait for %v %q...", op.OperationType, op.Name)

	start := time.Now()
	ctx, cf := context.WithTimeout(context.Background(), gceTimeout)
	defer cf()

	var err error
	for {
		if err = c.checkOp(op, err); err != nil || op.Status == "DONE" {
			return err
		}
		glog.V(1).Infof("Wait for %v %q: %v (%d%%): %v", op.OperationType, op.Name, op.Status, op.Progress, op.StatusMessage)
		select {
		case <-ctx.Done():
			return fmt.Errorf("gce operation %v %q timed out after %v", op.OperationType, op.Name, time.Since(start))
		case <-time.After(gceWaitSleep):
		}
		op, err = c.getOp(project, op)
	}
}

// getOp returns an updated operation.
func (c *ComputeService) getOp(project string, op *compute.Operation) (*compute.Operation, error) {
	if op.Zone != "" {
		return c.ZoneOperationsGet(project, path.Base(op.Zone), op.Name)
	} else {
		return c.GlobalOperationsGet(project, op.Name)
	}
}

func (c *ComputeService) checkOp(op *compute.Operation, err error) error {
	if err != nil || op.Error == nil || len(op.Error.Errors) == 0 {
		return err
	}

	var errs bytes.Buffer
	for _, v := range op.Error.Errors {
		errs.WriteString(v.Message)
		errs.WriteByte('\n')
	}
	return errors.New(errs.String())
}

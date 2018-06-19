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

package clients_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	compute "google.golang.org/api/compute/v1"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
)

func TestImagesGet(t *testing.T) {
	mux, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseImage := compute.Image{
		Name:             "imageName",
		ArchiveSizeBytes: 544,
	}
	mux.Handle("/compute/v1/projects/projectName/global/images/imageName", handler(nil, &responseImage))
	image, err := client.ImagesGet("projectName", "imageName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if image == nil {
		t.Error("expected a valid image")
	}
	if "imageName" != image.Name {
		t.Errorf("expected imageName got %v", image.Name)
	}
	if image.ArchiveSizeBytes != int64(544) {
		t.Errorf("expected %v got %v", image.ArchiveSizeBytes, 544)
	}
}

func TestImagesGetFromFamily(t *testing.T) {
	mux, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseImage := compute.Image{
		Name:             "imageName",
		ArchiveSizeBytes: 544,
	}
	mux.Handle("/compute/v1/projects/projectName/global/images/family/familyName", handler(nil, &responseImage))
	image, err := client.ImagesGetFromFamily("projectName", "familyName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if image == nil {
		t.Error("expected a valid image")
	}
	if "imageName" != image.Name {
		t.Errorf("expected imageName got %v", image.Name)
	}
	if image.ArchiveSizeBytes != int64(544) {
		t.Errorf("expected %v got %v", image.ArchiveSizeBytes, 544)
	}
}

func TestInstancesDelete(t *testing.T) {
	mux, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseOperation := compute.Operation{
		Id: 4501,
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances/instanceName", handler(nil, &responseOperation))
	op, err := client.InstancesDelete("projectName", "zoneName", "instanceName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Error("expected a valid operation")
	}
	if responseOperation.Id != uint64(4501) {
		t.Errorf("expected %v got %v", responseOperation.Id, 4501)
	}
}

func TestInstancesGet(t *testing.T) {
	mux, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseInstance := compute.Instance{
		Name: "instanceName",
		Zone: "zoneName",
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances/instanceName", handler(nil, &responseInstance))
	instance, err := client.InstancesGet("projectName", "zoneName", "instanceName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if instance == nil {
		t.Error("expected a valid instance")
	}
	if "instanceName" != instance.Name {
		t.Errorf("expected instanceName got %v", instance.Name)
	}
	if "zoneName" != instance.Zone {
		t.Errorf("expected zoneName got %v", instance.Zone)
	}
}

func TestInstancesInsert(t *testing.T) {
	mux, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseOperation := compute.Operation{
		Id: 3001,
	}
	mux.Handle("/compute/v1/projects/projectName/zones/zoneName/instances", handler(nil, &responseOperation))
	op, err := client.InstancesInsert("projectName", "zoneName", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Error("expected a valid operation")
	}
	if responseOperation.Id != uint64(3001) {
		t.Errorf("expected %v got %v", responseOperation.Id, 3001)
	}
}

func TestWaitForOperationSuccess(t *testing.T) {
	_, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	op := &compute.Operation{
		Id: 3001,
		Status: "DONE",
	}
	err := client.WaitForOperation("projectName", op)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForOperationError(t *testing.T) {
	_, server, client := createMuxServerAndComputeClient(t)
	defer server.Close()
	responseError := &compute.OperationErrorErrors{
		Message: "testerrorthrown",
	}
	responseErrors := []*compute.OperationErrorErrors{}
	responseErrors = append(responseErrors, responseError)
	op := &compute.Operation{
		Id: 3001,
		Error: &compute.OperationError{
			Errors:responseErrors,
		},
	}
	err := client.WaitForOperation("projectName", op)
	if err == nil || err.Error() != responseError.Message+"\n" {
		t.Errorf("expected error to occur: %v", responseError.Message)
	}
}

func createMuxServerAndComputeClient(t *testing.T) (*http.ServeMux, *httptest.Server, *clients.ComputeService) {
	mux, server := createMuxAndServer()
	client, err := clients.NewComputeServiceForURL(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unable to create compute service: %v", err)
	}
	return mux, server, client
}

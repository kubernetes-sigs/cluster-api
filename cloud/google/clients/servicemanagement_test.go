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
	"fmt"
	"google.golang.org/api/servicemanagement/v1"
	"net/http"
	"net/http/httptest"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"testing"
)

func TestSMSOperationsGet(t *testing.T) {
	mux, server, client := createMuxServerAndServiceManagementClient(t)
	defer server.Close()
	responseOp := servicemanagement.Operation{
		Name: "operations/operationName",
		Done: true,
	}
	mux.Handle("/v1/operations/operationName", handler(nil, &responseOp))
	op, err := client.OperationsGet("operations/operationName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Error("expected a valid operation")
	}
	if "operations/operationName" != op.Name {
		t.Errorf("expected operations/operationName got %v", op.Name)
	}
	if !op.Done {
		t.Errorf("expected operation's 'Done' flag to be set")
	}
}

func TestServicesEnableForProject(t *testing.T) {
	testCases := []struct {
		name        string
		serviceName string
		projectName string
		operation   servicemanagement.Operation
	}{
		{"fully qualified project name", "compute.googleapis.com", "projects/projectId", servicemanagement.Operation{Done: true, Name: "operations/operationName"}},
		{"unqualified project name", "compute.googleapis.com", "projectId", servicemanagement.Operation{Done: true, Name: "operations/operationName"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndServiceManagementClient(t)
			defer server.Close()
			mux.Handle(fmt.Sprintf("/v1/services/%v:enable", tc.serviceName), handler(nil, &tc.operation))
			op, err := client.ServicesEnableForProject(tc.serviceName, tc.projectName)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if op == nil {
				t.Fatalf("expected a valid operation")
			}
			if tc.operation.Name != op.Name {
				t.Errorf("operation name mismatch: expected '%v' got '%v'", tc.operation.Name, op.Name)
			}
			if tc.operation.Done != op.Done {
				t.Errorf("expected operation's 'Done' flag to be '%v' instead '%v'", tc.operation.Done, op.Done)
			}
		})
	}
}

func TestServicesList(t *testing.T) {
	computeService := servicemanagement.ManagedService{
		ServiceName: "compute.googleapis.com",
	}
	billingService := servicemanagement.ManagedService{
		ServiceName: "cloudbilling.googleapis.com",
	}
	services1 := []*servicemanagement.ManagedService{&computeService}
	services2 := []*servicemanagement.ManagedService{&billingService}
	testCases := []struct {
		name        string
		projectName string
		responses   []servicemanagement.ListServicesResponse
	}{
		{"empty project name", "", []servicemanagement.ListServicesResponse{{}}},
		{"fully qualified project name", "projects/projectId", []servicemanagement.ListServicesResponse{{Services: services1}}},
		{"unqualified project name", "projectId", []servicemanagement.ListServicesResponse{{Services: services1}}},
		{
			"paginated response",
			"project/projectId",
			[]servicemanagement.ListServicesResponse{
				{NextPageToken: "next-token", Services: services1},
				{NextPageToken: "", Services: services2},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndServiceManagementClient(t)
			defer server.Close()
			tokenToResponse := make(map[string]interface{})
			nextToken := ""
			for _, response := range tc.responses {
				tokenToResponse[nextToken] = response
				nextToken = response.NextPageToken
			}
			mux.Handle("/v1/services", paginatedHandler(nil, tokenToResponse))
			svcs, err := client.ServicesList(tc.projectName)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			verifyServicesWithResponses(t, svcs, tc.responses)
		})
	}
}

func verifyServicesWithResponses(t *testing.T, services []*servicemanagement.ManagedService, responses []servicemanagement.ListServicesResponse) {
	t.Helper()
	responseServices := make([]*servicemanagement.ManagedService, 0)
	for _, r := range responses {
		responseServices = append(responseServices, r.Services...)
	}
	if len(responseServices) != len(services) {
		t.Errorf("services list length mismatch: expected '%v' got '%v'", len(responseServices), len(services))
	}
}

func createMuxServerAndServiceManagementClient(t *testing.T) (*http.ServeMux, *httptest.Server, *clients.ServiceManagementService) {
	t.Helper()
	mux, server := createMuxAndServer()
	client, err := clients.NewServiceManagementServiceForURL(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unable to create service management service: %v", err)
	}
	return mux, server, client
}

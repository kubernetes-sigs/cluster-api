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
	"google.golang.org/api/cloudresourcemanager/v1"
	"net/http"
	"net/http/httptest"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"testing"
)

func TestCRMSOperationsGet(t *testing.T) {
	mux, server, client := createMuxServerAndCloudResourceManagerClient(t)
	defer server.Close()
	responseOp := cloudresourcemanager.Operation{
		Name: "operations/operationName",
		Done: true,
	}
	mux.Handle("/v1/operations/operationName", handler(nil, &responseOp))
	op, err := client.OperationsGet("operations/operationName")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Fatalf("expected a valid operation")
	}
	if "operations/operationName" != op.Name {
		t.Errorf("expected operations/operationName got %v", op.Name)
	}
	if !op.Done {
		t.Errorf("expected operation's 'Done' flag to be set")
	}
}

func TestProjectsCreate(t *testing.T) {
	mux, server, client := createMuxServerAndCloudResourceManagerClient(t)
	defer server.Close()
	responseOperation := cloudresourcemanager.Operation{
		Name: "operations/operationName",
		Done: true,
	}
	mux.Handle("/v1/projects", handler(nil, &responseOperation))
	project := cloudresourcemanager.Project{
		Name:      "projectId",
		ProjectId: "projectId",
	}
	op, err := client.ProjectsCreate(&project)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if op == nil {
		t.Fatalf("expected a valid operation")
	}
	if "operations/operationName" != op.Name {
		t.Errorf("expected operations/operationName got %v", op.Name)
	}
	if !op.Done {
		t.Errorf("expected operation's 'Done' flag to be set")
	}
}

func TestProjectGet(t *testing.T) {
	mux, server, client := createMuxServerAndCloudResourceManagerClient(t)
	defer server.Close()
	responseProject := cloudresourcemanager.Project{
		Name:      "projects/projectId",
		ProjectId: "projectId",
	}
	mux.Handle("/v1/projects/projectId", handler(nil, &responseProject))
	project, err := client.ProjectsGet("projectId")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if project == nil {
		t.Fatalf("expected a valid project")
	}
	if responseProject.Name != project.Name {
		t.Errorf("project.Name mismatch: expected '%v' got '%v'", responseProject.Name, project.Name)
	}
	if responseProject.ProjectId != project.ProjectId {
		t.Errorf("project.ProjectId mismatch: expected '%v' got '%v'", responseProject.ProjectId, project.ProjectId)
	}
}

func TestProjectList(t *testing.T) {
	project1 := cloudresourcemanager.Project{Name: "projects/projectId1", ProjectId: "projectId1"}
	project2 := cloudresourcemanager.Project{Name: "projects/projectId2", ProjectId: "projectId2"}
	testCases := []struct {
		name      string
		responses []cloudresourcemanager.ListProjectsResponse
	}{
		{"empty result", []cloudresourcemanager.ListProjectsResponse{{}}},
		{"single result", []cloudresourcemanager.ListProjectsResponse{{NextPageToken: "", Projects: []*cloudresourcemanager.Project{&project1}}}},
		{"multiple results", []cloudresourcemanager.ListProjectsResponse{
			{NextPageToken: "next-token", Projects: []*cloudresourcemanager.Project{&project1}},
			{NextPageToken: "", Projects: []*cloudresourcemanager.Project{&project2}},
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndCloudResourceManagerClient(t)
			defer server.Close()
			tokenToResponse := make(map[string]interface{})
			nextToken := ""
			for _, response := range tc.responses {
				tokenToResponse[nextToken] = response
				nextToken = response.NextPageToken
			}
			mux.Handle("/v1/projects", paginatedHandler(nil, tokenToResponse))
			projChan, err := client.ProjectsList("")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			verifyProjectsWithResponses(t, projChan, tc.responses)
		})
	}
}

func verifyProjectsWithResponses(t *testing.T, projects []*cloudresourcemanager.Project, responses []cloudresourcemanager.ListProjectsResponse) {
	t.Helper()
	responseProjects := make([]*cloudresourcemanager.Project, 0)
	for _, r := range responses {
		responseProjects = append(responseProjects, r.Projects...)
	}
	if len(responseProjects) != len(projects) {
		t.Errorf("projects list length mismatch: expected '%v' got '%v'", len(responseProjects), len(projects))
	}
}

func createMuxServerAndCloudResourceManagerClient(t *testing.T) (*http.ServeMux, *httptest.Server, *clients.CloudResourceManagerService) {
	t.Helper()
	mux, server := createMuxAndServer()
	client, err := clients.NewCloudResourceManagerServiceForURL(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unable to create cloud resource manager service: %v", err)
	}
	return mux, server, client
}

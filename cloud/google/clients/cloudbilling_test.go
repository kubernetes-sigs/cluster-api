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
	"google.golang.org/api/cloudbilling/v1"
	"net/http"
	"net/http/httptest"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"testing"
)

func TestBillingAccountsList(t *testing.T) {
	account1 := cloudbilling.BillingAccount{
		Name: "billingaccounts/account1",
	}
	account2 := cloudbilling.BillingAccount{
		Name: "billingaccounts/account2",
	}
	accounts1 := []*cloudbilling.BillingAccount{&account1}
	accounts2 := []*cloudbilling.BillingAccount{&account2}
	testCases := []struct {
		name        string
		projectName string
		responses   []cloudbilling.ListBillingAccountsResponse
	}{
		{"empty project name", "", []cloudbilling.ListBillingAccountsResponse{{}}},
		{"fully qualified project name", "projects/projectId", []cloudbilling.ListBillingAccountsResponse{{BillingAccounts: accounts1}}},
		{"unqualified project name", "projectId", []cloudbilling.ListBillingAccountsResponse{{BillingAccounts: accounts1}}},
		{
			"paginated response",
			"project/projectId",
			[]cloudbilling.ListBillingAccountsResponse{
				{NextPageToken: "next-token", BillingAccounts: accounts1},
				{NextPageToken: "", BillingAccounts: accounts2},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndCloudBillingClient(t)
			defer server.Close()
			tokenToResponse := make(map[string]interface{})
			nextToken := ""
			for _, response := range tc.responses {
				tokenToResponse[nextToken] = response
				nextToken = response.NextPageToken
			}
			mux.Handle("/v1/billingAccounts", paginatedHandler(nil, tokenToResponse))
			accounts, err := client.BillingAccountsList()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			verifyBillingAccountsWithResponses(t, accounts, tc.responses)
		})
	}
}

func TestProjectsGetBillingInfo(t *testing.T) {
	projectBillingInfo := cloudbilling.ProjectBillingInfo{BillingAccountName: "billingAccountName", ProjectId: "projectId"}
	testCases := []struct {
		name               string
		projectNameParam   string
		projectBillingInfo cloudbilling.ProjectBillingInfo
	}{
		{"fully qualified project name", "projects/projectId", projectBillingInfo},
		{"unqualified project name", "projectId", projectBillingInfo},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndCloudBillingClient(t)
			defer server.Close()
			mux.Handle("/v1/projects/projectId/billingInfo", handler(nil, &tc.projectBillingInfo))
			billingInfo, err := client.ProjectsGetBillingInfo(tc.projectNameParam)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if billingInfo == nil {
				t.Fatalf("expected a valid billingInfo")
			}
			if tc.projectBillingInfo.BillingAccountName != billingInfo.BillingAccountName {
				t.Errorf("ProjectBillingInfo.BillingAccountName mismatch: expected '%v' got '%v'", tc.projectBillingInfo.BillingAccountName, billingInfo.BillingAccountName)
			}
			if tc.projectBillingInfo.ProjectId != billingInfo.ProjectId {
				t.Errorf("ProjectBillingInfo.ProjectId mismatch: expected '%v' got '%v'", tc.projectBillingInfo.ProjectId, billingInfo.ProjectId)
			}
		})
	}
}

func TestProjectsUpdateBillingInfo(t *testing.T) {
	projectBillingInfo := cloudbilling.ProjectBillingInfo{BillingAccountName: "billingAccountName", ProjectId: "projectId"}
	testCases := []struct {
		name               string
		projectNameParam   string
		projectBillingInfo cloudbilling.ProjectBillingInfo
	}{
		{"fully qualified project name", "projects/projectId", projectBillingInfo},
		{"unqualified project name", "projectId", projectBillingInfo},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mux, server, client := createMuxServerAndCloudBillingClient(t)
			defer server.Close()
			mux.Handle("/v1/projects/projectId/billingInfo", handler(nil, &tc.projectBillingInfo))
			billingInfo, err := client.ProjectsUpdateBillingInfo(tc.projectNameParam, &tc.projectBillingInfo)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if billingInfo == nil {
				t.Fatalf("expected a valid billingInfo")
			}
			if tc.projectBillingInfo.BillingAccountName != billingInfo.BillingAccountName {
				t.Errorf("ProjectBillingInfo.BillingAccountName mismatch: expected '%v' got '%v'", tc.projectBillingInfo.BillingAccountName, billingInfo.BillingAccountName)
			}
			if tc.projectBillingInfo.ProjectId != billingInfo.ProjectId {
				t.Errorf("ProjectBillingInfo.ProjectId mismatch: expected '%v' got '%v'", tc.projectBillingInfo.ProjectId, billingInfo.ProjectId)
			}
		})
	}
}

func verifyBillingAccountsWithResponses(t *testing.T, accounts []*cloudbilling.BillingAccount, responses []cloudbilling.ListBillingAccountsResponse) {
	t.Helper()
	responseAccounts := make([]*cloudbilling.BillingAccount, 0)
	for _, r := range responses {
		responseAccounts = append(responseAccounts, r.BillingAccounts...)
	}
	if len(responseAccounts) != len(accounts) {
		t.Errorf("billing accounts list length mismatch: expected '%v' got '%v'", len(responseAccounts), len(accounts))
	}
}

func createMuxServerAndCloudBillingClient(t *testing.T) (*http.ServeMux, *httptest.Server, *clients.CloudBillingService) {
	t.Helper()
	mux, server := createMuxAndServer()
	client, err := clients.NewCloudBillingServiceForURL(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unable to create cloud billing service: %v", err)
	}
	return mux, server, client
}

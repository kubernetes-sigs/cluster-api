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
	"encoding/json"
	"fmt"
	"google.golang.org/api/googleapi"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"testing"
)

func createMuxAndServer() (*http.ServeMux, *httptest.Server) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	return mux, server
}

func handler(err *googleapi.Error, obj interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		handleTestRequest(w, err, obj)
	}
}

// this handler maps tokens to return values. This is useful with GCP APIs that use pagination. It is assumed that the API
// call will have a parameter named "pageToken" as this is standardized across all GCP services
func paginatedHandler(googleErr *googleapi.Error, tokenToObj map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		values, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			panic(err)
		}
		token := values.Get("pageToken")
		obj, ok := tokenToObj[token]
		if !ok {
			http.Error(w, fmt.Sprintf("invalid / unregistered page token: %v", token), http.StatusInternalServerError)
			return
		}
		handleTestRequest(w, googleErr, obj)
	}
}

func handleTestRequest(w http.ResponseWriter, handleErr *googleapi.Error, obj interface{}) {
	if handleErr != nil {
		http.Error(w, errMsg(handleErr), handleErr.Code)
		return
	}
	res, err := json.Marshal(obj)
	if err != nil {
		http.Error(w, "json marshal error", http.StatusInternalServerError)
		return
	}
	w.Write(res)
}

func errMsg(e *googleapi.Error) string {
	res, err := json.Marshal(&errorReply{e})
	if err != nil {
		return "json marshal error"
	}
	return string(res)
}

type errorReply struct {
	Error *googleapi.Error `json:"error"`
}

func TestGetConsumerIdForProject(t *testing.T) {
	testCases := []struct {
		name           string
		projectIdParam string
		expectedResult string
	}{
		{"basic", "projectId", "project:projectId"},
		{"odd-project-name", "%#$proj--", "project:%#$proj--"},
		{"all integers", "123456789", "project:123456789"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := clients.GetConsumerIdForProject(tc.projectIdParam)
			if result != tc.expectedResult {
				t.Errorf("unexpected result: got '%v' want '%v'", result, tc.expectedResult)
			}
		})
	}
}

func TestNormalizeProjectNameOrId(t *testing.T) {
	testCases := []struct {
		name           string
		projectIdParam string
		expectedResult string
	}{
		{"unqualified project id", "projectId", "projects/projectId"},
		{"fully qualified project id", "projects/projectId", "projects/projectId"},
		{"projectId name starting with slash", "/projectId", "projects//projectId"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := clients.NormalizeProjectNameOrId(tc.projectIdParam)
			if result != tc.expectedResult {
				t.Errorf("unexpected result: got '%v' want '%v'", result, tc.expectedResult)
			}
		})
	}
}

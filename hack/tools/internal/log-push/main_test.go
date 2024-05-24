//go:build tools
// +build tools

/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"testing"
)

func Test_calculateGCSLogLocation(t *testing.T) {
	tests := []struct {
		name       string
		logPath    string
		wantBucket string
		wantFolder string
	}{
		{
			name:       "ClusterAPI ProwJob",
			logPath:    "https://prow.k8s.io/view/gs/kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216",
			wantBucket: "kubernetes-jenkins",
			wantFolder: "pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216",
		},
		{
			name:       "GCS path",
			logPath:    "gs://kubernetes-jenkins/pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216",
			wantBucket: "kubernetes-jenkins",
			wantFolder: "pr-logs/pull/kubernetes-sigs_cluster-api/6189/pull-cluster-api-build-main/1496233282759561216",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBucket, gotFolder, err := calculateGCSLogLocation(tt.logPath)

			if err != nil {
				t.Errorf("calculateGCSLogLocation() unexpected error: %v", err)
			}
			if gotBucket != tt.wantBucket {
				t.Errorf("calculateGCSLogLocation() gotBucket = %v, wantBucket %v", gotBucket, tt.wantBucket)
			}
			if gotFolder != tt.wantFolder {
				t.Errorf("calculateGCSLogLocation() gotFolder = %v, wantFolder %v", gotFolder, tt.wantFolder)
			}
		})
	}
}

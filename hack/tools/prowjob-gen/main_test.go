/*
Copyright 2024 The Kubernetes Authors.

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

// main is the main package for prowjob-gen.
package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_Generator(t *testing.T) {
	g, err := newGenerator("test/test-configuration.yaml", "test", "test")
	if err != nil {
		t.Errorf("newGenerator() error = %v", err)
		return
	}

	if err := g.generate(); err != nil {
		t.Errorf("g.generate() error = %v", err)
		return
	}

	goldenData, err := os.ReadFile("test/test-main.yaml.golden")
	if err != nil {
		t.Errorf("reading golden file: %v", err)
		return
	}

	testData, err := os.ReadFile("test/test-main.yaml.tmp")
	if err != nil {
		t.Errorf("reading file generated from test: %v", err)
		return
	}

	if diff := cmp.Diff(string(goldenData), string(testData)); diff != "" {
		t.Errorf("generated and golden test file differ:\n%s", diff)
	}
}

const configFileDelimiter = "<!-- test/test-configuration.yaml -->"

func Test_testConfiguration(t *testing.T) {
	rawReadme, err := os.ReadFile("README.md")
	if err != nil {
		t.Errorf("reading file README.md: %v", err)
		return
	}

	splitted := strings.Split(string(rawReadme), configFileDelimiter)
	if len(splitted) != 3 {
		t.Errorf("expected README.md to contain %q twice", configFileDelimiter)
	}

	rawConfiguration, err := os.ReadFile("test/test-configuration.yaml")
	if err != nil {
		t.Errorf("reading file test/test-configuration.yaml: %v", err)
		return
	}

	readmeConfiguration := strings.TrimPrefix(string(rawConfiguration), "\n``yaml\n")
	readmeConfiguration = strings.TrimSuffix(readmeConfiguration, "```\n")

	if diff := cmp.Diff(readmeConfiguration, string(rawConfiguration)); diff != "" {
		t.Errorf("Configuration in README.md and test/test-configuration.yaml differ:\n%s", diff)
	}
}

func Test_bucketCron(t *testing.T) {
	tests := []struct {
		bucketName                                   string
		startHour, bucketSize, bucketIntervalMinutes int
		want                                         string
	}{
		// run test-e2e jobs staring at 2AM, max 3 jobs in the same cron bucket, defer exceeding jobs to the next bucket starting in 30m
		{bucketName: "test-e2e", startHour: 2, bucketSize: 3, bucketIntervalMinutes: 30, want: "0 2 * * *"},  // job #1, first bucket
		{bucketName: "test-e2e", startHour: 2, bucketSize: 3, bucketIntervalMinutes: 30, want: "0 2 * * *"},  // job #2, first bucket
		{bucketName: "test-e2e", startHour: 2, bucketSize: 3, bucketIntervalMinutes: 30, want: "0 2 * * *"},  // job #3, first bucket
		{bucketName: "test-e2e", startHour: 2, bucketSize: 3, bucketIntervalMinutes: 30, want: "30 2 * * *"}, // job #4, second bucket

		// handle EOD properly
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "0 22 * * *"},
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "30 22 * * *"},
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "0 23 * * *"},
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "30 23 * * *"},
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "0 0 * * *"},
		{bucketName: "test-e2e-update", startHour: 22, bucketSize: 1, bucketIntervalMinutes: 30, want: "30 0 * * *"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("bucketName %s, bucketSize: %d, hour: %d, bucketIntervalMinutes: %d", tt.bucketName, tt.bucketSize, tt.startHour, tt.bucketIntervalMinutes), func(t *testing.T) {
			if got := cronBucket(tt.bucketName, tt.startHour, tt.bucketSize, tt.bucketIntervalMinutes); got != tt.want {
				t.Errorf("cronBucket() = %v, want %v", got, tt.want)
			}
		})
	}
}

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

package kubeadm_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/cluster-api/pkg/kubeadm"
	"sigs.k8s.io/cluster-api/pkg/test-cmd-runner"
)

var (
	joinCommand   = "kubeadm join --token a53d73.21029eb48b9002d0 35.199.169.93:443 --discovery-token-ca-cert-hash sha256:3bb9ee3aa3cee9898b85523e8dd6921752e07e3053453084c8d26d40d929a132"
	errorExitCode = -1
	errorMessage  = "error message"
)

func init() {
	test_cmd_runner.RegisterCallback(echoCallback)
	test_cmd_runner.RegisterCallback(errorCallback)
	test_cmd_runner.RegisterCallback(tokenCallback)
}

func TestTokenCreateParameters(t *testing.T) {
	var tests = []struct {
		name   string
		err    error
		output string
		params kubeadm.TokenCreateParams
	}{
		{"empty params", nil, "kubeadm token create", kubeadm.TokenCreateParams{}},
		{"config", nil, "kubeadm token create --config /my/path/to/kubeadm-config", kubeadm.TokenCreateParams{Config: "/my/path/to/kubeadm-config"}},
		{"description", nil, "kubeadm token create --description my custom description", kubeadm.TokenCreateParams{Description: "my custom description"}},
		{"groups", nil, "kubeadm token create --groups system:bootstrappers:kubeadm:default-node-token", kubeadm.TokenCreateParams{Groups: []string{"system", "bootstrappers", "kubeadm", "default-node-token"}}},
		{"help", nil, "kubeadm token create --help", kubeadm.TokenCreateParams{Help: true}},
		{"print join command", nil, "kubeadm token create --print-join-command", kubeadm.TokenCreateParams{PrintJoinCommand: true}},
		{"ttl 24 hours", nil, "kubeadm token create --ttl 24h0m0s", kubeadm.TokenCreateParams{Ttl: toDuration(24, 0, 0)}},
		{"ttl 55 minutes", nil, "kubeadm token create --ttl 55m0s", kubeadm.TokenCreateParams{Ttl: toDuration(0, 55, 0)}},
		{"ttl 9 seconds", nil, "kubeadm token create --ttl 9s", kubeadm.TokenCreateParams{Ttl: toDuration(0, 0, 9)}},
		{"ttl 1 second", nil, "kubeadm token create --ttl 1s", kubeadm.TokenCreateParams{Ttl: toDuration(0, 0, 1)}},
		{"ttl 16 hours, 38 minutes, 2 seconds", nil, "kubeadm token create --ttl 16h38m2s", kubeadm.TokenCreateParams{Ttl: toDuration(16, 38, 2)}},
		{"usages", nil, "kubeadm token create --usages signing:authentication", kubeadm.TokenCreateParams{Usages: []string{"signing", "authentication"}}},
		{"all", nil, "kubeadm token create --config /my/config --description my description --groups bootstrappers --help --print-join-command --ttl 1h1m1s --usages authentication",
			kubeadm.TokenCreateParams{Config: "/my/config", Description: "my description", Groups: []string{"bootstrappers"}, Help: true, PrintJoinCommand: true, Ttl: toDuration(1, 1, 1), Usages: []string{"authentication"}}},
	}
	kadm := kubeadm.NewWithCmdRunner(test_cmd_runner.NewTestRunnerFailOnErr(t, echoCallback))
	for _, tst := range tests {
		output, err := kadm.TokenCreate(tst.params)
		if err != tst.err {
			t.Errorf("test case '%v', unexpected error: got '%v', want '%v'", tst.name, err, tst.err)
		}
		if output != tst.output {
			t.Errorf("test case '%v', unexpected output: got '%v', want '%v'", tst.name, output, tst.output)
		}
	}
}

func TestTokenCreateReturnsUnmodifiedOutput(t *testing.T) {
	kadm := kubeadm.NewWithCmdRunner(test_cmd_runner.NewTestRunnerFailOnErr(t, tokenCallback))
	output, err := kadm.TokenCreate(kubeadm.TokenCreateParams{})
	if err != nil {
		t.Errorf("unexpected error: wanted nil")
	}
	if output != joinCommand {
		t.Errorf("unexpected output: got '%v', want '%v'", output, joinCommand)
	}
}

func TestNonZeroExitCodeResultsInError(t *testing.T) {
	kadm := kubeadm.NewWithCmdRunner(test_cmd_runner.NewTestRunnerFailOnErr(t, errorCallback))
	output, err := kadm.TokenCreate(kubeadm.TokenCreateParams{})
	if err == nil {
		t.Errorf("expected error: got nil")
	}
	if output != errorMessage {
		t.Errorf("unexpected output: got '%v', want '%v'", output, errorMessage)
	}
}

func echoCallback(cmd string, args ...string) int {
	fullCmd := append([]string{cmd}, args...)
	fmt.Print(strings.Join(fullCmd, " "))
	return 0
}

func tokenCallback(cmd string, args ...string) int {
	fmt.Print(joinCommand)
	return 0
}

func errorCallback(cmd string, args ...string) int {
	fmt.Fprintf(os.Stderr, errorMessage)
	return errorExitCode
}

func TestMain(m *testing.M) {
	test_cmd_runner.TestMain(m)
}

func toDuration(hour int, minute int, second int) time.Duration {
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second
}

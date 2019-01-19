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

package kubeadm

import (
	"strings"
	"time"

	"sigs.k8s.io/cluster-api/pkg/cmdrunner"
)

// The purpose of Kubeadm and this file is to provide a unit tested wrapper around the 'kubeadm' exec command. Higher
// level, application specific functionality built on top of kubeadm should be be in another location.
type Kubeadm struct {
	runner cmdrunner.Runner
}

// see https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-token/ for an explanation of the parameters
type TokenCreateParams struct {
	Config           string
	Description      string
	Groups           []string
	Help             bool
	KubeConfig       string
	PrintJoinCommand bool
	TTL              time.Duration
	Usages           []string
}

func NewWithRunner(runner cmdrunner.Runner) *Kubeadm {
	return &Kubeadm{
		runner: runner,
	}
}

func New() *Kubeadm {
	return NewWithRunner(cmdrunner.New())
}

// TokenCreate execs `kubeadm token create` with the appropriate flags added by interpreting the params argument. The output of
// `kubeadm token create` is returned in full, including the terminating newline, without any modification.
func (k *Kubeadm) TokenCreate(params TokenCreateParams) (string, error) {
	args := []string{"token", "create"}
	args = appendStringParamIfPresent(args, "--config", params.Config)
	args = appendStringParamIfPresent(args, "--description", params.Description)
	args = appendStringSliceIfValid(args, "--groups", params.Groups)
	args = appendFlagIfTrue(args, "--help", params.Help)
	args = appendStringParamIfPresent(args, "--kubeconfig", params.KubeConfig)
	args = appendFlagIfTrue(args, "--print-join-command", params.PrintJoinCommand)
	if params.TTL != time.Duration(0) {
		args = append(args, "--ttl")
		args = append(args, params.TTL.String())
	}
	args = appendStringSliceIfValid(args, "--usages", params.Usages)
	return k.runner.CombinedOutput("kubeadm", args...)
}

func appendFlagIfTrue(args []string, paramName string, value bool) []string {
	if value {
		return append(args, paramName)
	}
	return args
}

func appendStringParamIfPresent(args []string, paramName string, value string) []string {
	if value == "" {
		return args
	}
	args = append(args, paramName)
	return append(args, value)
}

func appendStringSliceIfValid(args []string, paramName string, values []string) []string {
	if len(values) == 0 {
		return args
	}
	args = append(args, paramName)
	return append(args, toStringSlice(values))
}

func toStringSlice(args []string) string {
	return strings.Join(args, ":")
}

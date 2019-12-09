/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/kind/pkg/exec"
)

func TestRunCmdUnmarshal(t *testing.T) {
	cloudData := `
runcmd:
- [ ls, -l, / ]
- "ls -l /"`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	if err != nil {
		t.Fatal(err)
	}

	if len(r.Cmds) != 2 {
		t.Errorf("Expected 2 commands, found %d", len(r.Cmds))
	}

	expected0 := Cmd{Cmd: "ls", Args: []string{"-l", "/"}}
	if !reflect.DeepEqual(r.Cmds[0], expected0) {
		t.Errorf("Expected %+v commands, found %+v", expected0, r.Cmds[0])
	}

	expected1 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "ls -l /"}}
	if !reflect.DeepEqual(r.Cmds[1], expected1) {
		t.Errorf("Expected %+v commands, found %+v", expected1, r.Cmds[1])
	}
}

func TestRunCmdRun(t *testing.T) {
	var useCases = []struct {
		name          string
		r             runCmd
		expectedlines []string
		expectedError bool
	}{
		{
			name: "two command pass",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "foo", Args: []string{"bar"}},
					{Cmd: "baz", Args: []string{"bbb"}},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s foo bar", prompt),
				"command [foo bar] completed",
				fmt.Sprintf("%s baz bbb", prompt),
				"command [baz bbb] completed",
			},
		},
		{
			name: "first command fails",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "fail", Args: []string{"bar"}}, // fail force fakeCmd to fail
					{Cmd: "baz", Args: []string{"bbb"}},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s fail bar", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
				// there should not be a second command!
			},
			expectedError: true,
		},
		{
			name: "second command fails",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "foo", Args: []string{"bar"}},
					{Cmd: "fail", Args: []string{"qux"}}, // fail force fakeCmd to fail
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s foo bar", prompt),
				"command [foo bar] completed",
				fmt.Sprintf("%s fail qux", prompt),
				fmt.Sprintf("%s command fail is failed", errorPrefix),
			},
			expectedError: true,
		},
		{
			name: "hack kubeadm ingore errors",
			r: runCmd{
				Cmds: []Cmd{
					{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config /tmp/kubeadm.yaml"}},
				},
			},
			expectedlines: []string{
				fmt.Sprintf("%s /bin/sh -c kubeadm init --config /tmp/kubeadm.yaml --ignore-preflight-errors=all", prompt),
				"command [/bin/sh -c kubeadm init --config /tmp/kubeadm.yaml --ignore-preflight-errors=all] completed",
			},
		},
	}

	for _, rt := range useCases {
		t.Run(rt.name, func(t *testing.T) {

			cmder := fakeCmder{t: t}
			lines, err := rt.r.Run(cmder)

			if err == nil && rt.expectedError {
				t.Error("expected error, got nil")
			}
			if err != nil && !rt.expectedError {
				t.Errorf("expected nil, got error %v", err)
			}

			if !reflect.DeepEqual(rt.expectedlines, lines) {
				t.Errorf("expected %s, got %s", rt.expectedlines, lines)
			}
		})
	}
}

type fakeCmder struct {
	t *testing.T
}

var _ exec.Cmder = &fakeCmder{}

func (c fakeCmder) Command(name string, arg ...string) exec.Cmd {
	line := fmt.Sprintf("%s %s", name, strings.Join(arg, " "))
	fail := strings.Contains(line, "fail")
	return &fakeCmd{line: line, fail: fail, t: c.t}
}

type fakeCmd struct {
	line string
	fail bool
	w    io.Writer
	t    *testing.T
}

var _ exec.Cmd = &fakeCmd{}

func (cmd *fakeCmd) Run() error {
	if cmd.fail {
		return errors.New("command fail is failed")
	}
	cmd.Write(fmt.Sprintf("command [%s] completed\n", cmd.line))
	return nil
}

func (cmd *fakeCmd) SetEnv(env ...string) exec.Cmd {
	return cmd
}

func (cmd *fakeCmd) SetStdin(r io.Reader) exec.Cmd {
	return cmd
}

func (cmd *fakeCmd) SetStdout(w io.Writer) exec.Cmd {
	cmd.w = w
	return cmd
}

func (cmd *fakeCmd) SetStderr(w io.Writer) exec.Cmd {
	return cmd
}

func (cmd *fakeCmd) Write(s string) {
	if cmd.w != nil {
		_, err := cmd.w.Write([]byte(s))
		if err != nil {
			panic(err)
		}
	}
}

func TestHackKubeadmIgnoreErrors(t *testing.T) {
	cloudData := `
runcmd:
- kubeadm init --config=/tmp/kubeadm.yaml
- [ kubeadm, join, --config=/tmp/kubeadm-controlplane-join-config.yaml ]`
	r := runCmd{}
	err := r.Unmarshal([]byte(cloudData))
	if err != nil {
		t.Fatal(err)
	}

	if len(r.Cmds) != 2 {
		t.Errorf("Expected 2 commands, found %d", len(r.Cmds))
	}

	r.Cmds[0] = hackKubeadmIgnoreErrors(r.Cmds[0])

	expected0 := Cmd{Cmd: "/bin/sh", Args: []string{"-c", "kubeadm init --config=/tmp/kubeadm.yaml --ignore-preflight-errors=all"}}
	if !reflect.DeepEqual(r.Cmds[0], expected0) {
		t.Errorf("Expected %+v commands, found %+v", expected0, r.Cmds[0])
	}

	r.Cmds[1] = hackKubeadmIgnoreErrors(r.Cmds[1])

	expected1 := Cmd{Cmd: "kubeadm", Args: []string{"join", "--config=/tmp/kubeadm-controlplane-join-config.yaml", "--ignore-preflight-errors=all"}}
	if !reflect.DeepEqual(r.Cmds[1], expected1) {
		t.Errorf("Expected %+v commands, found %+v", expected1, r.Cmds[1])
	}
}

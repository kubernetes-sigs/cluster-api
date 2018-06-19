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

package test_cmd_runner

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type TestRunner struct {
	callback func(string, ...string) int
}

var (
	TestModeEnvVar       = "GO_TEST_MODE"
	CallbackTestMode     = "CALLBACK"
	CallbackFunctionName = "GO_CALLBACK_FUNCTION_NAME"
	reggedFunctions      = make(map[string]func(string, ...string) int)
)

// TestRunner makes it easy to mock out shell commands. It will start a new program that calls into the callback function
// supplied when creating TestRunner.
//
// To use TestRunner from your test, you must do the following:
// 1. In your test file's init() method call RegisterCallback with the same callback function you passed into TestRunner
// 2. Copy the TestMain below and insert it into your test file:
//		func TestMain(m *testing.M) {
//			cmd_runner.TestMain(m)
//		}
func NewTestRunner(callback func(cmd string, args ...string) (exitCode int)) (*TestRunner, error) {
	name := getFullyQualifiedFunctionName(callback)
	if _, ok := reggedFunctions[name]; !ok {
		shortName := getUnqualifiedFunctionName(name)
		registerCallbackShortName := getUnqualifiedFunctionName(getFullyQualifiedFunctionName(RegisterCallback))
		return nil, fmt.Errorf("unregistered function: '%v', you must add the following to your package's init() method, %v(%v)", name, registerCallbackShortName, shortName)
	}
	return &TestRunner{
		callback: callback,
	}, nil
}

func NewTestRunnerFailOnErr(t *testing.T, callback func(cmd string, args ...string) int) *TestRunner {
	runner, err := NewTestRunner(callback)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// Call this method to register your callback function from your init() method. This is required before creating a TestRunner that uses your function.
func RegisterCallback(fn func(cmd string, args ...string) int) {
	name := getFullyQualifiedFunctionName(fn)
	reggedFunctions[name] = fn
}

func (runner *TestRunner) CombinedOutput(cmd string, args ...string) (string, error) {
	args = append([]string{cmd}, args...)
	command := exec.Command(os.Args[0], args...)
	command.Env = []string{
		fmt.Sprintf("%v=%v", TestModeEnvVar, CallbackTestMode),
		fmt.Sprintf("%v=%v", CallbackFunctionName, getFullyQualifiedFunctionName(runner.callback)),
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(run(m))
}

func run(m *testing.M) int {
	switch os.Getenv(TestModeEnvVar) {
	case CallbackTestMode:
		return runTestExec()
	default:
		return m.Run()
	}
}

func runTestExec() int {
	functionName := os.Getenv(CallbackFunctionName)
	fn := reggedFunctions[functionName]
	if fn == nil {
		panic(fmt.Errorf("unregistered function name: %v", functionName))
	}
	return fn(os.Args[1], os.Args[2:len(os.Args)]...)
}

// given the output of getFullyQualifiedFunctionName(...) this function returns the actual 'short' function name stripping off the package path and file name.
func getUnqualifiedFunctionName(fullyQualifiedName string) string {
	parts := strings.Split(fullyQualifiedName, ".")
	return parts[len(parts)-1]
}

// code assumes that the 'fn' parameter has a kind of 'Func'
func getFullyQualifiedFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

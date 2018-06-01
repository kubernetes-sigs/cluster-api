package cmd_runner_test

import (
	"io/ioutil"
	"os"
	"sigs.k8s.io/cluster-api/pkg/cmd-runner"
	"testing"
)

func TestInvalidCommand(t *testing.T) {
	runner := cmd_runner.New()
	output, err := runner.CombinedOutput("asdf")
	if err == nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
	if output != "" {
		t.Errorf("unexpected output: expected empty string '', got '%v'", output)
	}
}

func TestValidCommandErrors(t *testing.T) {
	skipIfCommandNotPresent(t, "ls")
	runner := cmd_runner.New()
	_, err := runner.CombinedOutput("ls /invalid/path")
	if err == nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
}

func TestValidCommandSucceeds(t *testing.T) {
	skipIfCommandNotPresent(t, "ls")
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("unable to create temp dir: %v", err)
		t.FailNow()
	}
	defer os.RemoveAll(dir)
	runner := cmd_runner.New()
	output, err := runner.CombinedOutput("ls", "-al", dir)
	if err != nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
	if output == "" {
		t.Errorf("expected valid output got empty string")
	}
}

func TestCombinedOutputShouldIncludeStdOutAndErr(t *testing.T) {
	skipIfCommandNotPresent(t, "echo")
	skipIfCommandNotPresent(t, "sh")
	runner := cmd_runner.New()
	output, err := runner.CombinedOutput("sh", "-c", "echo \"stdout\" && (>&2 echo \"stderr\")")
	if err != nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
	expectedOutput := "stdout\nstderr\n"
	if output != expectedOutput {
		t.Errorf("invalid output: expected '%v', got '%v'", expectedOutput, output)
	}
}

func skipIfCommandNotPresent(t *testing.T, cmd string) {
	runner := cmd_runner.New()
	_, err := runner.CombinedOutput("ls")
	if err != nil {
		t.Skipf("unable to run test, 'ls' reults in error: %v", err)
	}
}

package cmd

import (
	"io/ioutil"
	"os"
	"testing"
)

const validCluster = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: cluster1 
spec:`

const validMachines = `
items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    name: machine1
  spec:`

func TestParseClusterYaml(t *testing.T) {
	t.Run("File does not exist", func(t *testing.T) {
		_, err := parseClusterYaml("fileDoesNotExist")
		if err == nil {
			t.Fatal("Was able to parse a file that does not exist")
		}
	})
	var testcases = []struct {
		name        string
		contents    string
		expectedName string
		expectErr   bool
	}{
		{
			name: "valid file",
			contents: validCluster,
			expectedName: "cluster1",
		},
		{
			name: "gibberish in file",
			contents:    `blah ` + validCluster + ` blah`,
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createTempFile(testcase.contents)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file)

			c, err := parseClusterYaml(file)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if err != nil {
				return
			}
			if c == nil {
				t.Fatalf("No cluster returned in success case.")
			}
			if c.Name != testcase.expectedName {
				t.Fatalf("Unexpected name. Got: %v, Want:%v", c.Name, testcase.expectedName)
			}
		})
	}
}

func TestParseMachineYaml(t *testing.T) {
	t.Run("File does not exist", func(t *testing.T) {
		_, err := parseMachinesYaml("fileDoesNotExist")
		if err == nil {
			t.Fatal("Was able to parse a file that does not exist")
		}
	})
	var testcases = []struct {
		name        string
		contents    string
		expectErr bool
		expectedMachineCount int
	}{
		{
			name: "valid file",
			contents: validMachines,
			expectedMachineCount: 1,
		},
		{
			name: "gibberish in file",
			contents:    `blah ` + validMachines + ` blah`,
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createTempFile(testcase.contents)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(file)

			m, err := parseMachinesYaml(file)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if err != nil {
				return
			}
			if m == nil {
				t.Fatalf("No machines returned in success case.")
			}
			if len(m) != testcase.expectedMachineCount {
				t.Fatalf("Unexpected machine count. Got: %v, Want: %v", len(m), testcase.expectedMachineCount)
			}
		})
	}
}

func createTempFile(contents string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()
	f.WriteString(contents)
	return f.Name(), nil
}

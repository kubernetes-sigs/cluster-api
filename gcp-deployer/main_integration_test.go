// +build integration

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

package main_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

const (
	gcpDeployerBinaryName = "gcp-deployer"
	workingDirectoryName  = "gcp-deployer"
	machinesFileName      = "machines.yaml"
	clusterFileName       = "cluster.yaml"
	generateScriptName    = "generate-yaml.sh"
)

func TestMain(m *testing.M) {
	flag.Parse()
	err := setupPrerequisites()
	if err != nil {
		fmt.Printf("unable to setup prerequisites: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestSmokeCreateAndDelete(t *testing.T) {
	fmt.Println("Creating cluster...")
	runCreate(t)
	fmt.Println("Cluster successfully created")
	fmt.Println("Deleting cluster...")
	runDelete(t)
}

func runCreate(t *testing.T) {
	tempDir, err := ioutil.TempDir("", workingDirectoryName)
	if err != nil {
		t.Fatalf("unable to create temporary folder: %v", err)
	}
	defer os.RemoveAll(tempDir)
	clusterPath, machinesPath, err := generateYaml(tempDir)
	if err != nil {
		t.Fatalf("unable to create yaml files: %v", err)
	}
	err = runGcpDeployerPipeOuputs("create", "-c", clusterPath, "-m", machinesPath)
	if err != nil {
		t.Fatalf("unable to successfully run create: %v", err)
	}
}

func runDelete(t *testing.T) {
	err := runGcpDeployerPipeOuputs("delete")
	if err != nil {
		t.Fatalf("unable to successfully run delete: %v", err)
	}
}

func generateYaml(dir string) (clusterPath string, machinesPath string, err error) {
	paths := make(map[string]string)
	for _, file := range []string{machinesFileName + ".template", clusterFileName + ".template", generateScriptName} {
		p, err := copyFileToDir(file, dir)
		if err != nil {
			return "", "", fmt.Errorf("unable to copy '%v': %v", file, err)
		}
		paths[file] = p
	}
	err = runCmdPipeOutput(dir, paths[generateScriptName])
	if err != nil {
		return "", "", fmt.Errorf("unable to run generate script at '%v': %v", paths[generateScriptName], err)
	}
	return path.Join(dir, clusterFileName), path.Join(dir, machinesFileName), nil
}

func copyFileToDir(src string, dstDir string) (fpath string, err error) {
	bytes, err := ioutil.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("unable to read file: %v", err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	_, fname := path.Split(src)
	fpath = path.Join(dstDir, fname)
	err = ioutil.WriteFile(fpath, bytes, info.Mode())
	if err != nil {
		return "", fmt.Errorf("unable to write file: %v", err)
	}
	return fpath, nil
}

func setupPrerequisites() error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to retrieve working directory: %v", err)
	}
	_, dir := path.Split(workDir)
	if dir != workingDirectoryName {
		return fmt.Errorf("invalid working directory name: want %v, got %v", workingDirectoryName, dir)
	}
	err = cleanAndBuildGcpDeployer()
	if err != nil {
		return fmt.Errorf("unable to build gcp-deployer: %v", err)
	}
	return nil
}

func cleanAndBuildGcpDeployer() error {
	err := exec.Command("go", "clean", "./...").Run()
	if err != nil {
		return fmt.Errorf("unable to run go clean: %v", err)
	}
	err = exec.Command("go", "build", ".").Run()
	if err != nil {
		return fmt.Errorf("unable to run go build: %v", err)
	}
	return nil
}

func runGcpDeployerPipeOuputs(args ...string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to retrieve working directory: %v", err)
	}
	return runCmdPipeOutput("", path.Join(workDir, gcpDeployerBinaryName), args...)
}

func runCmdPipeOutput(workDir string, cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Dir = workDir
	cmdStr := fmt.Sprintf("%v %v", cmd, strings.Join(args, " "))
	fmt.Printf("Running command: %v\n", cmdStr)
	return command.Run()
}

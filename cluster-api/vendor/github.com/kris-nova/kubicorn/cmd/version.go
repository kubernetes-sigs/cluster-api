// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/spf13/cobra"
)

// VersionCmd represents the version command
func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Verify Kubicorn version",
		Long: `Use this command to check the version of Kubicorn.
	
	This command will return the version of the Kubicorn binary.`,
		Run: func(cmd *cobra.Command, args []string) {
			err := RunVersion(vo)
			if err != nil {
				logger.Critical(err.Error())
				os.Exit(1)
			}

		},
	}
}

// VersionOptions contains fields for version output
type VersionOptions struct {
	Version   string `json:"Version"`
	GitCommit string `json:"GitCommit"`
	BuildDate string `json:"BuildDate"`
	GOVersion string `json:"GOVersion"`
	GOARCH    string `json:"GOARCH"`
	GOOS      string `json:"GOOS"`
}

var vo = &VersionOptions{}

// RunVersion populates VersionOptions and prints to stdout
func RunVersion(vo *VersionOptions) error {

	vo.Version = getVersion()
	vo.GitCommit = getGitCommit()
	vo.BuildDate = time.Now().UTC().String()
	vo.GOVersion = runtime.Version()
	vo.GOARCH = runtime.GOARCH
	vo.GOOS = runtime.GOOS
	voBytes, err := json.Marshal(vo)
	if err != nil {
		return err
	}
	fmt.Println("Kubicorn version: ", string(voBytes))
	return nil
}

var (
	versionFile = "/src/github.com/kris-nova/kubicorn/VERSION"
)

func getVersion() string {
	path := filepath.Join(os.Getenv("GOPATH") + versionFile)
	vBytes, err := ioutil.ReadFile(path)
	if err != nil {
		// ignore error
		return ""
	}
	return string(vBytes)
}

func getGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		// ignore error
		return ""
	}
	return string(output)
}

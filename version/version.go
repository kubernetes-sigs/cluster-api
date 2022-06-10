/*
Copyright 2020 The Kubernetes Authors.

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

// Package version implements version handling code.
package version

import (
	"fmt"
	"runtime"

	"github.com/pkg/errors"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const (
	// MinimumKubernetesVersion defines the minimum Kubernetes version that can be used in a Management Cluster.
	MinimumKubernetesVersion = "v1.20.0"

	// MinimumKubernetesVersionClusterTopology defines the minimum Kubernetes version that can be used in a
	// Management Cluster when enabling the ClusterTopology feature gate.
	MinimumKubernetesVersionClusterTopology = "v1.22.0"
)

// CheckKubernetesVersion return an error if the Kubernetes version in a cluster is lower than the specified minK8sVersion.
func CheckKubernetesVersion(config *rest.Config, minK8sVersion string) error {
	client := discovery.NewDiscoveryClientForConfigOrDie(config)
	serverVersion, err := client.ServerVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get the Kubernetes version")
	}

	compareResult, err := utilversion.MustParseGeneric(serverVersion.String()).Compare(minK8sVersion)
	if err != nil {
		return errors.Wrap(err, "failed to check MinK8sVersion")
	}

	if compareResult == -1 {
		return errors.Errorf("unsupported management cluster server version: %s - minimum required version is %s", serverVersion.String(), minK8sVersion)
	}
	return nil
}

var (
	gitMajor     string // major version, always numeric
	gitMinor     string // minor version, numeric possibly followed by "+"
	gitVersion   string // semantic version, derived by build scripts
	gitCommit    string // sha1 from git, output of $(git rev-parse HEAD)
	gitTreeState string // state of git tree, either "clean" or "dirty"
	buildDate    string // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

// Info exposes information about the version used for the current running code.
type Info struct {
	Major        string `json:"major,omitempty"`
	Minor        string `json:"minor,omitempty"`
	GitVersion   string `json:"gitVersion,omitempty"`
	GitCommit    string `json:"gitCommit,omitempty"`
	GitTreeState string `json:"gitTreeState,omitempty"`
	BuildDate    string `json:"buildDate,omitempty"`
	GoVersion    string `json:"goVersion,omitempty"`
	Compiler     string `json:"compiler,omitempty"`
	Platform     string `json:"platform,omitempty"`
}

// Get returns an Info object with all the information about the current running code.
func Get() Info {
	return Info{
		Major:        gitMajor,
		Minor:        gitMinor,
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns info as a human-friendly version string.
func (info Info) String() string {
	return info.GitVersion
}

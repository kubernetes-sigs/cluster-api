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

package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/cmd/version"
	"sigs.k8s.io/yaml"
)

var (
	// gitVersionRegEx matches git versions of style 0.3.7-45-c1aeccb679cd56
	// see ./hack/version.sh for more info.
	gitVersionRegEx = regexp.MustCompile(`(.*)-(\d+)-([0-9,a-f]{14})`)
)

type versionChecker struct {
	versionFilePath string
	cliVersion      func() version.Info
	githubClient    *github.Client
}

// newVersionChecker returns a versionChecker. Its behavior has been inspired
// by https://github.com/cli/cli.
func newVersionChecker(vc config.VariablesClient) *versionChecker {
	var client *github.Client
	token, err := vc.Get("GITHUB_TOKEN")
	if err == nil {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(context.TODO(), ts)
		client = github.NewClient(tc)
	} else {
		client = github.NewClient(nil)
	}

	return &versionChecker{
		versionFilePath: filepath.Join(homedir.HomeDir(), config.ConfigFolder, "version.yaml"),
		cliVersion:      version.Get,
		githubClient:    client,
	}
}

// ReleaseInfo stores information about the release.
type ReleaseInfo struct {
	Version string
	URL     string
}

// VersionState stores the release info and the last time it was updated.
type VersionState struct {
	LastCheck     time.Time
	LatestRelease ReleaseInfo
}

// Check returns a message if the current clusterctl version is less than the
// latest available release for CAPI
// (https://github.com/kubernetes-sigs/cluster-api). It gets the latest
// release from github at most once during a 24 hour period and caches the
// state by default in $HOME/.cluster-api/state.yaml. If the clusterctl
// version is the same or greater it returns nothing.
func (v *versionChecker) Check() (string, error) {
	log := logf.Log
	cliVer, err := semver.ParseTolerant(v.cliVersion().GitVersion)
	if err != nil {
		return "", errors.Wrap(err, "unable to semver parse clusterctl GitVersion")
	}

	release, err := v.getLatestRelease()
	if err != nil {
		return "", err
	}
	if release == nil {
		return "", nil
	}
	latestVersion, err := semver.ParseTolerant(release.Version)
	if err != nil {
		return "", errors.Wrap(err, "unable to semver parse latest release version")
	}

	// if we are using a dirty dev build, just log it out
	if strings.HasSuffix(cliVer.String(), "-dirty") {
		log.V(1).Info("⚠️  Using a development build of clusterctl.", "CLIVersion", cliVer.String(), "LatestGithubRelease", release.Version)
		return "", nil
	}

	// if the cli version is a dev build off of the latest available release,
	// the just log it out as informational.
	if strings.HasPrefix(cliVer.String(), latestVersion.String()) && gitVersionRegEx.MatchString(cliVer.String()) {
		log.V(1).Info("⚠️  Using a development build of clusterctl.", "CLIVersion", cliVer.String(), "LatestGithubRelease", release.Version)
		return "", nil
	}

	if cliVer.Major == latestVersion.Major &&
		cliVer.Minor == latestVersion.Minor &&
		latestVersion.GT(cliVer) {
		return fmt.Sprintf(`
New clusterctl version available: v%s -> v%s
%s
`, cliVer, latestVersion.String(), release.URL), nil
	}
	return "", nil
}

func (v *versionChecker) getLatestRelease() (*ReleaseInfo, error) {
	log := logf.Log
	vs, err := readStateFile(v.versionFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read version state file")
	}

	// if there is no release info in the state file, pull latest release from github
	if vs == nil {
		release, _, err := v.githubClient.Repositories.GetLatestRelease(context.TODO(), "kubernetes-sigs", "cluster-api")
		if err != nil {
			log.V(1).Info("⚠️ Unable to get latest github release for clusterctl")
			// failing silently here so we don't error out in air-gapped
			// environments.
			return nil, nil
		}

		vs = &VersionState{
			LastCheck: time.Now(),
			LatestRelease: ReleaseInfo{
				Version: release.GetTagName(),
				URL:     release.GetHTMLURL(),
			},
		}
	}

	if err := writeStateFile(v.versionFilePath, vs); err != nil {
		return nil, errors.Wrap(err, "unable to write version state file")
	}

	return &vs.LatestRelease, nil

}

func writeStateFile(path string, vs *VersionState) error {
	vsb, err := yaml.Marshal(vs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path, vsb, 0600); err != nil {
		return err
	}
	return nil

}

func readStateFile(filepath string) (*VersionState, error) {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// if the file doesn't exist yet, don't error
			return nil, nil
		}
		return nil, err
	}

	vs := new(VersionState)
	if err := yaml.Unmarshal(b, vs); err != nil {
		return nil, err
	}

	// If the last check is more than 24 hours ago, then don't return the
	// version state of the file.
	if time.Since(vs.LastCheck).Hours() > 24 {
		return nil, nil
	}

	return vs, nil
}

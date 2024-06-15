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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/blang/semver/v4"
	"github.com/google/go-github/v53/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/internal/goproxy"
	"sigs.k8s.io/cluster-api/version"
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
	goproxyClient   *goproxy.Client
}

// newVersionChecker returns a versionChecker. Its behavior has been inspired
// by https://github.com/cli/cli.
func newVersionChecker(ctx context.Context, vc config.VariablesClient) (*versionChecker, error) {
	var githubClient *github.Client
	token, err := vc.Get("GITHUB_TOKEN")
	if err == nil {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		githubClient = github.NewClient(tc)
	} else {
		githubClient = github.NewClient(nil)
	}

	var goproxyClient *goproxy.Client
	if scheme, host, err := goproxy.GetSchemeAndHost(os.Getenv("GOPROXY")); err == nil && scheme != "" && host != "" {
		goproxyClient = goproxy.NewClient(scheme, host)
	}

	configDirectory, err := xdg.ConfigFile(config.ConfigFolderXDG)
	if err != nil {
		return nil, err
	}

	return &versionChecker{
		versionFilePath: filepath.Join(configDirectory, "version.yaml"),
		cliVersion:      version.Get,
		githubClient:    githubClient,
		goproxyClient:   goproxyClient,
	}, nil
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
// state by default in $XDG_CONFIG_HOME/cluster-api/state.yaml. If the clusterctl
// version is the same or greater it returns nothing.
func (v *versionChecker) Check(ctx context.Context) (string, error) {
	log := logf.Log
	cliVer, err := semver.ParseTolerant(v.cliVersion().GitVersion)
	if err != nil {
		return "", errors.Wrap(err, "unable to semver parse clusterctl GitVersion")
	}

	release, err := v.getLatestRelease(ctx)
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
		log.V(1).Info("⚠️  Using a development build of clusterctl.", "cliVersion", cliVer.String(), "latestGithubRelease", release.Version)
		return "", nil
	}

	// if the cli version is a dev build off of the latest available release,
	// the just log it out as informational.
	if strings.HasPrefix(cliVer.String(), latestVersion.String()) && gitVersionRegEx.MatchString(cliVer.String()) {
		log.V(1).Info("⚠️  Using a development build of clusterctl.", "cliVersion", cliVer.String(), "latestGithubRelease", release.Version)
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

func (v *versionChecker) getLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	log := logf.Log

	// Try to get latest clusterctl version number from the local state file.
	// NOTE: local state file is ignored if older than 1d.
	vs, err := readStateFile(v.versionFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read version state file")
	}
	if vs != nil {
		return &vs.LatestRelease, nil
	}

	// Try to get latest clusterctl version number from go modules.
	latest, err := v.goproxyGetLatest(ctx)
	if err != nil {
		log.V(5).Info("error using Goproxy client to get latest versions for clusterctl, falling back to github client")
	}
	if latest != nil {
		vs = &VersionState{
			LastCheck:     time.Now(),
			LatestRelease: *latest,
		}

		if err := writeStateFile(v.versionFilePath, vs); err != nil {
			return nil, errors.Wrap(err, "unable to write version state file")
		}
		return &vs.LatestRelease, nil
	}

	// Otherwise fall back to get latest clusterctl version number from GitHub.
	latest, err = v.gitHubGetLatest(ctx)
	if err != nil {
		log.V(1).Info("⚠️ Unable to get latest github release for clusterctl")
		// failing silently here so we don't error out in air-gapped
		// environments.
		return nil, nil //nolint:nilerr
	}

	vs = &VersionState{
		LastCheck:     time.Now(),
		LatestRelease: *latest,
	}

	if err := writeStateFile(v.versionFilePath, vs); err != nil {
		return nil, errors.Wrap(err, "unable to write version state file")
	}

	return &vs.LatestRelease, nil
}

func (v *versionChecker) goproxyGetLatest(ctx context.Context) (*ReleaseInfo, error) {
	if v.goproxyClient == nil {
		return nil, nil
	}

	gomodulePath := path.Join("sigs.k8s.io", "cluster-api")
	versions, err := v.goproxyClient.GetVersions(ctx, gomodulePath)
	if err != nil {
		return nil, err
	}

	latest := semver.Version{}
	for _, v := range versions {
		if v.GT(latest) {
			latest = v
		}
	}
	return &ReleaseInfo{
		Version: latest.String(),
		URL:     gomodulePath,
	}, nil
}

func (v *versionChecker) gitHubGetLatest(ctx context.Context) (*ReleaseInfo, error) {
	release, _, err := v.githubClient.Repositories.GetLatestRelease(ctx, "kubernetes-sigs", "cluster-api")
	if err != nil {
		return nil, err
	}
	return &ReleaseInfo{
		Version: release.GetTagName(),
		URL:     release.GetHTMLURL(),
	}, nil
}

func writeStateFile(path string, vs *VersionState) error {
	vsb, err := yaml.Marshal(vs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	return os.WriteFile(path, vsb, 0600)
}

func readStateFile(filepath string) (*VersionState, error) {
	b, err := os.ReadFile(filepath) //nolint:gosec
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

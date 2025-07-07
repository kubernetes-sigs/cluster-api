//go:build tools
// +build tools

/*
Copyright 2024 The Kubernetes Authors.

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

// main is the main package for the release notes generator.
package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"

	release "sigs.k8s.io/cluster-api/hack/tools/release/internal"
)

/*
This tool prints all the titles of all PRs in between to references.

Use these as the base of your release notes.
*/

const (
	alphaRelease     = "ALPHA RELEASE"
	betaRelease      = "BETA RELEASE"
	releaseCandidate = "RELEASE CANDIDATE"

	// Pre-release type constants.
	preReleaseAlpha = "alpha"
	preReleaseBeta  = "beta"
	preReleaseRC    = "rc"
)

func main() {
	cmd := newNotesCmd()
	if err := cmd.run(); err != nil {
		log.Fatal(err)
	}
}

type notesCmdConfig struct {
	repo                        string
	fromRef                     string
	toRef                       string
	newTag                      string
	branch                      string
	previousReleaseVersion      string
	prefixAreaLabel             bool
	deprecation                 bool
	addKubernetesVersionSupport bool
}

func readCmdConfig() *notesCmdConfig {
	config := &notesCmdConfig{}

	flag.StringVar(&config.repo, "repository", "kubernetes-sigs/cluster-api", "The repo to run the tool from.")
	flag.StringVar(&config.fromRef, "from", "", "The tag or commit to start from. It must be formatted as heads/<branch name> for branches and tags/<tag name> for tags. If not set, it will be calculated from release.")
	flag.StringVar(&config.toRef, "to", "", "The ref (tag, branch or commit to stop at. It must be formatted as heads/<branch name> for branches and tags/<tag name> for tags. If not set, it will default to branch.")
	flag.StringVar(&config.branch, "branch", "", "The branch to generate the notes from. If not set, it will be calculated from release.")
	flag.StringVar(&config.newTag, "release", "", "The tag for the new release.")
	flag.StringVar(&config.previousReleaseVersion, "previous-release-version", "", "The tag for the previous beta release.")

	flag.BoolVar(&config.prefixAreaLabel, "prefix-area-label", true, "If enabled, will prefix the area label.")
	flag.BoolVar(&config.deprecation, "deprecation", true, "If enabled, will add a templated deprecation warning header.")
	flag.BoolVar(&config.addKubernetesVersionSupport, "add-kubernetes-version-support", true, "If enabled, will add the Kubernetes version support header.")

	flag.Parse()

	return config
}

type notesCmd struct {
	config *notesCmdConfig
}

func newNotesCmd() *notesCmd {
	config := readCmdConfig()
	return &notesCmd{
		config: config,
	}
}

func (cmd *notesCmd) run() error {
	releaseType := releaseTypeFromNewTag(cmd.config.newTag)
	if err := validateConfig(cmd.config); err != nil {
		return err
	}

	if err := computeConfigDefaults(cmd.config); err != nil {
		return err
	}

	if err := ensureInstalledDependencies(); err != nil {
		return err
	}

	from, to := parseRef(cmd.config.fromRef), parseRef(cmd.config.toRef)

	var previousReleaseRef ref
	if cmd.config.previousReleaseVersion != "" {
		previousReleaseRef = parseRef(cmd.config.previousReleaseVersion)
	}

	printer := newReleaseNotesPrinter(cmd.config.repo, from.value)
	printer.releaseType = releaseType
	printer.printDeprecation = cmd.config.deprecation
	printer.printKubernetesSupport = cmd.config.addKubernetesVersionSupport

	generator := newNotesGenerator(
		newGithubFromToPRLister(cmd.config.repo, from, to, cmd.config.branch),
		newPREntryProcessor(cmd.config.prefixAreaLabel),
		printer,
		newDependenciesProcessor(cmd.config.repo, from.value, to.value),
	)

	return generator.run(previousReleaseRef)
}

func releaseTypeFromNewTag(newTagConfig string) string {
	// Error handling can be ignored as the version has been validated in computeConfigDefaults already.
	newTag, _ := semver.ParseTolerant(newTagConfig)

	// Ensures version format includes exactly two dot-separated pre-release identifiers (e.g., v1.7.0-beta.1).
	// Versions with a single pre-release identifier (e.g., v1.7.0-beta or v1.7.0-beta1) are NOT supported.
	// Return early if the count of pre-release identifiers is not 2.
	if len(newTag.Pre) != 2 {
		return ""
	}

	// Only allow alpha, beta and rc releases. More types must be defined here.
	// If a new type is not defined, no warning banner will be printed.
	switch newTag.Pre[0].VersionStr {
	case preReleaseAlpha:
		return alphaRelease
	case preReleaseBeta:
		return betaRelease
	case preReleaseRC:
		return releaseCandidate
	}
	return ""
}

func ensureInstalledDependencies() error {
	if !commandExists("gh") {
		return errors.New("gh GitHub CLI not available. GitHub CLI is required to be present in the PATH. Refer to https://cli.github.com/ for installation")
	}

	return nil
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func validateConfig(config *notesCmdConfig) error {
	if config.fromRef == "" && config.newTag == "" {
		return errors.New("at least one of --from or --release need to be set")
	}

	if config.branch == "" && config.newTag == "" {
		return errors.New("at least one of --branch or --release need to be set")
	}

	if config.fromRef != "" {
		if err := validateRef(config.fromRef); err != nil {
			return err
		}
	}

	if config.toRef != "" {
		if err := validateRef(config.toRef); err != nil {
			return err
		}
	}

	if config.previousReleaseVersion != "" {
		if err := validatePreviousReleaseVersion(config.previousReleaseVersion); err != nil {
			return err
		}
	}

	return nil
}

func validatePreviousReleaseVersion(previousReleaseVersion string) error {
	// Extract version string from ref format (e.g. "tags/v1.0.0-rc.1" -> "v1.0.0-rc.1")
	if !strings.Contains(previousReleaseVersion, "/") {
		return errors.New("--previous-release-version must be in ref format (e.g. tags/v1.0.0-rc.1)")
	}

	parts := strings.SplitN(previousReleaseVersion, "/", 2)
	if len(parts) != 2 {
		return errors.New("--previous-release-version must be in ref format (e.g. tags/v1.0.0-rc.1)")
	}

	versionStr := parts[1]

	// Parse the version to check if it contains alpha, beta, or rc
	version, err := semver.ParseTolerant(versionStr)
	if err != nil {
		return errors.Wrap(err, "invalid --previous-release-version, is not a valid semver")
	}

	// Check if the version has pre-release identifiers
	if len(version.Pre) == 0 {
		return errors.Errorf("--previous-release-version must contain '%s', '%s', or '%s' pre-release identifier", preReleaseAlpha, preReleaseBeta, preReleaseRC)
	}

	// Check if the first pre-release identifier is 'alpha', 'beta', or 'rc'
	preReleaseType := version.Pre[0].VersionStr
	if preReleaseType != preReleaseAlpha && preReleaseType != preReleaseBeta && preReleaseType != preReleaseRC {
		return errors.Errorf("--previous-release-version must contain '%s', '%s', or '%s' pre-release identifier", preReleaseAlpha, preReleaseBeta, preReleaseRC)
	}

	return nil
}

// computeConfigDefaults calculates the value the non specified configuration fields
// based on the set fields.
func computeConfigDefaults(config *notesCmdConfig) error {
	if config.fromRef != "" && config.branch != "" && config.toRef != "" {
		return nil
	}

	newTag, err := semver.ParseTolerant(config.newTag)
	if err != nil {
		return errors.Wrap(err, "invalid --release, is not a semver")
	}

	if config.fromRef == "" {
		if newTag.Patch == 0 {
			// If patch = 0, this a new minor release
			// Hence we want to read commits from
			config.fromRef = release.TagsPrefix + fmt.Sprintf("v%d.%d.0", newTag.Major, newTag.Minor-1)
		} else {
			// if not new minor release, this is a new patch, just decrease the patch
			config.fromRef = release.TagsPrefix + fmt.Sprintf("v%d.%d.%d", newTag.Major, newTag.Minor, newTag.Patch-1)
		}
	}

	if config.branch == "" {
		config.branch = defaultBranchForNewTag(newTag)
	}

	if config.toRef == "" {
		config.toRef = "heads/" + config.branch
	}

	return nil
}

// defaultBranchForNewTag calculates the branch to cut a release
// based on the new release tag.
func defaultBranchForNewTag(newTag semver.Version) string {
	if newTag.Patch == 0 {
		if len(newTag.Pre) == 0 {
			// for new minor releases, use the release branch
			return releaseBranchForVersion(newTag)
		} else if len(newTag.Pre) == 2 && newTag.Pre[0].VersionStr == "rc" && newTag.Pre[1].VersionNum >= 1 {
			// for the second or later RCs, we use the release branch since we cut this branch with the first RC
			return releaseBranchForVersion(newTag)
		}

		// for any other pre release, we always cut from main
		// this includes all beta releases and the first RC
		return "main"
	}

	// If it's a patch, we use the release branch
	return releaseBranchForVersion(newTag)
}

func releaseBranchForVersion(version semver.Version) string {
	return fmt.Sprintf("release-%d.%d", version.Major, version.Minor)
}

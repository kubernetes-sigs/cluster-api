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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/yaml"
)

func TestVersionChecker_newVersionChecker(t *testing.T) {
	g := NewWithT(t)

	versionChecker := newVersionChecker(test.NewFakeVariableClient())

	expectedStateFilePath := filepath.Join(homedir.HomeDir(), ".cluster-api", "version.yaml")
	g.Expect(versionChecker.versionFilePath).To(Equal(expectedStateFilePath))
	g.Expect(versionChecker.cliVersion).ToNot(BeNil())
	g.Expect(versionChecker.githubClient).ToNot(BeNil())
}

func TestVersionChecker(t *testing.T) {
	tests := []struct {
		name           string
		cliVersion     func() version.Info
		githubResponse string
		expectErr      bool
		expectedOutput string
	}{
		{
			name: "returns error if unable to parse cli version",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "bla-version",
				}
			},
			expectErr: true,
		},
		{
			name: "returns error if unable to parse latest version",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "bla-version"}`,
			expectErr:      true,
		},
		{
			name: "returns error if github request fails",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `bad-response`,
			expectErr:      false,
			// here we are printing out a log instead
			expectedOutput: "",
		},
		{
			name: "returns message if cli version is less than latest available release within minor version only",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v0.3.10", "html_url": "https://github.com/foo/bar/releases/v0.3.10"}`,
			expectErr:      false,
			expectedOutput: `
New clusterctl version available: v0.3.8 -> v0.3.10
https://github.com/foo/bar/releases/v0.3.10
`,
		},
		{
			name: "returns message if cli version is a dev build deriving from latest-1 available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.7-45-c1aeccb679cd56",
				}
			},
			githubResponse: `{"tag_name": "v0.3.8", "html_url": "https://github.com/foo/bar/releases/v0.3.8"}`,
			expectErr:      false,
			expectedOutput: `
New clusterctl version available: v0.3.7-45-c1aeccb679cd56 -> v0.3.8
https://github.com/foo/bar/releases/v0.3.8
`,
		},
		{
			name: "returns message if latest available release is a pre-release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v0.3.9-alpha.0", "html_url": "https://github.com/foo/bar/releases/v0.3.9-alpha.0"}`,
			expectErr:      false,
			expectedOutput: `
New clusterctl version available: v0.3.8 -> v0.3.9-alpha.0
https://github.com/foo/bar/releases/v0.3.9-alpha.0
`,
		},
		{
			name: "returns message if cli version is a pre-release of the latest available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.9-alpha.0",
				}
			},
			githubResponse: `{"tag_name": "v0.3.9", "html_url": "https://github.com/foo/bar/releases/v0.3.9"}`,
			expectErr:      false,
			expectedOutput: `
New clusterctl version available: v0.3.9-alpha.0 -> v0.3.9
https://github.com/foo/bar/releases/v0.3.9
`,
		},
		{
			name: "returns message if cli version is a pre-release and latest available release is the next pre-release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8-alpha.0",
				}
			},
			githubResponse: `{"tag_name": "v0.3.8-alpha.1", "html_url": "https://github.com/foo/bar/releases/v0.3.8-alpha.1"}`,
			expectErr:      false,
			expectedOutput: `
New clusterctl version available: v0.3.8-alpha.0 -> v0.3.8-alpha.1
https://github.com/foo/bar/releases/v0.3.8-alpha.1
`,
		},
		{
			name: "does not return message if cli version is a dirty build deriving from latest available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8-45-c1aeccb679cd56-dirty",
				}
			},
			githubResponse: `{"tag_name": "v0.3.8", "html_url": "https://github.com/foo/bar/releases/v0.3.8"}`,
			expectErr:      false,
			// here we are printing out a log instead
			expectedOutput: "",
		},
		{
			name: "does not return message if cli version is a dev build deriving from latest available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8-45-c1aeccb679cd56",
				}
			},
			githubResponse: `{"tag_name": "v0.3.8", "html_url": "https://github.com/foo/bar/releases/v0.3.8"}`,
			expectErr:      false,
			// here we are printing out a log instead
			expectedOutput: "",
		},
		{
			name: "does not return message if latest available version has a higher minor version than the cli version",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v0.4.0", "html_url": "https://github.com/foo/bar/releases/v0.4.0"}`,
			expectErr:      false,
			expectedOutput: "",
		},
		{
			name: "does not return message if latest available version has a higher major version and same minor version than the cli version",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v100.3.9", "html_url": "https://github.com/foo/bar/releases/v100.3.9"}`,
			expectErr:      false,
			// we are only prompting upgrade within minor versions of the cli
			// version
			expectedOutput: "",
		},
		{
			name: "does not return message if cli version is greater than latest available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v0.3.7"}`,
			expectErr:      false,
			expectedOutput: "",
		},
		{
			name: "does not return message if cli version is equal to latest available release",
			cliVersion: func() version.Info {
				return version.Info{
					GitVersion: "v0.3.8",
				}
			},
			githubResponse: `{"tag_name": "v0.3.8"}`,
			expectErr:      false,
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			tmpVersionFile, cleanDir := generateTempVersionFilePath(g)
			defer cleanDir()

			fakeGithubClient, mux, cleanup := test.NewFakeGitHub()
			// test.NewFakeGitHub and handler for returning a fake release
			mux.HandleFunc(
				"/repos/kubernetes-sigs/cluster-api/releases/latest",
				func(w http.ResponseWriter, r *http.Request) {
					g.Expect(r.Method).To(Equal("GET"))
					fmt.Fprint(w, tt.githubResponse)
				},
			)
			defer cleanup()
			versionChecker := newVersionChecker(test.NewFakeVariableClient())
			versionChecker.cliVersion = tt.cliVersion
			versionChecker.githubClient = fakeGithubClient
			versionChecker.versionFilePath = tmpVersionFile

			output, err := versionChecker.Check()

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(output).To(Equal(tt.expectedOutput))
		})
	}
}

func TestVersionChecker_WriteStateFile(t *testing.T) {
	g := NewWithT(t)
	fakeGithubClient, mux, cleanup := test.NewFakeGitHub()
	mux.HandleFunc(
		"/repos/kubernetes-sigs/cluster-api/releases/latest",
		func(w http.ResponseWriter, r *http.Request) {
			g.Expect(r.Method).To(Equal("GET"))
			fmt.Fprint(w, `{"tag_name": "v0.3.8", "html_url": "https://github.com/foo/bar/releases/v0.3.8"}`)
		},
	)
	defer cleanup()

	tmpVersionFile, cleanDir := generateTempVersionFilePath(g)
	defer cleanDir()

	versionChecker := newVersionChecker(test.NewFakeVariableClient())
	versionChecker.versionFilePath = tmpVersionFile
	versionChecker.githubClient = fakeGithubClient

	release, err := versionChecker.getLatestRelease()

	g.Expect(err).ToNot(HaveOccurred())
	// ensure that the state file has been created
	g.Expect(tmpVersionFile).Should(BeARegularFile())
	fb, err := os.ReadFile(tmpVersionFile)
	g.Expect(err).ToNot(HaveOccurred())
	var actualVersionState VersionState
	g.Expect(yaml.Unmarshal(fb, &actualVersionState)).To(Succeed())
	g.Expect(actualVersionState.LatestRelease).To(Equal(*release))
}

func TestVersionChecker_ReadFromStateFile(t *testing.T) {
	g := NewWithT(t)

	tmpVersionFile, cleanDir := generateTempVersionFilePath(g)
	defer cleanDir()

	fakeGithubClient1, mux1, cleanup1 := test.NewFakeGitHub()
	mux1.HandleFunc(
		"/repos/kubernetes-sigs/cluster-api/releases/latest",
		func(w http.ResponseWriter, r *http.Request) {
			g.Expect(r.Method).To(Equal("GET"))
			fmt.Fprint(w, `{"tag_name": "v0.3.8", "html_url": "https://github.com/foo/bar/releases/v0.3.8"}`)
		},
	)
	defer cleanup1()
	versionChecker := newVersionChecker(test.NewFakeVariableClient())
	versionChecker.versionFilePath = tmpVersionFile
	versionChecker.githubClient = fakeGithubClient1

	// this call to getLatestRelease will pull from our fakeGithubClient1 and
	// store the information including timestamp into the state file.
	_, err := versionChecker.getLatestRelease()
	g.Expect(err).ToNot(HaveOccurred())

	// override the github client with response to a new version v0.3.99
	var githubCalled bool
	fakeGithubClient2, mux2, cleanup2 := test.NewFakeGitHub()
	mux2.HandleFunc(
		"/repos/kubernetes-sigs/cluster-api/releases/latest",
		func(w http.ResponseWriter, r *http.Request) {
			githubCalled = true
			fmt.Fprint(w, `{"tag_name": "v0.3.99", "html_url": "https://github.com/foo/bar/releases/v0.3.99"}`)
		},
	)
	defer cleanup2()
	versionChecker.githubClient = fakeGithubClient2

	// now instead of making another call to github, we want to read from the
	// file. This will avoid unnecessary calls to github.
	release, err := versionChecker.getLatestRelease()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(release.Version).To(Equal("v0.3.8"))
	g.Expect(release.URL).To(Equal("https://github.com/foo/bar/releases/v0.3.8"))
	g.Expect(githubCalled).To(BeFalse())
}

func TestVersionChecker_ReadFromStateFileWithin24Hrs(t *testing.T) {
	g := NewWithT(t)

	tmpVersionFile, cleanDir := generateTempVersionFilePath(g)
	defer cleanDir()

	// this state file was generated more than 24 hours ago.
	reallyOldVersionState := &VersionState{
		LastCheck: time.Now().Add(-25 * time.Hour),
		LatestRelease: ReleaseInfo{
			Version: "v0.3.8",
			URL:     "https://github.com/foo/bar/releases/v0.3.8",
		},
	}
	g.Expect(writeStateFile(tmpVersionFile, reallyOldVersionState)).To(Succeed())

	fakeGithubClient1, mux1, cleanup1 := test.NewFakeGitHub()
	mux1.HandleFunc(
		"/repos/kubernetes-sigs/cluster-api/releases/latest",
		func(w http.ResponseWriter, r *http.Request) {
			g.Expect(r.Method).To(Equal("GET"))
			fmt.Fprint(w, `{"tag_name": "v0.3.10", "html_url": "https://github.com/foo/bar/releases/v0.3.10"}`)
		},
	)
	defer cleanup1()
	versionChecker := newVersionChecker(test.NewFakeVariableClient())
	versionChecker.versionFilePath = tmpVersionFile
	versionChecker.githubClient = fakeGithubClient1

	_, err := versionChecker.getLatestRelease()
	g.Expect(err).ToNot(HaveOccurred())

	// Since the state file is more that 24 hours old we want to retrieve the
	// latest release from github.
	release, err := versionChecker.getLatestRelease()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(release.Version).To(Equal("v0.3.10"))
	g.Expect(release.URL).To(Equal("https://github.com/foo/bar/releases/v0.3.10"))
}

func generateTempVersionFilePath(g *WithT) (string, func()) {
	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	// don't create the state file, just have a path to the file
	tmpVersionFile := filepath.Join(dir, "clusterctl", "state.yaml")

	return tmpVersionFile, func() {
		os.RemoveAll(dir)
	}
}

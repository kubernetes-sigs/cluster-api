//go:build tools
// +build tools

/*
Copyright 2023 The Kubernetes Authors.

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

// main is the main package for the open issues utility.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
)

const (
	baseURL = "https://api.github.com"
)

var (
	repoList = []string{
		"kubernetes-sigs/cluster-api-addon-provider-helm",
		"kubernetes-sigs/cluster-api-provider-aws",
		"kubernetes-sigs/cluster-api-provider-azure",
		"kubernetes-sigs/cluster-api-provider-cloudstack",
		"kubernetes-sigs/cluster-api-provider-digitalocean",
		"kubernetes-sigs/cluster-api-provider-gcp",
		"kubernetes-sigs/cluster-api-provider-kubemark",
		"kubernetes-sigs/cluster-api-provider-kubevirt",
		"kubernetes-sigs/cluster-api-provider-ibmcloud",
		"kubernetes-sigs/cluster-api-provider-nested",
		"oracle/cluster-api-provider-oci",
		"kubernetes-sigs/cluster-api-provider-openstack",
		"kubernetes-sigs/cluster-api-operator",
		"kubernetes-sigs/cluster-api-provider-packet",
		"tinkerbell/cluster-api-provider-tinkerbell",
		"kubernetes-sigs/cluster-api-provider-vsphere",
		"metal3-io/cluster-api-provider-metal3",
	}
)

// Issue is the struct for the issue.
type Issue struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// IssueResponse is the struct for the issue response.
type IssueResponse struct {
	HTMLURL string `json:"html_url"`
}

// releaseDetails is the struct for the release details.
type releaseDetails struct {
	ReleaseTag       string
	BetaTag          string
	ReleaseLink      string
	ReleaseDate      string
	ReleaseNotesLink string
}

// Example command:
//
//	GITHUB_ISSUE_OPENER_TOKEN="fake" RELEASE_TAG="v1.6.0-beta.0" RELEASE_DATE="2023-11-28" PROVIDER_ISSUES_DRY_RUN="true" make release-provider-issues-tool
func main() {
	githubToken, keySet := os.LookupEnv("GITHUB_ISSUE_OPENER_TOKEN")
	if !keySet || githubToken == "" {
		fmt.Println("GitHub personal access token is required.")
		fmt.Println("Refer to README.md in folder for more information.")
		os.Exit(1)
	}

	// always start in dry run mode unless explicitly set to false
	var dryRun bool
	isDryRun, dryRunSet := os.LookupEnv("PROVIDER_ISSUES_DRY_RUN")
	if !dryRunSet || isDryRun == "" || isDryRun != "false" {
		fmt.Printf("\n")
		fmt.Println("###############################################")
		fmt.Println("This script will run in dry run mode.")
		fmt.Println("To run it for real, set the PROVIDER_ISSUES_DRY_RUN environment variable to \"false\".")
		fmt.Println("###############################################")
		fmt.Printf("\n")
		dryRun = true
	} else {
		dryRun = false
	}

	fmt.Println("List of CAPI Providers:")
	fmt.Println("-", strings.Join(repoList, "\n- "))
	fmt.Printf("\n")

	// get release details
	details, releaseDetailsErr := getReleaseDetails()
	if releaseDetailsErr != nil {
		fmt.Println(releaseDetailsErr.Error())
		os.Exit(1)
	}

	// generate title
	titleBuffer := bytes.NewBuffer([]byte{})
	if err := getIssueTitle().Execute(titleBuffer, details); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// generate issue body
	issueBuffer := bytes.NewBuffer([]byte{})
	if err := getIssueBody().Execute(issueBuffer, details); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Issue Title:")
	fmt.Println(titleBuffer.String())
	fmt.Printf("\n")

	fmt.Println("Issue body:")
	fmt.Println(issueBuffer.String())

	// if dry run, exit
	if dryRun {
		fmt.Printf("\n")
		fmt.Println("DRY RUN: issue(s) body will not be posted.")
		fmt.Println("Exiting...")
		fmt.Printf("\n")
		os.Exit(0)
	}

	// else, ask for confirmation
	fmt.Printf("\n")
	fmt.Println("Issues will be posted to the provider repositories.")
	continueOrAbort()
	fmt.Printf("\n")

	var issuesCreated, issuedFailed []string
	for _, repo := range repoList {
		issue := Issue{
			Title: titleBuffer.String(),
			Body:  issueBuffer.String(),
		}

		issueJSON, err := json.Marshal(issue)
		if err != nil {
			fmt.Printf("Failed to marshal issue: %s\n", err)
			os.Exit(1)
		}

		url := fmt.Sprintf("%s/repos/%s/issues", baseURL, repo)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBuffer(issueJSON))
		if err != nil {
			fmt.Printf("Failed to create request: %s\n", err)
			os.Exit(1)
		}

		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", githubToken))
		req.Header.Set("X-Github-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "provider_issues")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Failed to send request: %s\n", err)
			os.Exit(1)
		}

		if resp.StatusCode != http.StatusCreated {
			fmt.Println("Failed to create issue for the repository:", repo, "Status:", resp.Status)
			issuedFailed = append(issuedFailed, repo)
		} else {
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Failed to read response body: %s\n", err)
				err := resp.Body.Close()
				if err != nil {
					fmt.Printf("Failed to close response body: %s\n", err)
				}
				os.Exit(1)
			}

			var issueResponse IssueResponse
			err = json.Unmarshal(responseBody, &issueResponse)
			if err != nil {
				fmt.Printf("Failed to unmarshal issue response: %s\n", err)
				err := resp.Body.Close()
				if err != nil {
					fmt.Printf("Failed to close response body: %s\n", err)
				}
				os.Exit(1)
			}

			fmt.Println("Issue created for repository:", repo)
			issuesCreated = append(issuesCreated, issueResponse.HTMLURL)
		}
		err = resp.Body.Close()
		if err != nil {
			fmt.Printf("Failed to close response body: %s\n", err)
		}
	}

	if len(issuesCreated) > 0 || len(issuedFailed) > 0 {
		fmt.Printf("\n")
		fmt.Println("###############################################")
		fmt.Println("Summary:")
		fmt.Println("###############################################")
	}

	if len(issuesCreated) > 0 {
		fmt.Printf("\n")
		fmt.Println("List of issues created:")
		fmt.Println("-", strings.Join(issuesCreated, "\n- "))
	}

	if len(issuedFailed) > 0 {
		fmt.Printf("\n")
		fmt.Println("List of issues failed to create:")
		fmt.Println("-", strings.Join(issuedFailed, "\n- "))
	}
}

// continueOrAbort asks the user to continue or abort the script.
func continueOrAbort() {
	fmt.Println("Continue? (y/n)")
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		fmt.Printf("Failed to read response: %s\n", err)
		os.Exit(1)
	}
	if response != "y" {
		fmt.Println("Aborting...")
		os.Exit(0)
	}
}

// getReleaseDetails returns the release details from the environment variables.
func getReleaseDetails() (releaseDetails, error) {
	// Parse the release tag
	releaseSemVer, keySet := os.LookupEnv("RELEASE_TAG")
	if !keySet || releaseSemVer == "" {
		return releaseDetails{}, errors.New("release tag is a required. Refer to README.md in folder for more information")
	}

	// allow patterns like v1.7.0-beta.0
	pattern := `^v\d+\.\d+\.\d+-beta\.\d+$`
	match, err := regexp.MatchString(pattern, releaseSemVer)
	if err != nil || !match {
		return releaseDetails{}, errors.New("release tag must be in format `^v\\d+\\.\\d+\\.\\d+-beta\\.\\d+$` e.g. v1.7.0-beta.0")
	}

	major, minor, patch := "", "", ""
	majorMinorPatchPattern := `v(\d+)\.(\d+)\.(\d+)`
	re := regexp.MustCompile(majorMinorPatchPattern)
	releaseSemVerMatch := re.FindStringSubmatch(releaseSemVer)
	if len(releaseSemVerMatch) > 3 {
		major = releaseSemVerMatch[1]
		minor = releaseSemVerMatch[2]
		patch = releaseSemVerMatch[3]
	} else {
		return releaseDetails{}, errors.New("release tag contains invalid Major.Minor.Patch SemVer. It must be in format v(\\d+)\\.(\\d+)\\.(\\d+) e.g. v1.7.0")
	}

	// Parse the release date
	releaseDate, keySet := os.LookupEnv("RELEASE_DATE")
	if !keySet || releaseDate == "" {
		return releaseDetails{}, errors.New("release date is a required environmental variable. Refer to README.md in folder for more information")
	}

	formattedReleaseDate, err := formatDate(releaseDate)
	if err != nil {
		return releaseDetails{}, errors.New("unable to parse the date")
	}

	majorMinorWithoutPrefixV := fmt.Sprintf("%s.%s", major, minor) // e.g. 1.7 . Note that there is no "v" in the majorMinor
	releaseTag := fmt.Sprintf("v%s.%s.%s", major, minor, patch)    // e.g. v1.7.0
	betaTag := fmt.Sprintf("%s%s", releaseTag, "-beta.0")          // e.g. v1.7.0-beta.0
	releaseLink := fmt.Sprintf("https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/release/releases/release-%s.md#timeline", majorMinorWithoutPrefixV)
	releaseNotesLink := fmt.Sprintf("https://github.com/kubernetes-sigs/cluster-api/releases/tag/%s", betaTag)

	return releaseDetails{
		ReleaseDate:      formattedReleaseDate,
		ReleaseTag:       releaseTag,
		BetaTag:          betaTag,
		ReleaseLink:      releaseLink,
		ReleaseNotesLink: releaseNotesLink,
	}, nil
}

// formatDate takes a date in ISO format i.e "2006-01-02" and returns it in the format "Monday 2nd January 2006".
func formatDate(inputDate string) (string, error) {
	layoutISO := "2006-01-02"
	parsedDate, err := time.Parse(layoutISO, inputDate)
	if err != nil {
		return "", err
	}

	// Get the day suffix
	day := parsedDate.Day()
	suffix := "th"
	if day%10 == 1 && day != 11 {
		suffix = "st"
	} else if day%10 == 2 && day != 12 {
		suffix = "nd"
	} else if day%10 == 3 && day != 13 {
		suffix = "rd"
	}

	weekDay := parsedDate.Weekday().String()
	month := parsedDate.Month().String()
	formattedDate := fmt.Sprintf("%s, %d%s %s %d", weekDay, day, suffix, month, parsedDate.Year())

	return formattedDate, nil
}

// getIssueBody returns the issue body template.
func getIssueBody() *template.Template {
	// do not indent the body
	// indenting the body will result in the body being posted as a code snippet
	issueBody, err := template.New("issue").Parse(`CAPI {{.BetaTag}} has been released and is ready for testing.
Looking forward to your feedback before {{.ReleaseTag}} release!

## For quick reference

<!-- body -->
- [CAPI {{.BetaTag}} release notes]({{.ReleaseNotesLink}})
- [Shortcut to CAPI git issues](https://github.com/kubernetes-sigs/cluster-api/issues)

## Following are the planned dates for the upcoming releases

CAPI {{.ReleaseTag}} will be released on **{{.ReleaseDate}}**.

More details of the upcoming schedule can be seen at [CAPI {{.ReleaseTag}} release timeline]({{.ReleaseLink}}).

<!-- [List of CAPI providers](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/role-handbooks/communications/README.md#communicate-beta-to-providers) -->
<!-- body -->
`)
	if err != nil {
		panic(err)
	}
	return issueBody
}

// getIssueTitle returns the issue title template.
func getIssueTitle() *template.Template {
	issueTitle, err := template.New("title").Parse(`CAPI {{.BetaTag}} has been released and is ready for testing`)
	if err != nil {
		panic(err)
	}
	return issueTitle
}

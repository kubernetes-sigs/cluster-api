/*
Copyright 2022 The Kubernetes Authors.

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

package goproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	defaultGoProxyHost = "proxy.golang.org"
)

var (
	retryableOperationInterval = 10 * time.Second
	retryableOperationTimeout  = 1 * time.Minute
)

// Client is a client to query versions from a goproxy instance.
type Client struct {
	scheme string
	host   string
}

// NewClient returns a new goproxyClient instance.
func NewClient(scheme, host string) *Client {
	return &Client{
		scheme: scheme,
		host:   host,
	}
}

// GetVersions returns the a sorted list of semantical versions which exist for a go module.
func (g *Client) GetVersions(ctx context.Context, gomodulePath string) (semver.Versions, error) {
	parsedVersions := semver.Versions{}

	majorVersionNumber := 1
	var majorVersion string
	for {
		if majorVersionNumber > 1 {
			majorVersion = fmt.Sprintf("v%d", majorVersionNumber)
		}
		rawURL := url.URL{
			Scheme: g.scheme,
			Host:   g.host,
			Path:   path.Join(gomodulePath, majorVersion, "@v", "/list"),
		}
		majorVersionNumber++

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL.String(), http.NoBody)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get versions: failed to create request")
		}

		var rawResponse []byte
		var responseStatusCode int
		var retryError error
		_ = wait.PollUntilContextTimeout(ctx, retryableOperationInterval, retryableOperationTimeout, true, func(ctx context.Context) (bool, error) {
			retryError = nil

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				retryError = errors.Wrapf(err, "failed to get versions: failed to do request")
				return false, nil
			}
			defer resp.Body.Close()

			responseStatusCode = resp.StatusCode

			// Status codes OK and NotFound are expected results:
			// * OK indicates that we got a list of versions to read.
			// * NotFound indicates that there are no versions for this module / modules major version.
			if responseStatusCode != http.StatusOK && responseStatusCode != http.StatusNotFound {
				retryError = errors.Errorf("failed to get versions: response status code %d", resp.StatusCode)
				return false, nil
			}

			// only read the response for http.StatusOK
			if responseStatusCode == http.StatusOK {
				rawResponse, err = io.ReadAll(resp.Body)
				if err != nil {
					retryError = errors.Wrap(err, "failed to get versions: error reading goproxy response body")
					return false, nil
				}
			}
			return true, nil
		})
		if retryError != nil {
			return nil, retryError
		}

		// Don't try to read the versions if status was not found.
		if responseStatusCode == http.StatusNotFound {
			break
		}

		for _, s := range strings.Split(string(rawResponse), "\n") {
			if s == "" {
				continue
			}
			parsedVersion, err := semver.ParseTolerant(s)
			if err != nil {
				// Discard releases with tags that are not a valid semantic versions (the user can point explicitly to such releases).
				continue
			}
			parsedVersions = append(parsedVersions, parsedVersion)
		}
	}

	if len(parsedVersions) == 0 {
		return nil, fmt.Errorf("no versions found for go module %q", gomodulePath)
	}

	sort.Sort(parsedVersions)

	return parsedVersions, nil
}

// GetSchemeAndHost detects and returns the scheme and host for goproxy requests.
// It returns empty strings if goproxy is disabled via `off` or `direct` values.
func GetSchemeAndHost(goproxy string) (string, string, error) {
	// Fallback to default
	if goproxy == "" {
		return "https", defaultGoProxyHost, nil
	}

	var goproxyHost, goproxyScheme string
	// xref https://github.com/golang/go/blob/master/src/cmd/go/internal/modfetch/proxy.go
	for goproxy != "" {
		var rawURL string
		if i := strings.IndexAny(goproxy, ",|"); i >= 0 {
			rawURL = goproxy[:i]
			goproxy = goproxy[i+1:]
		} else {
			rawURL = goproxy
			goproxy = ""
		}

		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		if rawURL == "off" || rawURL == "direct" {
			// Return nothing to fallback to github repository client without an error.
			return "", "", nil
		}

		// Single-word tokens are reserved for built-in behaviors, and anything
		// containing the string ":/" or matching an absolute file path must be a
		// complete URL. For all other paths, implicitly add "https://".
		if strings.ContainsAny(rawURL, ".:/") && !strings.Contains(rawURL, ":/") && !filepath.IsAbs(rawURL) && !path.IsAbs(rawURL) {
			rawURL = "https://" + rawURL
		}

		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return "", "", errors.Wrapf(err, "parse GOPROXY url %q", rawURL)
		}
		goproxyHost = parsedURL.Host
		goproxyScheme = parsedURL.Scheme
		// A host was found so no need to continue.
		break
	}

	return goproxyScheme, goproxyHost, nil
}

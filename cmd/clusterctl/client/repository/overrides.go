/*
Copyright 2019 The Kubernetes Authors.

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

package repository

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/drone/envsubst/v2"
	"github.com/pkg/errors"
	"k8s.io/client-go/util/homedir"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

const (
	overrideFolder    = "overrides"
	overrideFolderKey = "overridesFolder"
)

// Overrider provides behavior to determine the overrides layer.
type Overrider interface {
	Path() (string, error)
}

// overrides implements the Overrider interface.
type overrides struct {
	configVariablesClient config.VariablesClient
	providerLabel         string
	version               string
	filePath              string
}

type newOverrideInput struct {
	configVariablesClient config.VariablesClient
	provider              config.Provider
	version               string
	filePath              string
}

// newOverride returns an Overrider.
func newOverride(o *newOverrideInput) Overrider {
	return &overrides{
		configVariablesClient: o.configVariablesClient,
		providerLabel:         o.provider.ManifestLabel(),
		version:               o.version,
		filePath:              o.filePath,
	}
}

// Path returns the fully formed path to the file within the specified
// overrides config.
func (o *overrides) Path() (string, error) {
	basepath := filepath.Join(homedir.HomeDir(), config.ConfigFolder, overrideFolder)
	f, err := o.configVariablesClient.Get(overrideFolderKey)
	if err == nil && strings.TrimSpace(f) != "" {
		basepath = f

		basepath, err = envsubst.Eval(basepath, os.Getenv)
		if err != nil {
			return "", errors.Wrapf(err, "unable to evaluate %s: %q", overrideFolderKey, basepath)
		}

		if runtime.GOOS == "windows" {
			parsedBasePath, err := url.Parse(basepath)
			if err != nil {
				return "", errors.Wrapf(err, "unable to parse %s: %q", overrideFolderKey, basepath)
			}

			// in case of windows, we should take care of removing the additional / which is required by the URI standard
			// for windows local paths. see https://blogs.msdn.microsoft.com/ie/2006/12/06/file-uris-in-windows/ for more details.
			// Encoded file paths are not required in Windows 10 versions <1803 and are unsupported in Windows 10 >=1803
			// https://support.microsoft.com/en-us/help/4467268/url-encoded-unc-paths-not-url-decoded-in-windows-10-version-1803-later
			basepath = filepath.FromSlash(strings.TrimPrefix(parsedBasePath.Path, "/"))
		}
	}

	return filepath.Join(
		basepath,
		o.providerLabel,
		o.version,
		o.filePath,
	), nil
}

// getLocalOverride return local override file from the config folder, if it exists.
// This is required for development purposes, but it can be used also in production as a workaround for problems on the official repositories.
func getLocalOverride(info *newOverrideInput) ([]byte, error) {
	overridePath, err := newOverride(info).Path()
	if err != nil {
		return nil, err
	}

	// it the local override exists, use it
	_, err = os.Stat(overridePath)
	if err == nil {
		content, err := os.ReadFile(overridePath) //nolint:gosec
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read local override for %s", overridePath)
		}
		return content, nil
	}

	// it the local override does not exists, return (so files from the provider's repository could be used)
	if os.IsNotExist(err) {
		return nil, nil
	}

	// blocks for any other error
	return nil, err
}

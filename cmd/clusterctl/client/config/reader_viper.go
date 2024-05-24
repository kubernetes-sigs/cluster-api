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

package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

const (
	// ConfigFolder defines the old name of the config folder under $HOME.
	ConfigFolder = ".cluster-api"
	// ConfigFolderXDG defines the name of the config folder under $XDG_CONFIG_HOME.
	ConfigFolderXDG = "cluster-api"
	// ConfigName defines the name of the config file under ConfigFolderXDG.
	ConfigName = "clusterctl"
	// DownloadConfigFile is the config file when fetching the config from a remote location.
	DownloadConfigFile = "clusterctl-download.yaml"
)

// viperReader implements Reader using viper as backend for reading from environment variables
// and from a clusterctl config file.
type viperReader struct {
	configPaths []string
}

type viperReaderOption func(*viperReader)

func injectConfigPaths(configPaths []string) viperReaderOption {
	return func(vr *viperReader) {
		vr.configPaths = configPaths
	}
}

// newViperReader returns a viperReader.
func newViperReader(opts ...viperReaderOption) (Reader, error) {
	configDirectory, err := xdg.ConfigFile(ConfigFolderXDG)
	if err != nil {
		return nil, err
	}
	vr := &viperReader{
		configPaths: []string{configDirectory, filepath.Join(xdg.Home, ConfigFolder)},
	}
	for _, o := range opts {
		o(vr)
	}
	return vr, nil
}

// Init initialize the viperReader.
func (v *viperReader) Init(ctx context.Context, path string) error {
	log := logf.Log

	// Configure viper for reading environment variables as well, and more specifically:
	// AutomaticEnv force viper to check for an environment variable any time a viper.Get request is made.
	// It will check for a environment variable with a name matching the key uppercased; in case name use the - delimiter,
	// the SetEnvKeyReplacer forces matching to name use the _ delimiter instead (- is not allowed in linux env variable names).
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AllowEmptyEnv(true)
	viper.AutomaticEnv()

	if path != "" {
		url, err := url.Parse(path)
		if err != nil {
			return errors.Wrap(err, "failed to url parse the config path")
		}

		switch {
		case url.Scheme == "https" || url.Scheme == "http":
			var configDirectory string
			if len(v.configPaths) > 0 {
				configDirectory = v.configPaths[0]
			} else {
				configDirectory, err = xdg.ConfigFile(ConfigFolderXDG)
				if err != nil {
					return err
				}
			}

			downloadConfigFile := filepath.Join(configDirectory, DownloadConfigFile)
			err = downloadFile(ctx, url.String(), downloadConfigFile)
			if err != nil {
				return err
			}

			viper.SetConfigFile(downloadConfigFile)
		default:
			if _, err := os.Stat(path); err != nil {
				return errors.Wrap(err, "failed to check if clusterctl config file exists")
			}
			// Use path file from the flag.
			viper.SetConfigFile(path)
		}
	} else {
		// Checks if there is a default $XDG_CONFIG_HOME/cluster-api/clusterctl{.extension} or $HOME/.cluster-api/clusterctl{.extension} file
		if !v.checkDefaultConfig() {
			// since there is no default config to read from, just skip
			// reading in config
			log.V(5).Info("No default config file available")
			return nil
		}
		// Configure viper for reading $XDG_CONFIG_HOME/cluster-api/clusterctl{.extension} or $HOME/.cluster-api/clusterctl{.extension} file
		viper.SetConfigName(ConfigName)
		for _, p := range v.configPaths {
			viper.AddConfigPath(p)
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		return err
	}
	log.V(5).Info("Using configuration", "file", viper.ConfigFileUsed())
	return nil
}

func downloadFile(ctx context.Context, url string, filepath string) error {
	// Create the file
	out, err := os.Create(filepath) //nolint:gosec // No security issue: filepath is safe.
	if err != nil {
		return errors.Wrapf(err, "failed to create the clusterctl config file %s", filepath)
	}
	defer out.Close()

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	// Get the data
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrapf(err, "failed to download the clusterctl config file from %s: failed to create request", url)
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to download the clusterctl config file from %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download the clusterctl config file from %s got %d", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to save the data in the clusterctl config")
	}

	return nil
}

func (v *viperReader) Get(key string) (string, error) {
	if viper.Get(key) == nil {
		return "", errors.Errorf("Failed to get value for variable %q. Please set the variable value using os env variables or using the .clusterctl config file", key)
	}
	return viper.GetString(key), nil
}

func (v *viperReader) Set(key, value string) {
	viper.Set(key, value)
}

func (v *viperReader) UnmarshalKey(key string, rawval interface{}) error {
	return viper.UnmarshalKey(key, rawval)
}

// checkDefaultConfig checks the existence of the default config.
// Returns true if it finds a supported config file in the available config
// folders.
func (v *viperReader) checkDefaultConfig() bool {
	for _, path := range v.configPaths {
		for _, ext := range viper.SupportedExts {
			f := filepath.Join(path, fmt.Sprintf("%s.%s", ConfigName, ext))
			_, err := os.Stat(f)
			if err == nil {
				return true
			}
		}
	}
	return false
}

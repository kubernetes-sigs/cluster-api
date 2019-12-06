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
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
)

// viperReader implements Reader using viper as backend for reading from environment variables
// and from a clusterctl config file.
type viperReader struct {
}

// newViperReader returns a viperReader.
func newViperReader() Reader {
	return &viperReader{}
}

// Init initialize the viperReader.
func (v *viperReader) Init(path string) error {
	if path != "" {
		// Use path file from the flag.
		viper.SetConfigFile(path)
	} else {
		// Configure for searching cluster-api/.clusterctl{.extension} in home directory
		viper.SetConfigName(".clusterctl")
		viper.AddConfigPath(filepath.Join(homedir.HomeDir(), "cluster-api"))
	}

	// Configure for reading environment variables as well, and more specifically:
	// AutomaticEnv force viper to check for an environment variable any time a viper.Get request is made.
	// It will check for a environment variable with a name matching the key uppercased; in case name use the - delimiter,
	// the SetEnvKeyReplacer forces matching to name use the _ delimiter instead (- is not allowed in linux env variable names).
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)

	viper.AutomaticEnv()

	// If a path file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		klog.V(1).Infof("Using %q configuration file", viper.ConfigFileUsed())
	}

	return nil
}

func (v *viperReader) GetString(key string) (string, error) {
	if viper.Get(key) == nil {
		return "", errors.Errorf("Failed to get value for variable %q. Please set the variable value using os env variables or using the .clusterctl config file", key)
	}
	return viper.GetString(key), nil
}

func (v *viperReader) UnmarshalKey(key string, rawval interface{}) error {
	return viper.UnmarshalKey(key, rawval)
}

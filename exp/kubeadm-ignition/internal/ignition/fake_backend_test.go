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

package ignition

import (
	ignTypes "github.com/coreos/ignition/config/v2_2/types"
)

type FakeBackend struct {
}

func (factory *FakeBackend) getIgnitionConfigTemplate(node *Node) (*ignTypes.Config, error) {
	out := &ignTypes.Config{
		Ignition: ignTypes.Ignition{
			Version: IngitionSchemaVersion,
		},
	}
	return out, nil
}

func (factory *FakeBackend) applyConfig(config *ignTypes.Config) (*ignTypes.Config, error) {
	return config, nil
}

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

package topologymutation

import "strings"

// logRef is used to correctly render a reference with GroupVersionKind, Namespace and Name for both JSON and text logging.
type logRef struct {
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

func (l logRef) String() string {
	var parts []string
	for _, s := range []string{l.Group, l.Version, l.Kind, l.Namespace, l.Name} {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

type logRefWithoutStringFunc logRef

// MarshalLog ensures that loggers with support for structured output will log
// as a struct by removing the String method via a custom type.
func (l logRef) MarshalLog() interface{} {
	return logRefWithoutStringFunc(l)
}

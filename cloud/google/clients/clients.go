/*
Copyright 2018 The Kubernetes Authors.

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

package clients

import (
	"fmt"
	"strings"
)

const (
	projectNamePrefix = "projects/"
)

// Formats the given project id as a GCP consumer id, there is no validation done on the input
func GetConsumerIdForProject(projectId string) string {
	return fmt.Sprintf("project:%v", projectId)
}

func NormalizeProjectNameOrId(name string) string {
	if strings.HasPrefix(name, projectNamePrefix) {
		return name
	}
	return fmt.Sprintf("%v%v", projectNamePrefix, name)
}

/*
Copyright 2021 The Kubernetes Authors.

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

package docker

import "fmt"

// ContainerNotRunningError is returned when trying to patch a container that is not running.
type ContainerNotRunningError struct {
	Name string
}

// Error returns the error string.
func (cse ContainerNotRunningError) Error() string {
	return fmt.Sprintf("container with name %q is not running", cse.Name)
}

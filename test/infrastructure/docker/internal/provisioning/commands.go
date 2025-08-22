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

// Package provisioning deals with various machine initialization methods viz. cloud-init, Ignition,
// etc.
package provisioning

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// Cmd defines a shell command.
type Cmd struct {
	Cmd   string
	Args  []string
	Stdin string
}

// UnmarshalJSON a runcmd command
// It can be either a list or a string.
// If the item is a list, the head of the list is the command and the tail are the args.
// If the item is a string, the whole command will be wrapped in `/bin/sh -c`.
func (c *Cmd) UnmarshalJSON(data []byte) error {
	// First, try to decode the input as a list
	var s1 []string
	if err := json.Unmarshal(data, &s1); err != nil {
		if _, ok := err.(*json.UnmarshalTypeError); !ok {
			return errors.WithStack(err)
		}
	} else {
		c.Cmd = s1[0]
		c.Args = s1[1:]
		return nil
	}

	// If it's not a list, it must be a string
	var s2 string
	if err := json.Unmarshal(data, &s2); err != nil {
		return errors.WithStack(err)
	}

	c.Cmd = "/bin/sh"
	c.Args = []string{"-c", s2}

	return nil
}

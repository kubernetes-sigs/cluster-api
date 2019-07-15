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

package logger

import (
	"fmt"

	"github.com/go-logr/logr"
)

// Log is a wrapper to add a stacktrace to the Error message
type Log struct {
	logr.Logger
}

func (k Log) Error(err error, msg string, keysAndValues ...interface{}) {
	keysAndValues = append(keysAndValues, "stacktrace", fmt.Sprintf("%+v", err))
	k.Logger.Error(err, msg, keysAndValues...)
}

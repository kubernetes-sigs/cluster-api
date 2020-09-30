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

/*
Package log mirrors the controller runtime approach to logging, by defining a global logger
that defaults to NullLogger.

You can set a custom logger by calling log.SetLogger.

NewLogger returns a clusterctl friendly logr.Logger derived from
https://git.k8s.io/klog/klogr/klogr.go.

The logger is designed to print logs to stdout with a formatting that is easy to read for users
but also simple to parse for identifying specific values.

Note: the clusterctl library also support usage of other loggers as long as they conform to the github.com/go-logr/logr.Logger interface.

Following logging conventions are used in clusterctl library:

Message:

All messages should start with a capital letter.

Log level:

Use Level 0 (the default, if you don't specify a level) for the most important user feedback only, e.g.
- reporting command progress for long running actions
- reporting command results when required

Use logging Levels 1 providing more info about the command's internal workflow.

Use logging Levels 5 for for providing all the information required for debug purposes/problem investigation.

Logging WithValues:

Logging WithValues should be preferred to embedding values into log messages because it allows
machine readability.

Variables name should start with a capital letter.

Logging WithNames:

Logging WithNames should be used carefully.
Please consider that practices like prefixing the logs with something indicating which part of code
is generating the log entry might be useful for developers, but it can create confusion for
the end users because it increases the verbosity without providing information the user can understand/take benefit from.

Logging errors:

A proper error management should always be preferred to the usage of log.Error.
*/
package log

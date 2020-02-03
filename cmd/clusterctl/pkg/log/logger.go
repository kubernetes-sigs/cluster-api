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
package log implements a clusterctl friendly logr.Logger derived from
https://github.com/kubernetes/klog/blob/master/klogr/klogr.go.

The logger is designed to print logs to stdout with a formatting that is easy to read for users
but also simple to parse for identifying specific values.

Note: the clusterctl library also support usage of other loggers as long as they conform to the github.com/go-logr/logr.Logger interface.

Following logging conventions are used in clusterctl:

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

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// logEntry defines the information that can be used for composing a log line.
type logEntry struct {
	// Prefix of the log line, composed of the hierarchy of log.WithName values.
	Prefix string

	// Level of the LogEntry.
	Level int

	// Values of the log line, composed of the concatenation of log.WithValues and KeyValue pairs passed to log.Info.
	Values []interface{}
}

// Option is a configuration option supplied to NewLogger
type Option func(*logger)

// WithThreshold implements a New Option that allows to set the threshold level for a new logger.
// The logger will write only log messages with a level/V(x) equal or higher to the threshold.
func WithThreshold(threshold int) Option {
	return func(c *logger) {
		c.threshold = threshold
	}
}

// NewLogger returns a new instance of the clusterctl.
func NewLogger(options ...Option) logr.Logger {
	l := &logger{}
	for _, o := range options {
		o(l)
	}
	return l
}

// logger defines a clusterctl friendly logr.Logger
type logger struct {
	threshold int
	level     int
	prefix    string
	values    []interface{}
}

var _ logr.Logger = &logger{}

// Enabled tests whether this Logger is enabled.
func (l *logger) Enabled() bool {
	return l.level >= l.threshold
}

// Info logs a non-error message with the given key/value pairs as context.
func (l *logger) Info(msg string, kvs ...interface{}) {
	if l.Enabled() {
		values := copySlice(l.values)
		values = append(values, kvs...)
		values = append(values, "msg", msg)
		l.write(values)
	}
}

// Error logs an error message with the given key/value pairs as context.
func (l *logger) Error(err error, msg string, kvs ...interface{}) {
	values := copySlice(l.values)
	values = append(values, kvs...)
	values = append(values, "msg", msg, "error", err)
	l.write(values)
}

// V returns an InfoLogger value for a specific verbosity level.
func (l *logger) V(level int) logr.InfoLogger {
	nl := l.clone()
	nl.level = level
	return nl
}

// WithName adds a new element to the logger's name.
func (l *logger) WithName(name string) logr.Logger {
	nl := l.clone()
	if len(l.prefix) > 0 {
		nl.prefix = l.prefix + "/"
	}
	nl.prefix += name
	return nl
}

// WithValues adds some key-value pairs of context to a logger.
func (l *logger) WithValues(kvList ...interface{}) logr.Logger {
	nl := l.clone()
	nl.values = append(nl.values, kvList...)
	return nl
}

func (l *logger) write(values []interface{}) {
	entry := logEntry{
		Prefix: l.prefix,
		Level:  l.level,
		Values: values,
	}
	f, err := flatten(entry)
	if err != nil {
		panic(err)
	}
	fmt.Println(f)
}

func (l *logger) clone() *logger {
	return &logger{
		level:  l.level,
		prefix: l.prefix,
		values: copySlice(l.values),
	}
}

func copySlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	copy(out, in)
	return out
}

// flatten returns a human readable/machine parsable text representing the LogEntry.
// Most notable difference with the klog implementation are:
// - The message is printed at the beginning of the line, without the Msg= variable name e.g.
//   "Msg"="This is a message" --> This is a message
// - Variables name are not quoted, eg.
//   This is a message "Var1"="value" --> This is a message Var1="value"
// - Variables are not sorted, thus allowing full control to the developer on the output.
func flatten(entry logEntry) (string, error) {
	var msgValue string
	var errorValue error
	if len(entry.Values)%2 == 1 {
		return "", errors.New("log entry cannot have odd number off keyAndValues")
	}

	keys := make([]string, 0, len(entry.Values)/2)
	values := make(map[string]interface{}, len(entry.Values)/2)
	for i := 0; i < len(entry.Values); i += 2 {
		k, ok := entry.Values[i].(string)
		if !ok {
			panic(fmt.Sprintf("key is not a string: %s", entry.Values[i]))
		}
		var v interface{}
		if i+1 < len(entry.Values) {
			v = entry.Values[i+1]
		}
		switch k {
		case "msg":
			msgValue, ok = v.(string)
			if !ok {
				panic(fmt.Sprintf("the msg value is not of type string: %s", v))
			}
		case "error":
			errorValue, ok = v.(error)
			if !ok {
				panic(fmt.Sprintf("the error value is not of type error: %s", v))
			}
		default:
			if _, ok := values[k]; !ok {
				keys = append(keys, k)
			}
			values[k] = v
		}
	}
	str := ""
	if entry.Prefix != "" {
		str += fmt.Sprintf("[%s] ", entry.Prefix)
	}
	str += msgValue
	if errorValue != nil {
		if msgValue != "" {
			str += ": "
		}
		str += errorValue.Error()
	}
	for _, k := range keys {
		prettyValue, err := pretty(values[k])
		if err != nil {
			return "", err
		}
		str += fmt.Sprintf(" %s=%s", k, prettyValue)
	}
	return str, nil
}

func pretty(value interface{}) (string, error) {
	jb, err := json.Marshal(value)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to marshal %s", value)
	}
	return string(jb), nil
}

// UnstructuredToValues provides a utility function for creating values describing Unstructured objects. e.g.
// - Deployment="capd-controller-manager" Namespace="capd-system"  (<Kind>=<name> Namespace=<Namespace>)
// - CustomResourceDefinition="dockerclusters.infrastructure.cluster.x-k8s.io" (omit Namespace if it does not apply)
func UnstructuredToValues(obj unstructured.Unstructured) []interface{} {
	values := []interface{}{
		obj.GetKind(), obj.GetName(),
	}
	if obj.GetNamespace() != "" {
		values = append(values, "Namespace", obj.GetNamespace())
	}
	return values
}

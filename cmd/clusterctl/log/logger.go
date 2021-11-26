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

package log

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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

// Option is a configuration option supplied to NewLogger.
type Option func(*logger)

// WithThreshold implements a New Option that allows to set the threshold level for a new logger.
// The logger will write only log messages with a level/V(x) equal or higher to the threshold.
func WithThreshold(threshold *int) Option {
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
	return logr.New(l)
}

// logger defines a clusterctl friendly logr.Logger.
type logger struct {
	threshold *int
	level     int
	prefix    string
	values    []interface{}
}

var _ logr.LogSink = &logger{}

func (l *logger) Init(info logr.RuntimeInfo) {
}

// Enabled tests whether this Logger is enabled.
func (l *logger) Enabled(level int) bool {
	if l.threshold == nil {
		return true
	}
	return level <= *l.threshold
}

// Info logs a non-error message with the given key/value pairs as context.
func (l *logger) Info(level int, msg string, kvs ...interface{}) {
	if l.Enabled(level) {
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
func (l *logger) V(level int) logr.LogSink {
	nl := l.clone()
	nl.level = level
	return nl
}

// WithName adds a new element to the logger's name.
func (l *logger) WithName(name string) logr.LogSink {
	nl := l.clone()
	if len(l.prefix) > 0 {
		nl.prefix = l.prefix + "/"
	}
	nl.prefix += name
	return nl
}

// WithValues adds some key-value pairs of context to a logger.
func (l *logger) WithValues(kvList ...interface{}) logr.LogSink {
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
	fmt.Fprintln(os.Stderr, f)
}

func (l *logger) clone() *logger {
	return &logger{
		threshold: l.threshold,
		level:     l.level,
		prefix:    l.prefix,
		values:    copySlice(l.values),
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

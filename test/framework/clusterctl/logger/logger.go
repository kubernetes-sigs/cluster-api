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

// Package logger implements clusterctl logging functionality.
package logger

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// Provides a logr.Logger to use during e2e tests.

type logger struct {
	writer io.Writer
	values []interface{}
}

var _ logr.LogSink = &logger{}

func (l *logger) Init(info logr.RuntimeInfo) {
}

func (l *logger) Enabled(level int) bool {
	return true
}

func (l *logger) Info(level int, msg string, kvs ...interface{}) {
	values := copySlice(l.values)
	values = append(values, kvs...)
	values = append(values, "msg", msg)
	f, err := flatten(values)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(l.writer, f)
}

func (l *logger) Error(err error, msg string, kvs ...interface{}) {
	panic("using log.Error is deprecated in clusterctl")
}

func (l *logger) V(level int) logr.LogSink {
	nl := l.clone()
	return nl
}

func (l *logger) WithName(name string) logr.LogSink {
	panic("using log.WithName is deprecated in clusterctl")
}

func (l *logger) WithValues(kvList ...interface{}) logr.LogSink {
	nl := l.clone()
	nl.values = append(nl.values, kvList...)
	return nl
}

func (l *logger) clone() *logger {
	return &logger{
		writer: l.writer,
		values: copySlice(l.values),
	}
}

func copySlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	copy(out, in)
	return out
}

func flatten(values []interface{}) (string, error) {
	var msgValue string
	var errorValue error
	if len(values)%2 == 1 {
		return "", errors.New("log entry cannot have odd number off keyAndValues")
	}

	keys := make([]string, 0, len(values)/2)
	val := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		k, ok := values[i].(string)
		if !ok {
			panic(fmt.Sprintf("key is not a string: %s", values[i]))
		}
		var v interface{}
		if i+1 < len(values) {
			v = values[i+1]
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
			if _, ok := val[k]; !ok {
				keys = append(keys, k)
			}
			val[k] = v
		}
	}
	str := ""
	str += msgValue
	if errorValue != nil {
		if msgValue != "" {
			str += ": "
		}
		str += errorValue.Error()
	}
	for _, k := range keys {
		prettyValue, err := pretty(val[k])
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

// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const (
	format    = "%v, %v, %v, all eyes on me!"
	formatExp = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.* \[%s\]  \d, \d, \d, all eyes on me!`
)

var (
	a = []interface{}{1, 2, 3}
)

func TestMain(m *testing.M) {
	TestMode = true

	m.Run()
}

func ExampleTestLog() {
	Log(format, a...)
	// Output: 1, 2, 3, all eyes on me!
}

func TestLog(t *testing.T) {
	e := fmt.Sprintf(format, a...)
	g := captureLoggerOutput(Log, format, a)

	if strings.Compare(e, g) != 0 {
		t.Fatalf("Log should produce '%v' but produces: %v", e, g)
	}
}

func TestAlways(t *testing.T) {
	e, err := regexp.Compile(fmt.Sprintf(formatExp, AlwaysLabel))
	g := captureLoggerOutput(Always, format, a)

	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g) {
		t.Fatalf("Always should produce a pattern '%v' but produces: %v", e.String(), g)
	}
}

func TestCritical(t *testing.T) {
	Level = 1

	e, err := regexp.Compile(fmt.Sprintf(formatExp, CriticalLabel))
	g := captureLoggerOutput(Critical, format, a)

	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g) {
		t.Fatalf("Critical should produce a pattern '%v' but produces: %v", e.String(), g)
	}
}

func TestInfo(t *testing.T) {
	Level = 3

	e, err := regexp.Compile(fmt.Sprintf(formatExp, InfoLabel))
	g := captureLoggerOutput(Info, format, a)

	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g) {
		t.Fatalf("Info should produce a pattern '%v' but produces: %v", e.String(), g)
	}
}

func TestDebug(t *testing.T) {
	Level = 4

	e, err := regexp.Compile(fmt.Sprintf(formatExp, DebugLabel))
	g := captureLoggerOutput(Debug, format, a)

	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g) {
		t.Fatalf("Info should produce a pattern '%v' but produces: %v", e.String(), g)
	}
}

func TestWarning(t *testing.T) {
	Level = 2

	e, err := regexp.Compile(fmt.Sprintf(formatExp, WarningLabel))
	g := captureLoggerOutput(Warning, format, a)

	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g) {
		t.Fatalf("Info should produce a pattern '%v' but produces: %v", e.String(), g)
	}
}

func captureLoggerOutput(l Logger, format string, a []interface{}) string {
	b := new(bytes.Buffer)
	l(format, append(a, b)...)
	return b.String()
}

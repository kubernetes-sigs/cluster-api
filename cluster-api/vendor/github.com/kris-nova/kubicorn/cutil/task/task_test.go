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

package task

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/kris-nova/kubicorn/cutil/logger"
)

type tickerTickBounds struct {
	lower int
	upper int
}

const (
	description    = "Running a long-running worker task now:\n"
	descriptionExp = "^" + description + `\.{%v,%v}$`
	symbol         = "."

	workerDuration = 300 * time.Millisecond
	tickerDuration = 100 * time.Millisecond
)

var (
	ticker = time.NewTicker(tickerDuration)
)

func TestMain(m *testing.M) {
	logger.TestMode = true

	m.Run()
}

func TestRunAnnotated(t *testing.T) {
	var w Task = func() error {
		time.Sleep(workerDuration)
		return nil
	}

	g := new(bytes.Buffer)
	err := RunAnnotated(w, description, symbol, byteBufferLogger(logger.Log, g), ticker)
	if err != nil {
		t.Fatalf("RunAnnotated should return the tasks's nil error value but returned: %v", err)
	}

	// make sure that there is an upper bound to the number of symbols printed
	n := calcTickerTickBounds(workerDuration, tickerDuration)
	e, err := regexp.Compile(fmt.Sprintf(descriptionExp, n.lower, n.upper))
	if err != nil {
		t.Fatalf("Failed to compile regexp '%v': %v", e.String(), err)
	}

	if !e.MatchString(g.String()) {
		t.Fatalf("RunAnnotated should produce a pattern '%v' but produces: %v", e.String(), g.String())
	}
}

func byteBufferLogger(l logger.Logger, b *bytes.Buffer) logger.Logger {
	return func(format string, a ...interface{}) {
		l(format, append(a, b)...)
	}
}

func calcTickerTickBounds(workerDuration time.Duration, tickerDuration time.Duration) tickerTickBounds {
	n := (float64)(workerDuration) / (float64)(tickerDuration)
	return tickerTickBounds{
		lower: (int)(math.Floor(n)),
		upper: (int)(math.Ceil(n)),
	}
}

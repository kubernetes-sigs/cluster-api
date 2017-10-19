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
	"time"

	"github.com/kris-nova/kubicorn/cutil/logger"
)

type Task func() error

var (
	DefaultTicker = time.NewTicker(200 * time.Millisecond)
)

// RunAnnotated annotates a task with a description and a sequence of symbols indicating task activity until it terminates
func RunAnnotated(task Task, description string, symbol string, options ...interface{}) error {
	doneCh := make(chan bool)
	errCh := make(chan error)

	l := logger.Log
	t := DefaultTicker

	for _, o := range options {
		if value, ok := o.(logger.Logger); ok {
			l = value
		} else if value, ok := o.(*time.Ticker); ok {
			t = value
		}
	}

	go func() {
		errCh <- task()
	}()

	l(description)
	logActivity(symbol, l, t, doneCh)

	err := <-errCh
	doneCh <- true

	return err
}

// logs a sequence of symbols (one for each tick) indicating task activity until a quit is received
func logActivity(symbol string, logger logger.Logger, ticker *time.Ticker, quitCh <-chan bool) {
	go func() {
		for {
			select {
			case <-ticker.C:
				logger(symbol)
			case <-quitCh:
				ticker.Stop()
				return
			}
		}
	}()
}

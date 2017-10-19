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

// Package retry exposes retryable runner.
package retry

import (
	"fmt"
	"os"
	"time"

	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/signals"
)

// Retryable is an interface that implements retrier.
type Retryable interface {
	Try() error
}

// Retrier defines retryer properties.
type Retrier struct {
	retries      int
	sleepSeconds int
	retryable    Retryable
}

// NewRetrier creates a new Retrier using given properties.
func NewRetrier(retries, sleepSeconds int, retryable Retryable) *Retrier {
	return &Retrier{
		retries:      retries,
		sleepSeconds: sleepSeconds,
		retryable:    retryable,
	}
}

// RunRetry runs a retryable function.
func (r *Retrier) RunRetry() error {
	// Start signal handler.
	sigHandler := signals.NewSignalHandler(10)
	go sigHandler.Register()

	finish := make(chan bool, 1)
	go func() {
		select {
		case <-finish:
			return
		case <-time.After(10 * time.Second):
			return
		default:
			for {
				if sigHandler.GetState() != 0 {
					logger.Critical("detected signal. retry failed.")
					os.Exit(1)
				}
			}
		}
	}()

	for i := 0; i < r.retries; i++ {
		err := r.retryable.Try()
		if err != nil {
			logger.Info("Retryable error: %v", err)
			time.Sleep(time.Duration(r.sleepSeconds) * time.Second)
			continue
		}
		finish <- true
		return nil
	}

	finish <- true
	return fmt.Errorf("unable to succeed at retry after %d attempts at %d seconds", r.retries, r.sleepSeconds)
}

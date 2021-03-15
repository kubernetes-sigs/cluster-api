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

package alpha

import "context"

var (
	ctx = context.TODO()
)

// Client is the alpha client.
type Client interface {
	Rollout() Rollout
}

// alphaClient implements Client.
type alphaClient struct {
	rollout Rollout
}

// ensure alphaClient implements Client.
var _ Client = &alphaClient{}

// Option is a configuration option supplied to New.
type Option func(*alphaClient)

// InjectRollout allows to override the rollout implementation to use.
func InjectRollout(rollout Rollout) Option {
	return func(c *alphaClient) {
		c.rollout = rollout
	}
}

// New returns a Client.
func New(options ...Option) Client {
	return newAlphaClient(options...)
}

func newAlphaClient(options ...Option) *alphaClient {
	client := &alphaClient{}
	for _, o := range options {
		o(client)
	}

	// if there is an injected rollout, use it, otherwise use a default one
	if client.rollout == nil {
		client.rollout = newRolloutClient()
	}

	return client
}

func (c *alphaClient) Rollout() Rollout {
	return c.rollout
}

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

package resources

import (
	"github.com/kris-nova/kubicorn/cloud/azure/azureSDK"
)

type Shared struct {
	Identifier string
	Name       string
	Tags       map[string]string
}

var Sdk *azureSDK.Sdk

func s(st string) *string {
	return &st
}

func i64(in int64) *int64 {
	return &in
}
func i32(in int32) *int32 {
	return &in
}

func b(bo bool) *bool {
	return &bo
}

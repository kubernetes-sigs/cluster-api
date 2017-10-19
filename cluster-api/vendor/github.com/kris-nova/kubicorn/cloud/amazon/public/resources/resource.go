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
	"fmt"

	"github.com/kris-nova/kubicorn/cloud/amazon/awsSdkGo"
)

type Shared struct {
	Identifier string
	Name       string
	Tags       map[string]string
}

func S(format string, a ...interface{}) *string {
	str := fmt.Sprintf(format, a...)
	return &str
}

func I64(i int) *int64 {
	i64 := int64(i)
	return &i64
}

func B(b bool) *bool {
	return &b
}

var Sdk *awsSdkGo.Sdk

func init() {

}

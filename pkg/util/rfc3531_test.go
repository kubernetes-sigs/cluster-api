// Copyright © 2018 The Kubernetes Authors.
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

package util_test

import (
	"reflect"
	"strings"
	"testing"

	util "sigs.k8s.io/cluster-api/pkg/util"
)

func expectedLeftmost() []string {
	return []string{
		"10.3.0.0/21",
		"10.3.128.0/21",
		"10.3.64.0/21",
		"10.3.192.0/21",
		"10.3.32.0/21",
		"10.3.160.0/21",
		"10.3.96.0/21",
		"10.3.224.0/21",
		"10.3.16.0/21",
		"10.3.144.0/21",
		"10.3.80.0/21",
		"10.3.208.0/21",
		"10.3.48.0/21",
		"10.3.176.0/21",
		"10.3.112.0/21",
		"10.3.240.0/21",
		"10.3.8.0/21",
		"10.3.136.0/21",
		"10.3.72.0/21",
		"10.3.200.0/21",
		"10.3.40.0/21",
		"10.3.168.0/21",
		"10.3.104.0/21",
		"10.3.232.0/21",
		"10.3.24.0/21",
		"10.3.152.0/21",
		"10.3.88.0/21",
		"10.3.216.0/21",
		"10.3.56.0/21",
		"10.3.184.0/21",
		"10.3.120.0/21",
		"10.3.248.0/21",
	}
}

func expectedRightmost() []string {
	return []string{
		"10.3.248.0/21",
		"10.3.120.0/21",
		"10.3.184.0/21",
		"10.3.56.0/21",
		"10.3.216.0/21",
		"10.3.88.0/21",
		"10.3.152.0/21",
		"10.3.24.0/21",
		"10.3.232.0/21",
		"10.3.104.0/21",
		"10.3.168.0/21",
		"10.3.40.0/21",
		"10.3.200.0/21",
		"10.3.72.0/21",
		"10.3.136.0/21",
		"10.3.8.0/21",
		"10.3.240.0/21",
		"10.3.112.0/21",
		"10.3.176.0/21",
		"10.3.48.0/21",
		"10.3.208.0/21",
		"10.3.80.0/21",
		"10.3.144.0/21",
		"10.3.16.0/21",
		"10.3.224.0/21",
		"10.3.96.0/21",
		"10.3.160.0/21",
		"10.3.32.0/21",
		"10.3.192.0/21",
		"10.3.64.0/21",
		"10.3.128.0/21",
		"10.3.0.0/21",
	}
}

func expectedCentermost() []string {
	return []string{
		"10.3.0.0/21",
		"10.3.32.0/21",
		"10.3.64.0/21",
		"10.3.96.0/21",
		"10.3.16.0/21",
		"10.3.48.0/21",
		"10.3.80.0/21",
		"10.3.112.0/21",
		"10.3.128.0/21",
		"10.3.144.0/21",
		"10.3.160.0/21",
		"10.3.176.0/21",
		"10.3.192.0/21",
		"10.3.208.0/21",
		"10.3.224.0/21",
		"10.3.240.0/21",
		"10.3.8.0/21",
		"10.3.24.0/21",
		"10.3.40.0/21",
		"10.3.56.0/21",
		"10.3.72.0/21",
		"10.3.88.0/21",
		"10.3.104.0/21",
		"10.3.120.0/21",
		"10.3.136.0/21",
		"10.3.152.0/21",
		"10.3.168.0/21",
		"10.3.184.0/21",
		"10.3.200.0/21",
		"10.3.216.0/21",
		"10.3.232.0/21",
		"10.3.248.0/21",
	}
}

func TestRFC3531InvalidCIDR(t *testing.T) {
	_, err := util.AllocateRFC3531("10.3.0.0/16a", 21, util.LEFTMOST)
	if err == nil {
		t.Fatalf("Expected error with invalid CIDR")
	}
}

func TestRFC3531SizeTooLarge(t *testing.T) {
	_, err := util.AllocateRFC3531("10.3.0.0/16", 8, util.LEFTMOST)
	if err == nil {
		t.Fatalf("Expected error with subnet size too large")
	}
}

func TestRFC3531LeftMostAllocation(t *testing.T) {
	expected := expectedLeftmost()
	actual, _ := util.AllocateRFC3531("10.3.0.0/16", 21, util.LEFTMOST)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Did not get expected subnets\nExpected: %v\nActual: %v",
			strings.Join(expected, ", "),
			strings.Join(actual, ", "),
		)
	}
}

func TestRFC3531CenterMostAllocation(t *testing.T) {
	expected := expectedCentermost()
	actual, _ := util.AllocateRFC3531("10.3.0.0/16", 21, util.CENTERMOST)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Did not get expected subnets\nExpected: %v\nActual: %v",
			strings.Join(expected, ", "),
			strings.Join(actual, ", "),
		)
	}
}

func TestRFC3531RightMostAllocation(t *testing.T) {
	expected := expectedRightmost()
	actual, _ := util.AllocateRFC3531("10.3.0.0/16", 21, util.RIGHTMOST)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Did not get expected subnets\nExpected: %v\nActual: %v",
			strings.Join(expected, ", "),
			strings.Join(actual, ", "),
		)
	}
}

func TestRFC3531InvalidStrategy(t *testing.T) {
	_, err := util.AllocateRFC3531("10.3.0.0/16", 21, 10)
	if err == nil {
		t.Fatalf("Expected error with invalid strategy")
	}
}

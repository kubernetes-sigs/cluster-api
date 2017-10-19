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

package compare

import (
	"crypto/md5"
	"encoding/json"
	"fmt"

	"github.com/kris-nova/kubicorn/cutil/logger"
)

func IsEqual(actual, expected interface{}) (bool, error) {
	abytes, err := json.Marshal(actual)
	if err != nil {
		return false, fmt.Errorf("Recoverable error comparing JSON: %v", err)
	}
	ebytes, err := json.Marshal(expected)
	if err != nil {
		return false, fmt.Errorf("Recoverable error comparing JSON: %v", err)
	}
	ahash := md5.Sum(abytes)
	ehash := md5.Sum(ebytes)
	logger.Debug("Actual   : %x", ahash)
	logger.Debug("Expected : %x", ehash)
	alen := len(abytes)
	blen := len(ebytes)
	if alen != blen {
		return false, nil
	}
	for i := 0; i < alen; i++ {
		if abytes[i] != ebytes[i] {
			return false, nil
		}
	}
	return true, nil
}

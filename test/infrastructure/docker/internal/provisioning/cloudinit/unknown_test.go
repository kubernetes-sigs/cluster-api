/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestUnknown_Run(t *testing.T) {
	g := NewWithT(t)

	u := &unknown{
		lines: []string{},
	}
	lines, err := u.Commands()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(lines).To(HaveLen(0))
}

func TestUnknown_Unmarshal(t *testing.T) {
	g := NewWithT(t)

	u := &unknown{}
	expected := []string{"test 1", "test 2", "test 3"}
	input := `["test 1", "test 2", "test 3"]`

	g.Expect(u.Unmarshal([]byte(input))).To(Succeed())
	g.Expect(u.lines).To(Equal(expected))
}

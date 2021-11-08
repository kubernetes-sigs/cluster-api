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

package log

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

func TestFlatten(t *testing.T) {
	type args struct {
		prefix string
		kvList []interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "message without values",
			args: args{
				prefix: "",
				kvList: []interface{}{
					"msg", "this is a message",
				},
			},
			want: "this is a message",
		},
		{
			name: "message with values",
			args: args{
				prefix: "",
				kvList: []interface{}{
					"msg", "this is a message",
					"val1", 123,
					"val2", "string",
					"val3", "string with spaces",
				},
			},
			want: "this is a message val1=123 val2=\"string\" val3=\"string with spaces\"",
		},
		{
			name: "error without values",
			args: args{
				prefix: "",
				kvList: []interface{}{
					"msg", "this is a message",
					"error", errors.New("this is an error"),
				},
			},
			want: "this is a message: this is an error",
		},
		{
			name: "error with values",
			args: args{
				prefix: "",
				kvList: []interface{}{
					"msg", "this is a message",
					"error", errors.New("this is an error"),
					"val1", 123,
				},
			},
			want: "this is a message: this is an error val1=123",
		},
		{
			name: "message with prefix",
			args: args{
				prefix: "a\\b",
				kvList: []interface{}{
					"msg", "this is a message",
				},
			},
			want: "[a\\b] this is a message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := flatten(logEntry{
				Prefix: tt.args.prefix,
				Level:  0,
				Values: tt.args.kvList,
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

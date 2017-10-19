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
package fileresource

import (
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"testing"
)

func TestReadFromResource(t *testing.T) {
	//create local files
	err := os.Mkdir("./tomeDir", 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll("./tomeDir")

	fileContent := []byte("testing")

	// relative path file
	tmpfile, err := ioutil.TempFile("./tomeDir", "testfile")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(fileContent); err != nil {
		log.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		log.Fatal(err)
	}

	// absolute path file
	tmpfile2, err := ioutil.TempFile(os.TempDir(), "testfile")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile2.Name())

	if _, err := tmpfile2.Write(fileContent); err != nil {
		log.Fatal(err)
	}
	if err := tmpfile2.Close(); err != nil {
		log.Fatal(err)
	}

	testURL := "://raw.githubusercontent.com/mariomazo/test-files/master/test"

	tests := []struct {
		name     string
		resource string
		want     string
		wantErr  bool
		isEqual  bool
	}{
		//http valids
		{name: "https has file", resource: "https" + testURL, want: ("test\n"), wantErr: false, isEqual: true},
		{name: "https has file with SB scheme", resource: "HTTps" + testURL, want: ("test\n"), wantErr: false, isEqual: true},
		{name: "http has file", resource: "http" + testURL, want: ("test\n"), wantErr: false, isEqual: true},
		//http invalids
		{name: "http url is empty", resource: "", want: ("test\n"), wantErr: true, isEqual: false},
		{name: "http host is unknow", resource: "http://fdfdsjhfk88sfdas3989438.com/file", want: ("test\n"), wantErr: true, isEqual: false},
		{name: "http file not found", resource: "https://github.com/mariomazo/test-files/some404", want: ("test\n"), wantErr: true, isEqual: false},

		//local valids
		{name: "path relative has file", resource: tmpfile.Name(), want: ("testing"), wantErr: false, isEqual: true},
		{name: "path full has file", resource: tmpfile2.Name(), want: ("testing"), wantErr: false, isEqual: true},
		{name: "path relative dot has file", resource: "./" + tmpfile.Name(), want: ("testing"), wantErr: false, isEqual: true},

		//local invalids
		{name: "file not in path", resource: tmpfile.Name() + "fake", want: ("testing"), wantErr: true, isEqual: false},
		{name: "path is invaid", resource: "././tomeDir/testfile", want: ("test"), wantErr: true, isEqual: false},
		{name: "path is empty", resource: "", want: ("testing"), wantErr: true, isEqual: false},

		//extras
		{name: "protocol not known", resource: "s3://raw.githubusercontent.com/mariomazo/test-files/master/test", want: ("test\n"), wantErr: true, isEqual: false},
		{name: "file not found", resource: "/fjdshlkfjdhs9879879ee", want: ("test\n"), wantErr: true, isEqual: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFromResource(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFromResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) && tt.isEqual {
				t.Errorf("ReadFromResource() = %v, want %v", got, tt.want)
			}
		})
	}
}

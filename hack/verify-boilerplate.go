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

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

const (
	yearPlaceholder  = "YEAR"
	boilerPlateStart = "Copyright "
	boilerPlateEnd   = "limitations under the License."
)

var (
	supportedExt = []string{".go", ".py", ".sh"}
	yearRegexp   = regexp.MustCompile("(20)[0-9][0-9]")
	boilerPlate  = []string{
		boilerPlateStart + yearPlaceholder + " The Kubernetes Authors.",
		"",
		`Licensed under the Apache License, Version 2.0 (the "License");`,
		"you may not use this file except in compliance with the License.",
		"You may obtain a copy of the License at",
		"",
		"    http://www.apache.org/licenses/LICENSE-2.0",
		"",
		"Unless required by applicable law or agreed to in writing, software",
		`distributed under the License is distributed on an "AS IS" BASIS,`,
		"WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.",
		"See the License for the specific language governing permissions and",
		boilerPlateEnd,
	}
)

// trimLeadingComment strips a single line comment characters such as # or //
// at the exact beginning of a line, but also the first possible space character after it.
func trimLeadingComment(line, c string) string {
	if strings.Index(line, c) == 0 {
		x := len(c)
		if len(line) == x {
			return ""
		}
		if line[x] == byte(' ') {
			return line[x+1:]
		}
		return line[x:]
	}
	return line
}

// verifyFileExtension verifies if the file extensions is supported
func isSupportedFileExtension(filePath string) bool {
	// check if the file has an extension
	idx := strings.LastIndex(filePath, ".")
	if idx == -1 {
		return false
	}

	// check if the file has a supported extension
	ext := filePath[idx : idx+len(filePath)-idx]
	for _, e := range supportedExt {
		if e == ext {
			return true
		}
	}
	return false
}

// verifyBoilerplate verifies if a string contains the boilerplate
func verifyBoilerplate(contents string) error {
	idx := 0
	foundBoilerplateStart := false
	lines := strings.Split(contents, "\n")
	for _, line := range lines {
		// handle leading comments
		line = trimLeadingComment(line, "//")
		line = trimLeadingComment(line, "#")

		// find the start of the boilerplate
		bpLine := boilerPlate[idx]
		if strings.Contains(line, boilerPlateStart) {
			foundBoilerplateStart = true

			// validate the year of the copyright
			yearWords := strings.Split(line, " ")
			expectedLen := len(strings.Split(boilerPlate[0], " "))
			if len(yearWords) != expectedLen {
				return fmt.Errorf("copyright line should contain exactly %d words", expectedLen)
			}
			if !yearRegexp.MatchString(yearWords[1]) {
				return fmt.Errorf("cannot parse the year in the copyright line")
			}
			bpLine = strings.ReplaceAll(bpLine, yearPlaceholder, yearWords[1])
		}

		// match line by line
		if foundBoilerplateStart {
			if line != bpLine {
				return fmt.Errorf("boilerplate line %d does not match\nexpected: %q\ngot: %q", idx+1, bpLine, line)
			}
			idx++
			// exit after the last line is found
			if strings.Index(line, boilerPlateEnd) == 0 {
				break
			}
		}
	}

	if !foundBoilerplateStart {
		return fmt.Errorf("the file is missing a boilerplate")
	}
	if idx < len(boilerPlate) {
		return errors.New("boilerplate has missing lines")
	}
	return nil
}

// verifyFile verifies if a file contains the boilerplate
func verifyFile(filePath string) error {
	if len(filePath) == 0 {
		return errors.New("empty file name")
	}

	if !isSupportedFileExtension(filePath) {
		fmt.Printf("skipping %q: unsupported file type\n", filePath)
		return nil
	}

	// read the file
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return verifyBoilerplate(string(b))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: " +
			"go run verify-boilerplate.go <path-to-file> <path-to-file> ...")
		os.Exit(1)
	}

	hasErr := false
	for _, filePath := range os.Args[1:] {
		if err := verifyFile(filePath); err != nil {
			fmt.Printf("error validating %q: %v\n", filePath, err)
			hasErr = true
		}
	}
	if hasErr {
		os.Exit(1)
	}
}

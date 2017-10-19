// Copyright © 2017 Kris Nova <kris@nivenly.com>
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
//
//  _  ___
// | |/ / | ___  _ __   ___
// | ' /| |/ _ \| '_ \ / _ \
// | . \| | (_) | | | |  __/
// |_|\_\_|\___/|_| |_|\___|
//
// stdout.go holds various functions useful for controlling STDOUT messaging with klone

package local

import (
	"fmt"
	"github.com/fatih/color"
	"os"
)

func PrintPrompt(msg string) {
	color.Blue(msg)
}

func RecoverableErrorf(msg string, a ...interface{}) {
	b := fmt.Sprintf("[klone]: [Recoverable Error]:  %s", msg)
	color.Cyan(b, a)
}

func RecoverableError(msg string) {
	b := fmt.Sprintf("[klone]: [Recoverable Error]:  %s", msg)
	color.Cyan(b)
}

func PrintStartBanner() {
	color.Magenta(banner, Version)
}

func Print(msg string) {
	b := fmt.Sprintf("[klone]:  %s", msg)
	color.Green(b)
}

func Printf(format string, a ...interface{}) {
	b := fmt.Sprintf("[klone]:  %s", format)
	color.Green(b, a...)
}

func PrintExclaimf(format string, a ...interface{}) {
	b := fmt.Sprintf("[klone]:  %s", format)
	color.Cyan(b, a...)
}

func PrintExclaim(msg string) {
	b := fmt.Sprintf("[klone]:  %s", msg)
	color.Cyan(b)
}

func PrintError(err error) {
	color.Red("%v\n", err)
}

func PrintErrorExit(err error) {
	color.Red(err.Error())
	os.Exit(1)
}

func PrintFatal(msg string) {
	color.Red(msg)
	os.Exit(-1)
}

func PrintErrorExitCode(err error, code int) {
	color.Red(err.Error())
	fmt.Println()
	os.Exit(code)
}

const banner = `  _  ___
 | |/ / | ___  _ __   ___
 | ' /| |/ _ \| '_ \ / _ \   V%s
 | . \| | (_) | | | |  __/   Kris Nova ⚧
 |_|\_\_|\___/|_| |_|\___|   kris@nivenly.com
 --------------------------------------------
`

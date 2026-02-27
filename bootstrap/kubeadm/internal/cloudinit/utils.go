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
	"fmt"
	"strings"
	"text/template"
)

var (
	defaultTemplateFuncMap = template.FuncMap{
		"Indent":                 templateYAMLIndent,
		"CloudInitPartitionType": cloudInitPartitionType,
	}
)

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)
	return strings.Repeat(" ", i) + strings.Join(split, ident)
}

func cloudInitPartitionType(partitionType any) string {
	switch v := fmt.Sprint(partitionType); v {
	case "Linux":
		return "83"
	case "LinuxSwap":
		return "82"
	case "LinuxRaid":
		return "fd"
	case "LVM":
		return "8e"
	case "Fat32":
		return "0c"
	case "NTFS":
		return "07"
	case "LinuxExtended":
		return "85"
	default:
		// Preserve any raw cloud-init-compatible values to avoid breaking older input.
		return v
	}
}

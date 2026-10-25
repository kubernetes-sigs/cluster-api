/*
Copyright 2021 The Kubernetes Authors.

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

// This command line application runs verification steps for conversion types.
//
// The following checks are performed:
//   - For each API Kind and Group, only one storage version must exist.
//   - Each storage version type and its List counterpart, if there are multiple API versions,
//     the type MUST have a Hub() method.
//   - For each type with multiple versions, that has a Hub() and storage version,
//     the type MUST have ConvertFrom() and ConvertTo() methods.

// main is the main package for the conversion-verifier.
package main

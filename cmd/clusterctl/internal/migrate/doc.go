/*
Copyright 2025 The Kubernetes Authors.

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

// Package migrate provides functionality for migrating Cluster API resources
// between different API versions.
//
// This package implements a mechanical conversion approach for CAPI resource
// migration, currently supporting v1beta1 to v1beta2 conversion.

// Future enhancements:
// - Multi-version support with auto-detection of input API versions
// - Target version selection with --to-version flag support
// - Validation-only mode for linting existing resources

// The package is designed to be easily promoted from alpha status and
// extended to support additional API version migrations in the future.
package migrate

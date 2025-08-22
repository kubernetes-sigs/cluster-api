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

// Package release is the package for the release notes generator.
package release

// Common tags used by PRs.
const (
	// Features is the tag used for PRs that add new features.
	Features = ":sparkles: New Features"

	// Bugs is the tag used for PRs that fix bugs.
	Bugs = ":bug: Bug Fixes"

	// Documentation is the tag used for PRs that update documentation.
	Documentation = ":book: Documentation"

	// Proposals is the tag used for PRs that add new proposals.
	Proposals = ":memo: Proposals"

	// Warning is the tag used for PRs that add breaking changes.
	Warning = ":warning: Breaking Changes"

	// Other is the tag used for PRs that don't fit in any other category.
	Other = ":seedling: Others"

	// Unknown is the tag used for PRs that need to be sorted by hand.
	Unknown = ":question: Sort these by hand"
)

// TagsPrefix is the prefix used for from/to refs.
const TagsPrefix = "tags/"

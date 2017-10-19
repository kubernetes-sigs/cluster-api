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
// newprovider.go is how we New() different kloneprovider implementations
// Lives here to keep logic separated and prevent import cycles

package klone

import (
	"github.com/kris-nova/klone/pkg/provider"
	"github.com/kris-nova/klone/pkg/provider/github"
)

var RefreshCredentials = false

func NewGithubProvider() provider.KloneProvider {
	var kloner *github.KloneProvider
	github.RefreshCredentials = RefreshCredentials
	return kloner
}

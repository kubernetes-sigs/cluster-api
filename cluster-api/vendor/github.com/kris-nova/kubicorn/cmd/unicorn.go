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

package cmd

import "fmt"

var GitSha string
var Version string

var Unicorn = fmt.Sprintf(`-----------------------------------------------------------------------------------
                                                         ,/
                                                        //
                                                      ,//
                                          ___   /|   |//
                                       __/\_ --(/|___/-/
 The Kubicorn Authors               \|\_-\___ __-_ - /-/ \.
    Copyright 2017                |\_-___,-\_____--/_)' ) \
                                   \ -_ /     __ \(  ( __ \|
                                    \__|      |\)\ ) /(/|
           ,._____.,            ',--//-|      \  |  '   /
          /     __. \,          / /,---|       \       /
         / /    _. \  \         / _/ _,'        |     |
        |  | ( (  \   |      ,/\'__/'/          |     |
        |  \  \ --,  _/_------______/           \(   )/
        | | \  \_. \,                            \___/\
        | |  \_   \  \                                 \
        \ \    \_ \   \   /              Kubicorn       \
         \ \  \._  \__ \_|       |       v%s        \
          \ \___  \      \       |                        \
           \__ \__ \  \_ |       \                         |
           |  \_____ \  ____      |                        |
           | \  \__ ---' .__\     |        |               |
           \  \__ ---   /   )     |        \              /
            \   \____/ / ()(      \           ---_       /|
             \__________/(,--__    \_________.    |    ./ |
               |     \ \   ---_\--,           \   \_,./   |
               |      \  \_   \    / ---_______-\   \\    /
                \      \.___, |   /              \   \\   \
                 \     |  \_ \|   \              (   |:    |
                  \    \      \    |             /  / |    ;
                   \    \      \    \          (  _'   \  |
                    \.   \      \.   \           __/   |  |
                      \   \       \.  \                |  |
                       \   \        \  \               (  )
                        \   |        \  |              |  |
                         |  \         \ \              |  |  ----
                         ( __;        ( _;            ('-_';  --------
                         |___\        \___:            \___:   ---------------

----[ %s ]--------------------------------------------

Create, Manage, Image, and Scale Kubernetes infrastructure in the cloud.
`, Version, GitSha)

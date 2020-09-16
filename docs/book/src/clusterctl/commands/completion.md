# clusterctl completion

The `clusterctl completion` command outputs shell completion code for the
specified shell (bash or zsh). The shell code must be evaluated to provide
interactive completion of clusterctl commands. This can be done by sourcing it
from the `~/.bash_profile`.

## Bash

<aside class="note">

<h1>Note</h1>

This requires the bash-completion framework.

</aside>

To install it on macOS use Homebrew:
```
$ brew install bash-completion
```

Once installed, bash_completion must be evaluated. This can be done by adding
the following line to the `~/.bash_profile`.

```bash
[[ -r "$(brew --prefix)/etc/profile.d/bash_completion.sh" ]] && . "$(brew --prefix)/etc/profile.d/bash_completion.sh"
```

If bash-completion is not installed on Linux, please install the
'bash-completion' package via your distribution's package manager.

You now have to ensure that the clusterctl completion script gets sourced in
all your shell sessions. There are multiple ways to achieve this:

- Source the completion script in your `~/.bash_profile` file:
    ```
    source <(clusterctl completion bash)
    ```
- Add the completion script to the /usr/local/etc/bash_completion.d directory:
    ```
    clusterctl completion bash >/usr/local/etc/bash_completion.d/clusterctl
    ```

## Zsh

<aside class="note">

<h1>Note</h1>

Zsh completions are only supported in versions of zsh >= 5.2

</aside>

The clusterctl completion script for Zsh can be generated with the command
`clusterctl completion zsh`. Sourcing the completion script in your shell
enables clusterctl autocompletion.

To do so in all your shell sessions, add the following to your `~/.zshrc` file:
```sh
source <(clusterctl completion zsh)
```

After reloading your shell, clusterctl autocompletion should be working.

If you get an error like `complete:13: command not found: compdef`, then add
the following to the beginning of your `~/.zshrc` file:
```sh
autoload -Uz compinit
compinit
```

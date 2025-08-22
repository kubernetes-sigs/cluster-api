# clusterctl completion

The `clusterctl completion` command outputs shell completion code for the
specified shell (bash or zsh). The shell code must be evaluated to provide
interactive completion of clusterctl commands.

## Bash

<aside class="note">

<h1>Note</h1>

This requires the bash-completion framework.

</aside>

To install `bash-completion` on macOS, use Homebrew:

```bash
brew install bash-completion
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
    ```bash
    source <(clusterctl completion bash)
    ```
- Add the completion script to the /usr/local/etc/bash_completion.d directory:
    ```bash
    clusterctl completion bash >/usr/local/etc/bash_completion.d/clusterctl
    ```

## Zsh

<aside class="note">

<h1>Note</h1>

Zsh completions are only supported in versions of zsh >= 5.2

</aside>

The clusterctl completion script for Zsh can be generated with the command
`clusterctl completion zsh`.

If shell completion is not already enabled in your environment you will need to
enable it. You can execute the following once:

```zsh
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

To load completions for each session, execute once:

```zsh
clusterctl completion zsh > "${fpath[1]}/_clusterctl"
```

You will need to start a new shell for this setup to take effect.

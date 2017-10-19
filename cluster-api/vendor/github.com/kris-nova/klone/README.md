# About

[![Go Report Card](https://goreportcard.com/badge/github.com/kris-nova/klone)](https://goreportcard.com/report/github.com/kris-nova/klone)


`klone` is a command line tool that makes it easy to start working with a git repository.

`klone` can can check out an arbitrary github repository and run it in any Docker container you like.

<p align="center">
  <img src="doc/img/docker.png"> </image>
</p>


# Installing

```bash
go get -u github.com/kris-nova/klone
```

# Example (kloning in a Docker container)

A user can easily go from an arbitrary github repository, to a docker container of their choice.

```bash
klone docker -c golang:1.8.3
```

Will use `docker` to checkout the `github.com/docker/docker` repository on a `golang:1.8.3` Docker image.

`klone` will run itself inside the container and use the exact same logic it would use if running locally (which is described below).

A full walkthrough of using `klone` with [golang/dep](https://github.com/golang/dep) can be found [here](doc/dep-example.md).

# Example (local)

```bash
klone kubernetes
```

`klone` has built in logic to form possible repository information from a simple string.

For example `klone kubernetes` will attempt `github.com/kubernetes/kubernetes` and find the repository.

`klone` will also detect the programming language the repository is written in, and check the code out accordingly.

After a `klone` you should have the following `git remote -v` configuration

| Remote        | URL                                         |
| ------------- | ------------------------------------------- |
| origin        | git@github.com:`$you`/`$repo`               |
| upstream      | git@github.com:`$them`/`$repo`              |


# GitHub Credentials

Klone will prompt you the first time you use the program for needed credentials.
On a first run klone will cache local configuration to `~/.klone/auth` after it creates a token via the API.
Klone will *never* store your passwords in plaintext.

# Testing

Export `TEST_KLONE_GITHUBUSER` and `TEST_KLONE_GITHUBPASS` with a GitHub user/pass for a test account.
I use handy dandy @knovabot for my testing.

Run the test suite

```
make test
```

# Environmental variables

| Variable                              | Behaviour                                              |
| ------------------------------------- | ------------------------------------------------------ |
|KLONE_WORKSPACE                        | If set, klone will klone here for simple klones only   |
|KLONE_GITHUBTOKEN                      | GitHub acccess token to use with GitHub.com            |
|KLONE_GITHUBUSER                       | GitHub user name to authenticate with                  |
|KLONE_GITHUBPASS                       | GitHub password to authenticate with                   |
|TEST_KLONE_GITHUBTOKEN                 | (Testing) GitHub acccess token to use with GitHub.com  |
|TEST_KLONE_GITHUBUSER                  | (Testing) GitHub user name to authenticate with        |
|TEST_KLONE_GITHUBPASS                  | (Testing) GitHub password to authenticate with         |

# Passing environmental variables to containers

By default klone will automatically pass ALL environmental variables that begin with the prefix "KLONE_CONTAINER_" and will pass the variable WITHOUT the prefix.

For example setting `KLONE_CONTAINER_GOPATH=/my/gopath` will pass `GOPATH=/my/gopath` to the container.

Environmental variables cannot be passed in as command line flags.
# Using `Makefile`

Run `make help` for command line usage.

## Prerequisites

To be able to build `kubicorn` using Makefile you need to have configured Go 1.8 environment, which you can set up by following the official [install tutorial](https://golang.org/doc/install).
Before continuing, make sure that you can use `go` command from your shell.

### Building `kubicorn`

Details about the building proccess can be found in [BUILD docs](https://github.com/kris-nova/kubicorn/blob/master/docs/BUILD.md) and [INSTALL docs](https://github.com/kris-nova/kubicorn/blob/master/docs/INSTALL.md).

The following `make` commands are available for building `kubicorn`:
* `make` — parse Bootstrap scripts and create `kubicorn` executable in the `./bin` directory and the `AUTHORS` file.
* `make compile` — create the `kubicorn` executable in the `./bin` directory.
* `make install` — create the `kubicorn` executable in `$GOPATH/bin` directory.
* `make build` — clean the project tree and generate the `kubicorn` executables for 64-bit Linux, macOS, Windows and FreeBSD in the `./bin` directory, as well as generate the `AUTHORS` file.
* `make clean` — removes files from the `./bin` directory and the `bootstrap/bootstrap.go` file.

#### Additional building commands

* `make build-linux-amd64` — create the `kubicorn` executable for Linux 64-bit OS in the `./bin` directory. Requires Docker.
* `make build-darwin-amd64` — create the `kubicorn` executable for macOS in the `./bin` directory.
* `make build-freebsd-amd64` — create the `kubicorn` executable for FreeBSD 64-bit in the `./bin` directory.
* `make build-windows-amd64` — create the `kubicorn` executable for Windows 64-bit in the `./bin` directory.


### Miscellaneous

* `make authorsfile` — create the `AUTHORS` file.
* `make check-header` — verify all Go files to make sure they have license headers.
* `make headers` — add license headers to the Go files who are missing them.

### Formatting Go code

The following commands can be used to format and verify Go code:
* `make gofmt` — forma the all Go files using `go fmt`.
* `make lint` — check for style mistakes all Go files using `golint`.
* `make vet` — apply `go vet` to the all Go files.

### Testing

`make test` and `make ci` are used to run tests and E2E tests. More about testing can be found in the [`test` package](https://github.com/kris-nova/kubicorn/tree/master/test).

### `Makefile` verbose mode

By default, `make` will not show which commands it runs. To see what exactly is being run, you need to set `VERBOSE` variable, such as `VERBOSE=1 make`.
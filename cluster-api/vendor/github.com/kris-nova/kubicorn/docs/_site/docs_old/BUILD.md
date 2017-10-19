# Building Kubicorn

## Make

Run `make help` for command line usage.

First off you will need to have a working Golang-1.8 development environment. To get this you can follow [this](https://golang.org/doc/install) tutorial.
Make sure you can use the `go` command and that your `GOPATH` environment variable is set.
As a alternative you can have a look at the [Docker build script](#docker), you wont need `go` locally for this, but you will need `docker`.    

### Environment

Set `GOPATH` in your bash profile:
```bash
$ export GOPATH=/Users/<your user>/go
$ export PATH=$GOPATH/bin:$PATH
```
You can add these two lines to `~/.bash_profile` to make sure these variables are always.

If you have this you should be able to run the following command:

```bash
$ go get github.com/kris-nova/kubicorn
```

### Building
Now you can run `make` from the src directory of kubicorn:

```bash
$ cd $GOPATH/src/github.com/kris-nova/kubicorn/
$ make
```
The kubicorn binary will get built and placed under `$GOPATH/src/github.com/kris-nova/kubicorn/bin`. You can also run `make all` to also get the binary under `$GOPATH/bin`. 

Now you can run kubicorn to check if everything is working:

```bash
$ kubicorn
```

### Other branches or your fork
If you want to build a different git branch just switch inside the Kubicorn project:
```bash
$ cd $GOPATH/src/github.com/kris-nova/kubicorn/
$ git checkout digitalocean2
$ make
```

You can also fork the Kubicorn repository and make your own changes, check [this](https://help.github.com/articles/fork-a-repo/) link out on how to do this.
If you have your own fork you want to build you can just do a `go get` to your own repository instead of the main repository:

```bash
$ go get github.com/YOU_GITHUB_ACCOUNT/kubicorn
$ cd $GOPATH/src/github.com/YOU_GITHUB_ACCOUNT/kubicorn/
$ make
```

Keep in mind that other branches might have different dependencies that will need to present in you `GOPATH` directory.

## Docker

As a simple alternative there is a script located in the Docker folder name "build.sh". 
This can be used to build Kubicorn without the need to setup a golang environment on your local machine.
You will need to have Docker installed on your development environment.
This script should work on any platform that support Docker.
Have a look at the [official Docker documentation](https://docs.docker.com/engine/installation/.) on how to install Docker for your platform of choice.

To use this script just make a git checkout of the Kubicorn repository and run the build.sh:
```bash
$ git clone https://github.com/kris-nova/kubicorn.git
$ cd kubicorn/docker
$ ./build.sh 
```
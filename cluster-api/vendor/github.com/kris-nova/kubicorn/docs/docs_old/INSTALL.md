# Installing Kubicorn

**Note**: You can always run `make help` to learn more about the `Makefile`.

## Quickstart

If you have a working Golang environment the fastest way to install and run
kubicorn is:

```bash
$ go get github.com/kris-nova/kubicorn
$ ls $GOPATH/bin/kubicorn
/home/pczarkowski/development/go/bin/kubicorn
```

## Not so Quickstart

If you clone down this repo you can also build it with a local GO environment by running:

```bash
$ make all
$ ls $GOPATH/bin/kubicorn
/home/pczarkowski/development/go/bin/kubicorn
```

Otherwise you can try to build it in a Docker container:

> Note: we need to take ownership back from root (thanks Docker!) and rename
  it when we install it to $GOPATH/bin


```bash
$ make build-linux-amd64
$ sudo chown $USER:$USER bin/*
$ install --mode 0755 bin/linux-amd64 $GOPATH/bin/kubicorn
$ ls $GOPATH/bin/kubicorn
/home/pczarkowski/development/go/bin/kubicorn
```

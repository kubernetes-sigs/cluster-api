PKGS=$(shell go list ./... | grep -v /vendor)

default: compile
compile:
	go install .

build: clean build-linux-amd64 build-darwin-amd64 build-freebsd-amd64 build-windows-amd64

clean:
	rm -rf bin/*

gofmt:
	gofmt -w ./cmd
	gofmt -w ./pkg
	gofmt -w ./hack
	gofmt -w ./e2e
	gofmt -w ./doc

# Because of https://github.com/golang/go/issues/6376 We actually have to build this in a container
build-linux-amd64:
	docker run \
	-i -t \
	-w /go/src/github.com/kris-nova/klone \
	-v ${GOPATH}/src/github.com/kris-nova/klone:/go/src/github.com/kris-nova/klone \
	-v ${HOME}/.ssh:/root/.ssh \
	-e GOPATH=/go \
	--rm golang:1.8.1 make docker-build-linux-amd64

docker-build-linux-amd64:
	go build -v -o bin/linux-amd64

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -v -o bin/darwin-amd64 &

build-freebsd-amd64:
	GOOS=freebsd GOARCH=amd64 go build -v -o bin/freebsd-amd64 &

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build -v -o bin/windows-amd64 &

linux: shell
shell:
	docker run \
	-i -t \
	-w /go/src/github.com/kris-nova/klone \
	-v ${GOPATH}/src/github.com/kris-nova/klone:/go/src/github.com/kris-nova/klone \
	-v ${HOME}/.ssh:/root/.ssh \
	-e TEST_KLONE_GITHUBTOKEN=${TEST_KLONE_GITHUBTOKEN} \
	-e TEST_KLONE_GITHUBUSER=${TEST_KLONE_GITHUBUSER} \
	-e TEST_KLONE_GITHUBPASS=${TEST_KLONE_GITHUBPASS} \
	-e KLONE_GITHUBTOKEN=${KLONE_GITHUBTOKEN} \
	-e KLONE_GITHUBUSER=${KLONE_GITHUBUSER} \
	-e KLONE_GITHUBPASS=${KLONE_GITHUBPASS} \
	--rm golang:1.8.1 /bin/bash

test:
	@go test $(PKGS)
############################################################
# Dockerfile to build kubicorn container images
# Based on golang tip
############################################################

FROM golang:latest

# Create the default data directory
RUN go get github.com/kris-nova/kubicorn/...

WORKDIR /go/src/github.com/kris-nova/kubicorn/

# Runs appropriate make command based on environment
RUN  CGO_ENABLED=0 GOOS=linux  make docker-build-linux-amd64
#RUN GOOS=darwin make build-darwin-amd64
#RUN GOOS=windows make build-windows-amd64
#RUN GOOS=freebsd make build-freebsd-amd64 

# Multi-stage build docker image
FROM alpine:latest

# File Author / Maintainer
MAINTAINER Kris Nova <kris@nivenly.com>

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN	apk add --no-cache \
	ca-certificates

WORKDIR /root/
COPY --from=0 /go/src/github.com/kris-nova/kubicorn .

RUN echo "Image build complete."


CMD [ "./kubicorn" ]

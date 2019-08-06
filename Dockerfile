# Build the manager binary
FROM golang:1.12.7

# default the go proxy
ARG goproxy=https://proxy.golang.org

# run this with docker build --build_arg $(go env GOPROXY) to override the goproxy
ENV GOPROXY=$goproxy

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY kubeadm/ kubeadm/
COPY cloudinit/ cloudinit/

# Allow containerd to restart pods by calling /restart.sh (mostly for tilt + fast dev cycles)
# TODO: Remove this on prod and use a multi-stage build
COPY third_party/forked/rerun-process-wrapper/start.sh .
COPY third_party/forked/rerun-process-wrapper/restart.sh .

# Build and run
RUN go install -v .
RUN mv /go/bin/cluster-api-bootstrap-provider-kubeadm /manager
ENTRYPOINT ["./start.sh", "/manager"]

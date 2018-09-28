# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR $GOPATH/src/sigs.k8s.io/cluster-api
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager sigs.k8s.io/cluster-api/cmd/manager

# Copy the controller-manager into a thin image
# TODO: Build this on scratch
FROM debian:latest
WORKDIR /root/
COPY --from=builder /go/src/sigs.k8s.io/cluster-api/manager .
ENTRYPOINT ["./manager"]

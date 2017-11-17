all:
	go build
	cd machine-controller && go build
	cd examples/gce-machines-controller && go build
	cd examples/machines-client && go build
	cd examples/machines-controller && go build
	cd examples/machines-crd-installer && go build
	cd examples/machineset && go build

fmt:
	find -path './vendor' -prune -o -name '*.go' -exec gofmt -w {} \;

.PHONY: all fmt

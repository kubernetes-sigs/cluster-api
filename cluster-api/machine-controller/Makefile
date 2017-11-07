PROJECT=k8s-cluster-api
NAME=machine-controller
VERSION=0.1

staticbuild:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' .

image: staticbuild
	docker build -t "gcr.io/$(PROJECT)/$(NAME):$(VERSION)" .

push: image
	gcloud docker -- push "gcr.io/$(PROJECT)/$(NAME):$(VERSION)"

.PHONY: image push staticbuild

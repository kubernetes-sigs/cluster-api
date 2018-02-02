GCLOUD_PROJECT ?= $(shell /usr/bin/gcloud config get-value project)
NAME=machine-controller
VERSION=0.19

staticbuild:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' .

image: staticbuild
	docker build -t "gcr.io/$(GCLOUD_PROJECT)/$(NAME):$(VERSION)" .

push: image
	gcloud docker -- push "gcr.io/$(GCLOUD_PROJECT)/$(NAME):$(VERSION)"

fix-image-permissions:
	gsutil acl ch -u AllUsers:READ gs://artifacts.$(GCLOUD_PROJECT).appspot.com
	gsutil -m acl ch -r -u AllUsers:READ gs://artifacts.$(GCLOUD_PROJECT).appspot.com

.PHONY: image push staticbuild fix-image-permissions

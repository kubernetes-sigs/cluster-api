TAG=v1
IMAGE=gcr.io/mikedanese-k8s/install-k8s

build:
	docker build -t "$(IMAGE):$(TAG)" .

push: build
	docker push "$(IMAGE):$(TAG)"

.PHONY: build push

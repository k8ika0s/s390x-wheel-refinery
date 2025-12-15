REGISTRY ?= local
TAG ?= latest
ENGINE ?= podman

.PHONY: build-rocky build-fedora build-ubuntu

build-rocky:
	$(ENGINE) build -t $(REGISTRY)/refinery-rocky:$(TAG) -f containers/rocky/Dockerfile .

build-fedora:
	$(ENGINE) build -t $(REGISTRY)/refinery-fedora:$(TAG) -f containers/fedora/Dockerfile .

build-ubuntu:
	$(ENGINE) build -t $(REGISTRY)/refinery-ubuntu:$(TAG) -f containers/ubuntu/Dockerfile .

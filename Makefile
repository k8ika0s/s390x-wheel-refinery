REGISTRY ?= local
TAG ?= latest
ENGINE ?= podman
COMPOSE_FILE ?= podman-compose.yml

.PHONY: build-rocky build-fedora build-ubuntu build-builder build-images prep-dirs up

build-rocky:
	$(ENGINE) build -t $(REGISTRY)/refinery-rocky:$(TAG) -f containers/rocky/Containerfile .

build-fedora:
	$(ENGINE) build -t $(REGISTRY)/refinery-fedora:$(TAG) -f containers/fedora/Containerfile .

build-ubuntu:
	$(ENGINE) build -t $(REGISTRY)/refinery-ubuntu:$(TAG) -f containers/ubuntu/Containerfile .

build-builder:
	$(ENGINE) build -t $(REGISTRY)/refinery-builder:$(TAG) -f containers/refinery-builder/Containerfile .

build-images: build-builder
	$(ENGINE) build -t $(REGISTRY)/refinery-control-plane:$(TAG) -f containers/go-control-plane/Containerfile .
	$(ENGINE) build -t $(REGISTRY)/refinery-worker:$(TAG) -f containers/go-worker/Containerfile .
	$(ENGINE) build -t $(REGISTRY)/refinery-ui:$(TAG) -f containers/ui/Containerfile .

prep-dirs:
	mkdir -p input output cache

up: prep-dirs
	$(ENGINE) compose -f $(COMPOSE_FILE) up

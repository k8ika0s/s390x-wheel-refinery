REGISTRY ?= local
TAG ?= latest
ENGINE ?= podman
COMPOSE_FILE ?= podman-compose.yml
DEV_IMAGE ?= ghcr.io/k8ika0s/s390x-wheel-refinery-dev:latest

.PHONY: build-rocky build-fedora build-ubuntu build-builder-base build-worker-base build-builder build-images build-devcontainer prep-dirs up build-stack-hostnet publish-builder-image stack-up-no-build stack-up-core-no-build stack-up-hostnet-no-build stack-diagnostics

build-rocky:
	$(ENGINE) build -t $(REGISTRY)/refinery-rocky:$(TAG) -f containers/rocky/Containerfile .

build-fedora:
	$(ENGINE) build -t $(REGISTRY)/refinery-fedora:$(TAG) -f containers/fedora/Containerfile .

build-ubuntu:
	$(ENGINE) build -t $(REGISTRY)/refinery-ubuntu:$(TAG) -f containers/ubuntu/Containerfile .

build-builder-base:
	$(ENGINE) build -t localhost/s390x-wheel-refinery_builder-base:ubi8 -f containers/refinery-builder-base/Containerfile .

build-worker-base:
	$(ENGINE) build -t localhost/s390x-wheel-refinery_worker-base:ubi8 -f containers/go-worker-base/Containerfile .

build-builder:
	$(ENGINE) build -t $(REGISTRY)/refinery-builder:$(TAG) --build-arg BUILDER_BASE_IMAGE=localhost/s390x-wheel-refinery_builder-base:ubi8 -f containers/refinery-builder/Containerfile .

build-images: build-builder-base build-worker-base build-builder
	$(ENGINE) build -t $(REGISTRY)/refinery-control-plane:$(TAG) -f containers/go-control-plane/Containerfile .
	$(ENGINE) build -t $(REGISTRY)/refinery-worker:$(TAG) --build-arg WORKER_BASE_IMAGE=localhost/s390x-wheel-refinery_worker-base:ubi8 -f containers/go-worker/Containerfile .
	$(ENGINE) build -t $(REGISTRY)/refinery-ui:$(TAG) -f containers/ui/Containerfile .

build-devcontainer:
	$(ENGINE) build -t $(DEV_IMAGE) -f .devcontainer/Containerfile .

prep-dirs:
	mkdir -p input output cache cache/cas cache/pip cache/plans

up: prep-dirs
	$(ENGINE) compose -f $(COMPOSE_FILE) up

build-stack-hostnet:
	./scripts/build-stack-images-hostnet.sh

publish-builder-image:
	./scripts/publish-builder-image.sh

stack-up-no-build:
	./scripts/stack-up-no-build.sh

stack-up-core-no-build:
	COMPOSE_FILE=podman-compose.core.yml ./scripts/stack-up-no-build.sh

stack-up-hostnet-no-build:
	COMPOSE_FILE=podman-compose.hostnet.yml ./scripts/stack-up-no-build.sh

stack-diagnostics:
	./scripts/stack-diagnostics.sh

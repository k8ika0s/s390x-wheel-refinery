REGISTRY ?= local
TAG ?= latest

.PHONY: build-rocky build-fedora build-ubuntu

build-rocky:
	docker build -t $(REGISTRY)/refinery-rocky:$(TAG) -f containers/rocky/Dockerfile containers/rocky

build-fedora:
	docker build -t $(REGISTRY)/refinery-fedora:$(TAG) -f containers/fedora/Dockerfile containers/fedora

build-ubuntu:
	docker build -t $(REGISTRY)/refinery-ubuntu:$(TAG) -f containers/ubuntu/Dockerfile containers/ubuntu

.PHONY: build-web
build-web:
	docker build -t $(REGISTRY)/refinery-web:$(TAG) -f containers/web/Dockerfile .

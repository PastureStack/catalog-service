TARGETS := $(shell ls scripts)
DAPPER_IMAGE ?= pasturestack-catalog-service-dapper:ubuntu26
DAPPER_SOURCE ?= /go/src/github.com/PastureStack/catalog-service

.dapper-image: Dockerfile.dapper
	docker build \
		--build-arg DAPPER_HOST_ARCH=$${DAPPER_HOST_ARCH:-amd64} \
		-t $(DAPPER_IMAGE) \
		-f Dockerfile.dapper .

$(TARGETS): .dapper-image
	docker run --rm \
		-v $(CURDIR):$(DAPPER_SOURCE) \
		-e DAPPER_UID=$$(id -u) \
		-e DAPPER_GID=$$(id -g) \
		-e ARCH=$${ARCH:-amd64} \
		-e VERSION_OVERRIDE \
		$(DAPPER_IMAGE) $@

trash:
	@echo "Dependencies are vendored; no external dependency fetch is required."

trash-keep: trash

deps: trash

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS) deps trash trash-keep

DOCKER ?= docker

BUILD_IMAGE_TAG ?= 1.26
BUILD_IMAGE ?= registry.suse.com/bci/golang:$(BUILD_IMAGE_TAG)

COMMIT_SHA := $(shell git rev-parse --short HEAD)
HARVESTER_VERSION ?= 1.9
CLI_VERSION ?= $(HARVESTER_VERSION)+$(COMMIT_SHA)

IMAGE_NAME ?= hperf
IMAGE_TAG ?= $(HARVESTER_VERSION)-$(COMMIT_SHA)
IMAGE_PLATFORMS ?= linux/amd64
IMAGE_OUTPUT_TYPE ?= docker

# these apply to the 'compile' target only. the 'image' target pins its own
# architecture by passing $(IMAGE_PLATFORMS) to buildx
GOOS ?= linux
GOARCH ?= amd64

# host cache locations to be shared with the build container so that dependencies
# and build artifacts survive between runs
HOST_GOCACHE := $(shell go env GOCACHE)
HOST_GOMODCACHE := $(shell go env GOMODCACHE)

# propagate host ownership into the container so that anything written to the
# mounted caches, or to bin/, stays owned by the host user
HOST_UID := $(shell id -u)
HOST_GID := $(shell id -g)

DOCKER_RUN = $(DOCKER) run --rm \
	--user $(HOST_UID):$(HOST_GID) \
	--mount type=bind,src=$(CURDIR),dst=/go/src \
	--mount type=bind,src=$(HOST_GOCACHE),dst=/tmp/go-build \
	--mount type=bind,src=$(HOST_GOMODCACHE),dst=/tmp/go-mod \
	--workdir /go/src \
	--env HOME=/tmp \
	--env GOCACHE=/tmp/go-build \
	--env GOMODCACHE=/tmp/go-mod

compile:
	mkdir -p bin
	$(DOCKER_RUN) \
		--mount type=bind,src=$(CURDIR)/bin,dst=/out \
		--env GOOS=$(GOOS) \
		--env GOARCH=$(GOARCH) \
		--env CGO_ENABLED="0" \
		--env GOFLAGS="-trimpath" \
		$(BUILD_IMAGE) bash -c "go build -o /out/hperf ."

test:
	$(DOCKER_RUN) $(BUILD_IMAGE) bash -c "go test -cover -race -shuffle=on ./..."

image:
	$(DOCKER) build --rm --pull \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg CLI_VERSION=$(CLI_VERSION) \
		--output=type=$(IMAGE_OUTPUT_TYPE) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) .

clean:
	rm -rf bin

.PHONY: compile test image clean

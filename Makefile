HARVESTER_VERSION ?= 1.9
COMMIT_SHA := $(shell git rev-parse --short HEAD)
CLI_VERSION ?= $(HARVESTER_VERSION)+$(COMMIT_SHA)
ETCD_VERSION ?= v3.6.14
BCI_GO_TAG ?= 1.26

# use BCL golang image to compile the CLI and run unit test
DOCKER ?= docker

IMAGE_NAME ?= hvperf
IMAGE_TAG ?= $(HARVESTER_VERSION)-$(COMMIT_SHA)
IMAGE_PLATFORMS ?= linux/amd64
DIST_DIR := dist

go/build:
	$(DOCKER) build --rm --pull \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg BCI_GO_TAG=$(BCI_GO_TAG) \
		--build-arg CLI_VERSION=$(CLI_VERSION) \
		--output=type=local,dest=$(DIST_DIR) \
		--target=hvperf-dist \
		.

go/test:
	$(DOCKER) build --rm --pull \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg BCI_GO_TAG=$(BCI_GO_TAG) \
		--target=hvperf-test \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		.

tools:
	$(DOCKER) build --rm --pull \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg BCI_GO_TAG=$(BCI_GO_TAG) \
		--build-arg CLI_VERSION=$(CLI_VERSION) \
		--build-arg ETCD_VERSION=$(ETCD_VERSION) \
		--output=type=local,dest=$(DIST_DIR) \
		--target=tools-dist \
		-t $(IMAGE_NAME):$(IMAGE_TAG) .

.PHONY: go/build go/test tools

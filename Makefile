HARVESTER_VERSION ?= 1.9

BCI_GO_TAG ?= 1.26
CLI_VERSION ?= $(HARVESTER_VERSION)+$(COMMIT_SHA)
COMMIT_SHA := $(shell git rev-parse --short HEAD)
ETCD_VERSION ?= v3.6.14
IPERF3_VERSION ?= 3.22
NETPERF_VERSION ?= afc51ff9764741da4ed6702651fba9d9c23f8557

# use BCL golang image to compile the CLI and run unit test
DOCKER ?= docker

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
		.

tools:
	$(DOCKER) build --rm --pull \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg BCI_GO_TAG=$(BCI_GO_TAG) \
		--build-arg CLI_VERSION=$(CLI_VERSION) \
		--build-arg ETCD_VERSION=$(ETCD_VERSION) \
		--build-arg IPERF3_VERSION=$(IPERF3_VERSION) \
		--build-arg NETPERF_VERSION=$(NETPERF_VERSION) \
		--output=type=local,dest=$(DIST_DIR) \
		--target=tools-dist \
		.

.PHONY: go/build go/test tools

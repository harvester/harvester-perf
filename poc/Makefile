IMAGE       ?= harvester-perf
TAG         ?= dev
KUBECONFIG_ ?= $(HOME)/.kube/config
RESULTS     ?= $(CURDIR)/results
DOCKER      ?= docker
RUN_FLAGS   ?= --rm --network host \
               -v $(KUBECONFIG_):/root/.kube/config:ro \
               -v $(RESULTS):/results \
               -e PERF_UID=$(shell id -u) -e PERF_GID=$(shell id -g)
RUN_CMD ?= run

.PHONY: build run run-all shell lint clean help

help: ## show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## build the harvester-perf image
	$(DOCKER) build -t $(IMAGE):$(TAG) .

run: build ## run the default (read-only) suites
	@mkdir -p $(RESULTS)
	$(DOCKER) run $(RUN_FLAGS) $(IMAGE):$(TAG) $(RUN_CMD)

lint: ## shellcheck the scripts
	shellcheck -x entrypoint.sh bin/*.sh suites/*.sh lib/*.sh

clean: ## remove local results
	rm -rf $(RESULTS)

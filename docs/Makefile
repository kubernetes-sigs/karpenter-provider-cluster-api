.PHONY: help serve

CONTAINER_ENGINE ?= podman
MKDOCS_IMAGE ?= docker.io/squidfunk/mkdocs-material:latest
pwd := $(shell pwd)

all: help

build: ## Build the site files, output in ./site
	${CONTAINER_ENGINE} run --rm -it -p 8000:8000 -v ${pwd}:/docs:Z ${MKDOCS_IMAGE} build

clean: ## Clean up locally generated files
	rm -r site

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

serve: ## Serve the site locally
	${CONTAINER_ENGINE} run --rm -it -p 8000:8000 -v ${pwd}:/docs:Z ${MKDOCS_IMAGE}

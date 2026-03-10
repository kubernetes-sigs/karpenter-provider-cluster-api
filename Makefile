# Copyright 2024 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

KWOK_REPO ?= kind.local

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0
ENVTEST = go run ${PROJECT_DIR}/vendor/sigs.k8s.io/controller-runtime/tools/setup-envtest

GINKGO = go run ${PROJECT_DIR}/vendor/github.com/onsi/ginkgo/v2/ginkgo
GINKGO_ARGS = -v --randomize-all --randomize-suites --keep-going --race --trace --timeout=30m

CONTROLLER_GEN = go run ${PROJECT_DIR}/vendor/sigs.k8s.io/controller-tools/cmd/controller-gen

ARCH ?= $(shell go env GOARCH)

CONTAINER_RUNTIME ?= docker
USE_DOCKER ?=
ifeq ($(CONTAINER_RUNTIME), docker)
	USE_DOCKER = -docker
endif

BUILDER_IMAGE ?=

BUILDX_CMD ?= $(CONTAINER_RUNTIME) buildx
IMG_BUILD_CMD ?= $(BUILDX_CMD) build

IMG_REGISTRY ?= gcr.io/k8s-staging-karpenter-cluster-api
IMG_NAME ?= karpenter-clusterapi-controller
IMG_REPO ?= $(IMG_REGISTRY)/$(IMG_NAME)
IMG_TAG ?= $(shell git describe --tags --dirty --always)
IMG ?= $(IMG_REPO):$(IMG_TAG)

ifdef EXTRA_TAG
IMG_EXTRA_TAG ?= $(IMG_REPO):$(EXTRA_TAG)
endif
ifdef IMG_EXTRA_TAG
IMG_BUILD_EXTRA_OPTS += -t $(IMG_EXTRA_TAG)
endif

all: help

.PHONY: build
build: karpenter-clusterapi-controller ## build all binaries

.PHONY: help
help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: gen-objects
gen-objects: ## generate the controller-gen related objects
	$(CONTROLLER_GEN) object paths="./..."

.PHONY: generate
generate: gen-objects manifests ## generate all controller-gen files

karpenter-clusterapi-controller: ## build the main karpenter controller
	go build -o bin/karpenter-clusterapi-controller cmd/controller/main.go

# https://github.com/containers/buildah/issues/4671
# podman/buildah does not support "buildx build --push"
# Remove all this extra logic when it does
.PHONY: image-build
image-build:
	$(IMG_BUILD_CMD) -t $(IMG) \
		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
		--build-arg ARCH=$(ARCH) \
		$(IMG_BUILD_EXTRA_OPTS) .
	$(CONTAINER_RUNTIME) push $(IMG) $(IMG_EXTRA_TAG)

.PHONY: image-build-docker
image-build-docker:
	$(IMG_BUILD_CMD) -t $(IMG) \
		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
		--build-arg ARCH=$(ARCH) \
		$(PUSH) \
		$(IMG_BUILD_EXTRA_OPTS) .

.PHONY: image-push # Push the manager container image to the container IMG_REGISTRY
image-push: PUSH=--push
image-push: image-build$(USE_DOCKER)

.PHONY: manifests
manifests: ## generate the controller-gen kubernetes manifests
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=pkg/apis/crds
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./vendor/sigs.k8s.io/karpenter/pkg/apis/..." output:crd:artifacts:config=pkg/apis/crds

.PHONY: test
test: vendor unit ## vendor the dependencies and run unit tests

.PHONY: unit
unit: ## run the unit tests
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./vendor/sigs.k8s.io/cluster-api/api/v1beta1/..." output:crd:artifacts:config=vendor/sigs.k8s.io/cluster-api/api/v1beta1
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --bin-dir $(PROJECT_DIR)/bin)" ${GINKGO} ${GINKGO_ARGS} ${GINKGO_EXTRA_ARGS} ./...

.PHONY: vendor
vendor: ## update modules and populate local vendor directory
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: docgen
docgen: ## Generate documentation files
	./hack/docgen.sh

.PHONY: release
release: ## Create a release branch, update chart version, and push changes
	./hack/release.sh

.PHONY: build-with-ko
build-with-ko: ## Build the Karpenter CAPI controller image using ko build
	$(eval CONTROLLER_IMG=$(shell KO_DOCKER_REPO="$(KWOK_REPO)" ko build -B sigs.k8s.io/karpenter-provider-cluster-api/cmd/controller))
	$(eval IMG_REPOSITORY=$(shell echo $(CONTROLLER_IMG) | cut -d ":" -f 1))
	$(eval IMG_TAG=latest)

# TODO(maxcao13): replace with helm
# Note, this requires a configmap with the name "mgmt-kubeconfig" with management cluster kubeconfig contents
.PHONY: apply
apply: build-with-ko ## Deploy the Karpenter CAPI controller from the current state of your git repository into your ~/.kube/config cluster
	kubectl apply -f pkg/apis/crds/
	sed 's|REPLACE_IMAGE|$(IMG_REPOSITORY):$(IMG_TAG)|g' test/resources/karpenter-install.yaml | kubectl apply -f -

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

all: help

.PHONY: build
build: karpenter-clusterapi-controller ## build all binaries

.PHONY: help
help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: gen-objects
gen-objects: ## generate the controller-gen related objects
	controller-gen object paths="./..."

.PHONY: generate
generate: gen-objects manifests ## generate all controller-gen files

karpenter-clusterapi-controller: ## build the main karpenter controller
	go build -o bin/karpenter-clusterapi-controller cmd/controller/main.go

.PHONY: manifests
manifests: ## generate the controller-gen kubernetes manifests
	controller-gen rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=pkg/apis/crds
	controller-gen rbac:roleName=manager-role crd paths="./vendor/sigs.k8s.io/karpenter/..." output:crd:artifacts:config=pkg/apis/crds

.PHONY: vendor
vendor: ## update modules and populate local vendor directory
	go mod tidy
	go mod vendor
	go mod verify

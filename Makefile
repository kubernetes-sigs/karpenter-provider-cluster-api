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

help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

manifests: vendor ## generate the kubernetes manifests
	controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=pkg/apis/crds
	controller-gen rbac:roleName=manager-role crd webhook paths="./vendor/sigs.k8s.io/karpenter/..." output:crd:artifacts:config=pkg/apis/crds

vendor: ## update modules and populate local vendor directory
	go mod tidy
	go mod vendor
	go mod verify

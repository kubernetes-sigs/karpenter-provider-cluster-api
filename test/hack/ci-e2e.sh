#!/bin/bash

set -euo pipefail

export HELM_VERSION=4.3.1
export KIND_VERSION=0.33.0
export KO_VERSION=0.19.1
export CAPK_VERSION=v0.10.1
export CAPI_VERSION=release-1.10
export CALICO_VERSION=v3.31.5
export KIND_CLUSTER_IMAGE=kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

bash "$SCRIPT_DIR/steps/1-install-deps.sh"
bash "$SCRIPT_DIR/steps/2-setup-cluster.sh"
bash "$SCRIPT_DIR/steps/3-run-tests.sh"
bash "$SCRIPT_DIR/steps/4-teardown.sh"

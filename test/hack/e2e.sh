#!/bin/bash

set -euo pipefail

CAPK_VERSION=v0.10.1
CAPI_VERSION=release-1.10
CALICO_VERSION=v3.31.5
DEFAULT_KIND_IMAGE=kindest/node:v1.32.8

CLONE_WORKSPACE="${CLONE_WORKSPACE:-$(pwd)}"
mkdir -p "$CLONE_WORKSPACE"

if [[ ! -d $CLONE_WORKSPACE/cluster-api-provider-kubemark ]]; then
  git clone https://github.com/kubernetes-sigs/cluster-api-provider-kubemark.git -b "$CAPK_VERSION" --single-branch
fi

CAPK_HACK="$CLONE_WORKSPACE/cluster-api-provider-kubemark/hack"

if [[ ! -d $CLONE_WORKSPACE/cluster-api ]]; then
  git clone https://github.com/kubernetes-sigs/cluster-api.git -b "$CAPI_VERSION" --single-branch
fi

export KIND_CLUSTER_IMAGE="${KIND_CLUSTER_IMAGE:-$DEFAULT_KIND_IMAGE}"
export CAPI_PATH="$CLONE_WORKSPACE/cluster-api"

# don't run the entire suite with make -C cluster-api-provider-kubemark/hack/tests test-e2e
# we want to create our own machine deployments and scale them up with karpenter
 
# kubemark script doesn't delete the tenant cluster, if it exists
kind delete cluster --name km-cp

make -C "$CAPK_HACK/tests" .start-kind-cluster
make -C "$CAPK_HACK/tests" .install-cert-manager
make -C "$CAPK_HACK/tests" .capi-build-clusterctl
make -C "$CAPK_HACK/tests" .generate-clusterctl-config
make -C "$CAPK_HACK/tests" .generate-manifests
make -C "$CAPK_HACK/tests" .docker-build
make -C "$CAPK_HACK/tests" .create-local-repository
make -C "$CAPK_HACK/tests" .config-local-repository
make -C "$CAPK_HACK/tests" .deploy-cluster-api
make -C "$CAPK_HACK/tests" .create-tenant-cluster-control-plane
# make -C "$CAPK_HACK/tests" .create-tenant-cluster-hollow-nodes # purposefully ignoring
make -C "$CAPK_HACK/tests" .generate-tenant-cluster-kubeconfig
make -C "$CAPK_HACK/tests" .tenant-cluster-info

TENANT_KUBECTL="kubectl --kubeconfig /tmp/km.kubeconfig"

# apply capi workload resources
kubectl apply -f ./test/resources/kubemark-machine-deployment.yaml

# apply calico for CNI
$TENANT_KUBECTL apply -f https://raw.githubusercontent.com/projectcalico/calico/$CALICO_VERSION/manifests/calico.yaml

# we have to manually edit the management kubeconfig for karpenter to use from within the tenant cluster
# use Kind control-plane container's IP as server
# drop CA and skip TLS verify (Kind cert SAN doesn't match IP)
kind get kubeconfig > /tmp/mgmt.kubeconfig
KIND_CP_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' kind-control-plane)
sed -i "s|server: .*|server: https://${KIND_CP_IP}:6443|g" /tmp/mgmt.kubeconfig
sed -i '/certificate-authority-data:/d' /tmp/mgmt.kubeconfig
sed -i '/server: https:\/\//a\    insecure-skip-tls-verify: true' /tmp/mgmt.kubeconfig
$TENANT_KUBECTL create configmap mgmt-kubeconfig \
  --from-file=kubeconfig=/tmp/mgmt.kubeconfig -n kube-system || true

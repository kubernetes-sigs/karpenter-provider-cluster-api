#!/bin/bash

set -euo pipefail

CAPK_VERSION=v0.10.0
CAPI_VERSION=release-1.10
CALICO_VERSION=v3.24.1
DEFAULT_KIND_IMAGE=kindest/node:v1.32.8

WORKSPACE="${WORKSPACE:-$(pwd)}"
mkdir -p "$WORKSPACE"
cd "$WORKSPACE"

if [[ ! -d cluster-api-provider-kubemark ]]; then
  git clone https://github.com/kubernetes-sigs/cluster-api-provider-kubemark.git -b "$CAPK_VERSION" --single-branch
fi

CAPK_HACK="$WORKSPACE/cluster-api-provider-kubemark/hack"

# TODO(maxcao13): kubemark v0.10.0 is using golang.org/x/tools@v0.24.0 which is incompatible with go1.25
# https://github.com/golang/go/issues/7446
go mod -C "$CAPK_HACK/tools" edit -replace=golang.org/x/tools=golang.org/x/tools@v0.24.1
go mod -C "$CAPK_HACK/tools" tidy && go mod -C "$CAPK_HACK/tools" download

if [[ ! -d cluster-api ]]; then
  git clone https://github.com/kubernetes-sigs/cluster-api.git -b "$CAPI_VERSION" --single-branch
fi

export KIND_CLUSTER_IMAGE="${KIND_CLUSTER_IMAGE:-$DEFAULT_KIND_IMAGE}"
export CAPI_PATH="$WORKSPACE/cluster-api"

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
kubectl apply -f "$WORKSPACE"/test/resources/kubemark-machine-deployment.yaml

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
  --from-file=kubeconfig=/tmp/mgmt.kubeconfig -n kube-system

# apply karpenter deployment manifests and CRDs
# deployment should have the mounted configmap
KIND_CLUSTER_NAME=km-cp KWOK_REPO=kind.local KUBECONFIG=/tmp/km.kubeconfig make apply

$TENANT_KUBECTL wait -n kube-system deployment karpenter --for condition=Available --timeout=2m

# TODO(maxcao13): below is a temporary validation test that karpenter works, we should remove this when we have real tests

# apply karpenter workload manifests
$TENANT_KUBECTL apply -f "$WORKSPACE"/test/resources/default_clusterapinodeclass.yaml
$TENANT_KUBECTL apply -f "$WORKSPACE"/test/resources/default_nodepool.yaml
$TENANT_KUBECTL apply -f "$WORKSPACE"/test/resources/sample_deployment.yaml

# scale up the deployment
$TENANT_KUBECTL scale deployment scale-up --replicas=3

printf "\nWaiting for 20 seconds for karpenter to scale up...\n"
sleep 20

$TENANT_KUBECTL get pods,nodeclaims,nodes -o wide

# scale down the deployment
$TENANT_KUBECTL scale deployment scale-up --replicas=0

printf "\nWaiting for 60 seconds for karpenter to scale down...\n"
sleep 60

$TENANT_KUBECTL get pods,nodeclaims,nodes -o wide

printf "\nTest completed successfully!"

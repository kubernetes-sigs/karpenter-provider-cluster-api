#!/bin/bash

set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)

export KUBECONFIG=/tmp/km.kubeconfig
export KWOK_REPO=kind.local
export KIND_CLUSTER_NAME=km-cp

# apply karpenter deployment manifests and CRDs
# deployment should have the mounted configmap
make -C "$REPO_ROOT" apply

kubectl wait -n kube-system deployment karpenter --for condition=Available --timeout=2m

# TODO(maxcao13): below is a temporary validation test that karpenter works, we should remove this when we have real tests

# apply karpenter workload manifests
kubectl apply -f "$REPO_ROOT"/test/resources/default_clusterapinodeclass.yaml
kubectl apply -f "$REPO_ROOT"/test/resources/default_nodepool.yaml
kubectl apply -f "$REPO_ROOT"/test/resources/sample_deployment.yaml

# scale up the deployment
kubectl scale deployment scale-up --replicas=3

printf "\nWaiting for 20 seconds for karpenter to scale up...\n"
sleep 20

kubectl get pods,nodeclaims,nodes -o wide

# scale down the deployment
kubectl scale deployment scale-up --replicas=0

printf "\nWaiting for 60 seconds for karpenter to scale down...\n"
sleep 60

kubectl get pods,nodeclaims,nodes -o wide

printf "\nTest completed successfully!"

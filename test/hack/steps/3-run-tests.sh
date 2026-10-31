#!/bin/bash

set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)

export KUBECONFIG=/tmp/km.kubeconfig
export KWOK_REPO=kind.local
export KIND_CLUSTER_NAME=km-cp

# Install Karpenter into the tenant cluster using the management kubeconfig ConfigMap.
make -C "$REPO_ROOT" apply
kubectl wait -n kube-system deployment karpenter --for condition=Available --timeout=2m

kubectl get pods -n kube-system -l app.kubernetes.io/name=karpenter
kubectl get nodepools
kubectl get pods -A
kubectl describe nodes

kubectl taint nodes --selector='node-role.kubernetes.io/control-plane' CriticalAddonsOnly:NoSchedule --overwrite
make -C "$REPO_ROOT" upstream-e2etests

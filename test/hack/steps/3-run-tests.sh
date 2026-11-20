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
# TEMP(oom-debug): sample memory every 60s to see what fills the job's 16Gi limit
(
  set +eu +o pipefail  # sampling is best effort and must never fail the job
  while true; do
    # the job's own cgroup is the namespace root here, so it has no memory.current; use free and docker stats instead
    echo "[mem] $(date -u +%H:%M:%S) nodes=$(kubectl get nodes --no-headers 2>/dev/null | wc -l) $(free -m | awk '/^Mem:/{print "free_used="$3"Mi free_total="$2"Mi"}')"
    docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' 2>/dev/null | awk '
      { split($2, a, /[0-9.]+/); v = $2 + 0; u = substr($2, length(v "") + 1)
        if (u ~ /^GiB/) v *= 1024; else if (u ~ /^KiB/) v /= 1024; else if (u ~ /^B/) v /= 1048576
        printf "[mem] docker %s %.0fMi\n", $1, v; t += v }
      END { printf "[mem] docker total %.0fMi\n", t }'
    docker exec kind-control-plane crictl stats -o json 2>/dev/null | python3 -c 'import json,sys; s=json.load(sys.stdin)["stats"]; s.sort(key=lambda c:-int(c["memory"]["workingSetBytes"]["value"])); [print("[mem] kind", c["attributes"]["labels"].get("io.kubernetes.pod.name","?"), c["attributes"]["metadata"]["name"], int(c["memory"]["workingSetBytes"]["value"])//1048576, "Mi") for c in s[:10]]; print("[mem] kind total", sum(int(c["memory"]["workingSetBytes"]["value"]) for c in s)//1048576, "Mi in", len(s), "containers")' 2>/dev/null
    sleep 60
  done
) &
MEM_SAMPLER_PID=$!
trap 'kill $MEM_SAMPLER_PID 2>/dev/null || true' EXIT

make -C "$REPO_ROOT" upstream-e2etests

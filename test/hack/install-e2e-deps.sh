#!/bin/bash

set -euo pipefail

HELM_VERSION=4.3.1
KIND_VERSION=0.33.0
KO_VERSION=0.19.1

MACHINE_ARCH=$(uname -m)
case "$MACHINE_ARCH" in
  x86_64) GOARCH=amd64; KO_ARCH=x86_64 ;;
  aarch64|arm64) GOARCH=arm64; KO_ARCH=arm64 ;;
  *) echo "Unsupported architecture: $MACHINE_ARCH" >&2; exit 1 ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
TITLE_CASE_OS=$(uname -s)

if [[ ! -w /usr/local/bin ]]; then
  echo "/usr/local/bin is not writable. Run with appropriate permissions." >&2
  exit 1
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# install helm
curl -sSfL "https://get.helm.sh/helm-v${HELM_VERSION}-${OS}-${GOARCH}.tar.gz" > "$TMP_DIR/helm.tar.gz"
tar -zxvf "$TMP_DIR/helm.tar.gz" -C /usr/local/bin --strip-components=1 "${OS}-${GOARCH}/helm"
chmod +x /usr/local/bin/helm

# install kind
curl -sSfLo "$TMP_DIR/kind" "https://kind.sigs.k8s.io/dl/v${KIND_VERSION}/kind-${OS}-${GOARCH}"
install -m 0755 "$TMP_DIR/kind" /usr/local/bin/kind

# install ko
curl -sSfL "https://github.com/ko-build/ko/releases/download/v${KO_VERSION}/ko_${KO_VERSION}_${TITLE_CASE_OS}_${KO_ARCH}.tar.gz" > "$TMP_DIR/ko.tar.gz"
tar -zxvf "$TMP_DIR/ko.tar.gz" -C /usr/local/bin ko
chmod +x /usr/local/bin/ko

#!/bin/bash
set -eu -o pipefail

git_tag="$(git describe --exact-match --tags || echo "none")"
if [[ "${git_tag}" != v* ]]; then
  echo "::notice::This commit is no tagged."
  exit 1
fi

git config user.name "Release"
git config user.email "release@users.noreply.github.com"
git remote set-url origin "https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPO}"

branch_name="release-${git_tag}"
git checkout -b "${branch_name}"

# Bump the version and appVersion of the Helm chart in charts/karpenter/Chart.yaml
sed -i "s/^appVersion: .*/appVersion: \"${git_tag#v}\"/" charts/karpenter/Chart.yaml
sed -i "s/^version: .*/version: ${git_tag#v}/" charts/karpenter/Chart.yaml

git add go.mod
git add go.sum
git add docs/docs
git add charts/karpenter
git commit -m "chore(release): release ${git_tag} (automated)"
git push --set-upstream origin "${branch_name}"

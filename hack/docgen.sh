#!/bin/bash
set -eu -o pipefail

go run hack/docs/settings_gen/main.go docs/docs/settings.md

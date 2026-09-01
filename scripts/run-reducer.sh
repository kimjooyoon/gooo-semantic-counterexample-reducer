#!/usr/bin/env bash
set -Eeuo pipefail

repo_root=${1:?repository root is required}
runner=${2:?runner path is required}
output=${3:?caller-owned output is required}

test "$(git -C "$repo_root" status --porcelain)" = ""
"$runner" reduce --contract "$repo_root/meta/counterexample-reducer.gooo" --repo-root "$repo_root" --out "$output"
test "$(git -C "$repo_root" status --porcelain)" = ""

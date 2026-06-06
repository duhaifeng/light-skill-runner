#!/usr/bin/env sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
cd "$repo_root"

binary="$repo_root/skill-desktop"

if command -v pkill >/dev/null 2>&1; then
  pkill -f "$binary" >/dev/null 2>&1 || true
fi

go build -tags "desktop,production" -o "$binary" ./cmd/skill-desktop

"$binary" >/dev/null 2>&1 &

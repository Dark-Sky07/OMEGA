#!/bin/sh
# Resolve the newest stable Xray-core release without baking a version into
# the panel bundle. GitHub's /latest endpoint intentionally excludes drafts
# and prereleases; validate the result before it is used in a download URL.
set -eu

api="https://api.github.com/repos/XTLS/Xray-core/releases/latest"
json="$(curl -fsSL --retry 3 --connect-timeout 10 "$api")" || {
  echo "unable to query Xray-core releases" >&2
  exit 1
}

tag="$(printf '%s' "$json" | sed -nE 's/.*"tag_name"[[:space:]]*:[[:space:]]*"(v[0-9]+\.[0-9]+\.[0-9]+)".*/\1/p' | head -n 1)"
case "$tag" in
  v[0-9]*.[0-9]*.[0-9]*) printf '%s\n' "$tag" ;;
  *) echo "Xray-core latest response did not contain a stable semver tag" >&2; exit 1 ;;
esac

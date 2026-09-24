#!/bin/sh
# Resolve the highest published Xray-core release that follows the numeric
# vMAJOR.MINOR.PATCH tag format. OMEGA intentionally resolves this at build or
# install time instead of silently retaining the core bundled in an old panel.
set -eu

api_url="https://api.github.com/repos/XTLS/Xray-core/releases?per_page=100"
releases="$(curl -4fsSL --retry 3 --connect-timeout 10 "$api_url")" || {
  echo "failed to query Xray-core releases" >&2
  exit 1
}

version="$(printf '%s\n' "$releases" \
  | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+"' \
  | sed -E 's/.*"(v[0-9]+\.[0-9]+\.[0-9]+)"/\1/' \
  | sort -V \
  | tail -n 1)"

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) printf '%s\n' "$version" ;;
  *) echo "no valid Xray-core release was returned by GitHub" >&2; exit 1 ;;
esac

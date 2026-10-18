#!/bin/sh
# Resolve the highest published Xray-core release that follows the numeric
# vMAJOR.MINOR.PATCH tag format. OMEGA intentionally resolves this at build or
# install time instead of silently retaining the core bundled in an old panel.
set -eu

api_url="https://api.github.com/repos/XTLS/Xray-core/releases?per_page=20"
auth_arg=""
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth_arg="-H Authorization: Bearer ${GITHUB_TOKEN}"
fi

releases=""
if [ -n "$auth_arg" ]; then
  releases="$(curl -4fsSL --retry 3 --connect-timeout 10 -H "Authorization: Bearer ${GITHUB_TOKEN}" "$api_url" 2>/dev/null)" || releases=""
else
  releases="$(curl -4fsSL --retry 3 --connect-timeout 10 "$api_url" 2>/dev/null)" || releases=""
fi

version="$(printf '%s\n' "$releases" \
  | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+"' \
  | sed -E 's/.*"(v[0-9]+\.[0-9]+\.[0-9]+)"/\1/' \
  | sort -V \
  | tail -n 1 || true)"

# Fallback when unauthenticated GitHub API rate-limits shared CI runner IPs:
# resolve the /releases/latest HTTP redirect (never API rate-limited).
if [ -z "$version" ]; then
  eff_url="$(curl -4fsSL -o /dev/null -w '%{url_effective}' --retry 3 --connect-timeout 10 "https://github.com/XTLS/Xray-core/releases/latest" 2>/dev/null || true)"
  version="$(printf '%s\n' "$eff_url" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+$' || true)"
fi

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) printf '%s\n' "$version" ;;
  *) echo "no valid Xray-core release was returned by GitHub" >&2; exit 1 ;;
esac

#!/bin/sh
# OMEGA release/runtime bundles are deliberately pinned to the tested
# Xray-core release. Do not silently fall back to the version shipped by an
# older panel archive or resolve an unrelated future /latest build here.
set -eu

XRAY_VERSION="v26.9.9"
case "$XRAY_VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) printf '%s\n' "$XRAY_VERSION" ;;
  *) echo "invalid pinned Xray-core version: $XRAY_VERSION" >&2; exit 1 ;;
esac

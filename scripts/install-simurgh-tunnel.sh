#!/usr/bin/env bash
set -euo pipefail

SIMURGH_VERSION="3.1.1"
SIMURGH_ARCHIVE_SHA256="c8f3abfc55f686c4fda594dc3774e38da59b37c823beb6250972bb2aff77acb7"
SIMURGH_ARCHIVE_URL="https://codeload.github.com/Dark-Sky07/Simurgh-Tunnel/tar.gz/refs/tags/v${SIMURGH_VERSION}"
APP_DIR="/opt/phoenix-simurgh"
SIMURGH_HOME="/etc/phoenix-simurgh"
UNIT_FILE="/etc/systemd/system/phoenix-simurgh-relay.service"

say() { printf '%s\n' "[Simurgh] $*"; }

if [[ "${EUID}" -ne 0 ]]; then
  say "Run as root; skipping the optional service installation."
  exit 0
fi
install_python_if_missing() {
  command -v python3 >/dev/null 2>&1 && return 0
  . /etc/os-release 2>/dev/null || true
  case "${ID:-}" in
    debian|ubuntu|armbian) apt-get update && apt-get install -y python3 ;;
    fedora|rhel|rocky|almalinux|amzn|ol) dnf install -y python3 2>/dev/null || yum install -y python3 ;;
    centos) yum install -y python3 ;;
    arch|manjaro) pacman -Sy --noconfirm python ;;
    opensuse*|sles) zypper --non-interactive install python3 ;;
    alpine) apk add --no-cache python3 ;;
    *) return 1 ;;
  esac
}

if ! command -v python3 >/dev/null 2>&1 && ! install_python_if_missing; then
  say "Python could not be installed; PHOENIX will continue without the separate service."
  exit 0
fi
if ! command -v python3 >/dev/null 2>&1 || ! python3 -c 'import sys; raise SystemExit(sys.version_info < (3, 9))'; then
  say "Python 3.9 or newer is required; the PHOENIX installer will continue without the service."
  exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if ! curl -fL --retry 2 --connect-timeout 10 "$SIMURGH_ARCHIVE_URL" -o "$work/simurgh.tar.gz"; then
  say "Could not download the pinned Simurgh source archive; PHOENIX remains unchanged."
  exit 0
fi
if ! printf '%s  %s\n' "$SIMURGH_ARCHIVE_SHA256" "$work/simurgh.tar.gz" | sha256sum -c >/dev/null 2>&1; then
  say "The Simurgh archive checksum did not match; refusing to install it."
  exit 0
fi

mkdir -p "$APP_DIR/lib" "$SIMURGH_HOME"
chmod 700 "$SIMURGH_HOME"

# A standalone Simurgh update may be newer than the version bundled by this
# PHOENIX release. Never replace a newer (or equal) Python engine with the
# older bundled source during a PHOENIX reinstall/update.
installed_version=""
if [[ -f "$APP_DIR/lib/simurgh/__init__.py" ]]; then
  installed_version="$(python3 - "$APP_DIR/lib/simurgh/__init__.py" <<'PY'
import re
import sys
from pathlib import Path
match = re.search(r"^__version__\s*=\s*['\"]([^'\"]+)['\"]", Path(sys.argv[1]).read_text(encoding="utf-8"), re.M)
print(match.group(1) if match else "")
PY
)"
fi
keep_installed=false
if [[ "$installed_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] && python3 - "$installed_version" "$SIMURGH_VERSION" <<'PY'
import sys
version = lambda value: tuple(int(part) for part in value.split("."))
raise SystemExit(0 if version(sys.argv[1]) >= version(sys.argv[2]) else 1)
PY
then
  keep_installed=true
  say "Keeping installed Simurgh v${installed_version}; bundled version is ${SIMURGH_VERSION}."
fi

if [[ "$keep_installed" != true ]]; then
  tar -xzf "$work/simurgh.tar.gz" --strip-components=1 -C "$APP_DIR/lib" \
    "Simurgh-Tunnel-${SIMURGH_VERSION}/simurgh" \
    "Simurgh-Tunnel-${SIMURGH_VERSION}/LICENSE"
fi
install -m 644 "$(dirname "$0")/phoenix-simurgh-launcher.py" "$APP_DIR/launcher.py"
cat > "$APP_DIR/simurgh" <<'SH'
#!/bin/sh
APP_DIR="/opt/phoenix-simurgh"
export PYTHONPATH="$APP_DIR/lib${PYTHONPATH:+:$PYTHONPATH}"
exec python3 "$APP_DIR/launcher.py" "$@"
SH
chmod 755 "$APP_DIR/simurgh"
chmod 644 "$APP_DIR/launcher.py"

if [[ ! -s "$SIMURGH_HOME/relay.toml" ]]; then
  if ! SIMURGH_HOME="$SIMURGH_HOME" "$APP_DIR/simurgh" install \
    --role relay --pair-later --panel-port 18787 --engine python \
    --no-units --no-enable --no-start --force >"$work/configure.log" 2>&1; then
    say "Could not create an idle relay configuration; the rest of PHOENIX is untouched."
    exit 0
  fi
fi

if ! python3 - <<'PY' >/dev/null 2>&1
import socket
s = socket.socket()
s.bind(("127.0.0.1", 18787))
s.close()
PY
then
  own_service_active=false
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet phoenix-simurgh-relay.service; then
    own_service_active=true
  elif command -v rc-service >/dev/null 2>&1 && rc-service phoenix-simurgh status >/dev/null 2>&1; then
    own_service_active=true
  fi
  if [[ "$own_service_active" != true ]]; then
    say "Local port 18787 is already in use; leaving the service stopped to avoid interfering with another process."
    exit 0
  fi
fi

if command -v systemctl >/dev/null 2>&1 && systemctl show --property=Version >/dev/null 2>&1; then
  if [[ -e "$UNIT_FILE" ]] && ! grep -q 'PHOENIX bundled Simurgh panel' "$UNIT_FILE"; then
    say "An unrelated phoenix-simurgh-relay.service already exists; leaving it untouched."
    exit 0
  fi
  cat > "$UNIT_FILE" <<'UNIT'
[Unit]
Description=PHOENIX bundled Simurgh panel
After=network-online.target x-ui.service
Wants=network-online.target

[Service]
Type=simple
Environment=SIMURGH_HOME=/etc/phoenix-simurgh
Environment=PYTHONUNBUFFERED=1
ExecStart=/opt/phoenix-simurgh/simurgh relay --panel-host 127.0.0.1 --panel-port 18787
Restart=on-failure
RestartSec=4
LimitNOFILE=1048576
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
UNIT
  chmod 644 "$UNIT_FILE"
  systemctl daemon-reload
  if systemctl is-active --quiet phoenix-simurgh-relay.service; then
    service_action=restart
    systemctl enable phoenix-simurgh-relay.service >/dev/null 2>&1 || true
  else
    service_action=start
    systemctl enable phoenix-simurgh-relay.service >/dev/null 2>&1 || true
  fi
  if ! systemctl "$service_action" phoenix-simurgh-relay.service >/dev/null 2>&1; then
    say "The separate panel service did not start; PHOENIX and its Xray service were not changed."
  fi
elif command -v rc-update >/dev/null 2>&1 && [[ -d /etc/init.d ]]; then
  if [[ -e /etc/init.d/phoenix-simurgh ]] && ! grep -q 'PHOENIX bundled Simurgh panel' /etc/init.d/phoenix-simurgh; then
    say "An unrelated phoenix-simurgh service already exists; leaving it untouched."
    exit 0
  fi
  if [[ ! -e /etc/init.d/phoenix-simurgh ]]; then
    cat > /etc/init.d/phoenix-simurgh <<'OPENRC'
#!/sbin/openrc-run
description="PHOENIX bundled Simurgh panel"
command="/opt/phoenix-simurgh/simurgh"
command_args="relay --panel-host 127.0.0.1 --panel-port 18787"
command_background=true
pidfile="/run/phoenix-simurgh.pid"
export SIMURGH_HOME="/etc/phoenix-simurgh"
export PYTHONUNBUFFERED="1"
OPENRC
    chmod 755 /etc/init.d/phoenix-simurgh
    rc-update add phoenix-simurgh default >/dev/null 2>&1 || true
  fi
  if rc-service phoenix-simurgh status >/dev/null 2>&1; then
    rc-service phoenix-simurgh restart >/dev/null 2>&1 || say "The separate service did not restart; PHOENIX was not changed."
  else
    rc-service phoenix-simurgh start >/dev/null 2>&1 || say "The separate service did not start; PHOENIX was not changed."
  fi
else
  say "No supported service manager found; Simurgh files are installed but its service was not started."
fi

say "The UI is available in PHOENIX under Simurgh; its management listener is bound to loopback only."

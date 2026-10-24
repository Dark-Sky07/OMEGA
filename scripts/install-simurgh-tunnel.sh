#!/usr/bin/env bash
set -euo pipefail

APP_DIR="/opt/phoenix-simurgh"
SIMURGH_HOME="/etc/phoenix-simurgh"
UNIT_FILE="/etc/systemd/system/phoenix-simurgh-relay.service"
PHOENIX_REPO="${PHOENIX_REPO:-Dark-Sky07/PHOENIX}"
PHOENIX_REF="${PHOENIX_REF:-${PHOENIX_TAG:-}}"

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
if [[ -z "$PHOENIX_REF" ]]; then
  PHOENIX_REF="$(curl -4fsSL "https://api.github.com/repos/${PHOENIX_REPO}/releases/latest" 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name", ""))' 2>/dev/null || true)"
fi
if [[ -z "$PHOENIX_REF" ]]; then
  say "Could not resolve the PHOENIX release containing the verified Simurgh installer."
  exit 1
fi

mkdir -p "$APP_DIR/lib" "$SIMURGH_HOME"
chmod 700 "$SIMURGH_HOME"

# A fresh install uses the latest stable, SHA-256-verified upstream package.
# Existing installations are left in place here and updated transactionally by
# the PHOENIX updater after this service is healthy.
if [[ ! -f "$APP_DIR/lib/simurgh/__init__.py" ]]; then
  for helper in update_simurgh.py build_latest_simurgh.py; do
    if ! curl -fL --retry 2 --connect-timeout 10 \
      "https://raw.githubusercontent.com/${PHOENIX_REPO}/${PHOENIX_REF}/scripts/${helper}" \
      -o "$work/${helper}"; then
      say "Could not download ${helper} from PHOENIX ${PHOENIX_REF}; Simurgh was not installed."
      exit 1
    fi
  done
  if ! python3 -m py_compile "$work/update_simurgh.py" "$work/build_latest_simurgh.py"; then
    say "The downloaded Simurgh installer helpers failed validation."
    exit 1
  fi
  if ! python3 "$work/build_latest_simurgh.py" "$APP_DIR/lib"; then
    say "Could not install the latest verified Simurgh package; PHOENIX remains available."
    exit 1
  fi
fi

if [[ ! -f "$APP_DIR/lib/simurgh/__init__.py" ]] || \
   ! PYTHONPATH="$APP_DIR/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m simurgh --version >/dev/null 2>&1; then
  say "The Simurgh package is missing or failed its version smoke test."
  exit 1
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

verify_panel() {
  SIMURGH_HOME="$SIMURGH_HOME" PYTHONPATH="$APP_DIR/lib${PYTHONPATH:+:$PYTHONPATH}" \
    python3 - <<'PY'
import base64
import json
import os
import time
import urllib.error
import urllib.request
from pathlib import Path

base = "http://127.0.0.1:18787"
state_path = Path(os.environ["SIMURGH_HOME"]) / "state.json"
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
deadline = time.monotonic() + 30
last_error = "no response"
while time.monotonic() < deadline:
    try:
        with opener.open(base + "/healthz", timeout=2) as response:
            health = json.load(response)
        if (health.get("ok") is not True or health.get("product") != "Simurgh Tunnel"
                or health.get("role") != "relay"):
            raise RuntimeError("health endpoint returned unexpected service metadata")

        state = json.loads(state_path.read_text(encoding="utf-8"))
        auth = state.get("panel_auth") or {}
        user, password = auth.get("user"), auth.get("password")
        if not user or not password:
            raise RuntimeError("panel credentials were not initialized")
        token = base64.b64encode(f"{user}:{password}".encode()).decode()
        request = urllib.request.Request(base + "/", headers={"Authorization": "Basic " + token})
        with opener.open(request, timeout=3) as response:
            html = response.read(256 * 1024).decode("utf-8", "replace")
        if "<title>Simurgh Tunnel</title>" not in html or 'src="/app.js"' not in html:
            raise RuntimeError("panel HTML is missing its expected title or JavaScript bundle")
        for asset in ("/app.js", "/app.css"):
            with opener.open(base + asset, timeout=3) as response:
                if response.status != 200 or not response.read(64):
                    raise RuntimeError(f"panel asset failed to load: {asset}")
        print(f"Simurgh panel verified: v{health.get('version')} (health, login, and assets OK).")
        raise SystemExit(0)
    except (OSError, ValueError, KeyError, RuntimeError, urllib.error.URLError) as exc:
        last_error = str(exc)
        time.sleep(0.5)
print(f"Simurgh panel health/UI check failed: {last_error}")
raise SystemExit(1)
PY
}

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
    exit 1
  fi
fi

if command -v systemctl >/dev/null 2>&1 && systemctl show --property=Version >/dev/null 2>&1; then
  if [[ -e "$UNIT_FILE" ]] && ! grep -q 'PHOENIX bundled Simurgh panel' "$UNIT_FILE"; then
    say "An unrelated phoenix-simurgh-relay.service already exists; leaving it untouched."
    exit 1
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
    say "The Simurgh service failed to start; inspect: journalctl -u phoenix-simurgh-relay.service"
    exit 1
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
    if ! rc-service phoenix-simurgh restart >/dev/null 2>&1; then
      say "The Simurgh service failed to restart; inspect the OpenRC service log."
      exit 1
    fi
  elif ! rc-service phoenix-simurgh start >/dev/null 2>&1; then
    say "The Simurgh service failed to start; inspect the OpenRC service log."
    exit 1
  fi
else
  say "No supported service manager found; Simurgh was not started."
  exit 1
fi

if ! verify_panel; then
  say "Simurgh did not pass its health and UI checks; PHOENIX and Xray were left running."
  say "Inspect: journalctl -u phoenix-simurgh-relay.service"
  exit 1
fi

say "Simurgh Tunnel is installed, running, and verified through PHOENIX; its listener is loopback-only."

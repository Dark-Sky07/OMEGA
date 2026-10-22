import shutil
import subprocess

from simurgh import cli, systemd

# Keep UI service actions scoped to this bundled instance, never to a separate
# Simurgh installation that may already exist on the host.
cli._service_names = lambda role: ("phoenix-simurgh-relay",)

# OpenRC distributions do not have systemd's service manager. Route the same
# panel buttons to this installation's dedicated OpenRC unit instead.
if not systemd.available() and shutil.which("rc-service"):
    systemd.available = lambda: True

    def _openrc_control(action, services):
        results = {}
        for service in services:
            if service != "phoenix-simurgh-relay":
                results[service] = "refused: not the bundled service"
                continue
            command = ["rc-service", "phoenix-simurgh", action]
            if action == "enable":
                command = ["rc-update", "add", "phoenix-simurgh", "default"]
            elif action == "disable":
                command = ["rc-update", "del", "phoenix-simurgh", "default"]
            result = subprocess.run(command, capture_output=True, text=True, timeout=30)
            results[service] = "ok" if result.returncode == 0 else (result.stderr or result.stdout).strip()
        return results

    systemd.control = _openrc_control

raise SystemExit(cli.main())

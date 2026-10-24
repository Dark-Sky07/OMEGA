import os
import shutil
import subprocess

from simurgh import cli, menu, systemd


# Keep CLI service actions scoped to this bundled instance, never to a separate
# Simurgh installation that may already exist on the host.
cli._service_names = lambda role: ("phoenix-simurgh-relay",)


def running_in_docker():
    return os.environ.get("XUI_IN_DOCKER", "").lower() == "true" or os.path.exists("/.dockerenv")


_OPENRC = not systemd.available() and bool(shutil.which("rc-service") and shutil.which("rc-update"))

# OpenRC distributions do not have systemd's service manager. Route service
# actions to this installation's dedicated OpenRC unit instead.
if _OPENRC:
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


# The upstream text menu normally addresses `simurgh-relay.service`. PHOENIX
# owns a deliberately separate unit; map both display and actions to it so the
# menu cannot restart or stop an unrelated host installation.
def _bundled_service_state(service):
    if service == "simurgh-relay":
        if running_in_docker():
            return "managed by Docker"
        if _OPENRC:
            result = subprocess.run(
                ["rc-service", "phoenix-simurgh", "status"],
                capture_output=True, text=True, timeout=15,
            )
            return "active" if result.returncode == 0 else "stopped"
        if not systemd.available():
            return "no systemd/OpenRC"
        state = systemd.service_state("phoenix-simurgh-relay")
        active = state.get("ActiveState", "?")
        sub = state.get("SubState", "")
        label = {"active": menu.c("active", "green"),
                 "inactive": menu.c("stopped", "red"),
                 "failed": menu.c("failed", "red"),
                 "activating": menu.c("starting", "yellow")}.get(active, active)
        return label + (f"/{sub}" if sub else "")
    if service == "simurgh-exit":
        return "not bundled by PHOENIX"
    return "not managed by PHOENIX"


def _bundled_service_action(role, action):
    if role != "relay":
        print(menu.c("\n  PHOENIX only bundles the relay service.", "red"))
        menu.pause()
        return
    if running_in_docker():
        print(menu.c("\n  Simurgh is managed by Docker; pull and recreate the PHOENIX image.", "yellow"))
        menu.pause()
        return
    if not systemd.available():
        print(menu.c("\n  No supported service manager is available.", "red"))
        menu.pause()
        return
    results = systemd.control(action, ("phoenix-simurgh-relay",))
    for name, result in results.items():
        mark = menu.c("✔", "green") if result == "ok" else menu.c("✘", "red")
        print(f"  {mark} {name}: {action} {'' if result == 'ok' else result}")
    menu.time.sleep(0.8)


menu.service_state = _bundled_service_state
menu.service_action = _bundled_service_action


def main(argv=None):
    return cli.main(argv)


if __name__ == "__main__":
    raise SystemExit(main())

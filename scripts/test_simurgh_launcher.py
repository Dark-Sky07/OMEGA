#!/usr/bin/env python3
"""Verify the bundled Simurgh CLI menu cannot control another service."""
from __future__ import annotations

import importlib.util
import io
import sys
import types
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import Mock, patch

SCRIPT = Path(__file__).with_name("phoenix-simurgh-launcher.py")


def load_launcher():
    package = types.ModuleType("simurgh")
    package.__path__ = []
    cli = types.ModuleType("simurgh.cli")
    cli.main = Mock(return_value=0)
    systemd = types.ModuleType("simurgh.systemd")
    systemd.available = Mock(return_value=True)
    systemd.control = Mock(return_value={"phoenix-simurgh-relay": "ok"})
    systemd.service_state = Mock(return_value={"ActiveState": "active", "SubState": "running"})
    menu = types.ModuleType("simurgh.menu")
    menu.service_state = Mock(return_value="upstream-service")
    menu.service_action = Mock()
    menu.c = lambda text, _colour: text
    menu.pause = Mock()
    menu.time = types.SimpleNamespace(sleep=Mock())
    package.cli, package.menu, package.systemd = cli, menu, systemd
    modules = {"simurgh": package, "simurgh.cli": cli,
               "simurgh.menu": menu, "simurgh.systemd": systemd}
    spec = importlib.util.spec_from_file_location("phoenix_simurgh_launcher_test", SCRIPT)
    assert spec and spec.loader
    launcher = importlib.util.module_from_spec(spec)
    with patch.dict(sys.modules, modules):
        spec.loader.exec_module(launcher)
    return launcher, cli, menu, systemd


class BundledMenuTests(unittest.TestCase):
    def test_menu_routes_only_to_the_phoenix_relay_unit(self):
        launcher, cli, menu, systemd = load_launcher()
        self.assertEqual(cli._service_names("relay"), ("phoenix-simurgh-relay",))

        with patch.object(launcher, "running_in_docker", return_value=False):
            self.assertIn("active", menu.service_state("simurgh-relay"))
            systemd.service_state.assert_called_once_with("phoenix-simurgh-relay")
            menu.service_action("relay", "restart")
            systemd.control.assert_called_once_with("restart", ("phoenix-simurgh-relay",))

            output = io.StringIO()
            with redirect_stdout(output):
                menu.service_action("exit", "stop")
            self.assertIn("only bundles the relay", output.getvalue())
            systemd.control.assert_called_once()

    def test_menu_refuses_to_restart_container_managed_service(self):
        launcher, _cli, menu, systemd = load_launcher()
        output = io.StringIO()
        with patch.object(launcher, "running_in_docker", return_value=True), redirect_stdout(output):
            menu.service_action("relay", "restart")
        self.assertIn("managed by Docker", output.getvalue())
        systemd.control.assert_not_called()
        self.assertEqual(menu.pause.call_count, 1)


if __name__ == "__main__":
    unittest.main()

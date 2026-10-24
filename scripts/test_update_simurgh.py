#!/usr/bin/env python3
"""Offline safety tests for the PHOENIX Simurgh updater."""
from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import sys
import tarfile
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("update_simurgh.py")
SPEC = importlib.util.spec_from_file_location("update_simurgh", SCRIPT)
assert SPEC and SPEC.loader
updater = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(updater)
sys.modules["update_simurgh"] = updater

BUILD_SPEC = importlib.util.spec_from_file_location(
    "build_latest_simurgh", Path(__file__).with_name("build_latest_simurgh.py")
)
assert BUILD_SPEC and BUILD_SPEC.loader
builder = importlib.util.module_from_spec(BUILD_SPEC)
BUILD_SPEC.loader.exec_module(builder)


class FakeServiceManager:
    def __init__(self, active: bool = True, fail_first_start: bool = False):
        self.active = active
        self.fail_first_start = fail_first_start
        self.stops = 0
        self.starts = 0

    def is_active(self) -> bool:
        return self.active

    def stop(self) -> None:
        self.stops += 1
        self.active = False

    def start(self) -> None:
        self.starts += 1
        if self.fail_first_start and self.starts == 1:
            raise updater.UpdateError("simulated start failure")
        self.active = True


def write_package(root: Path, version: str, marker: str = "") -> None:
    package = root / "simurgh"
    package.mkdir(parents=True)
    (package / "__init__.py").write_text(f'__version__ = "{version}"\n', encoding="utf-8")
    (package / "__main__.py").write_text(
        "from . import __version__\nprint(f'Simurgh Tunnel v{__version__}')\n",
        encoding="utf-8",
    )
    (package / "marker.txt").write_text(marker, encoding="utf-8")


def make_archive(path: Path, version: str, marker: str = "new") -> str:
    with tarfile.open(path, "w:gz") as archive:
        members = {
            "usr/lib/simurgh/simurgh/__init__.py": f'__version__ = "{version}"\n'.encode(),
            "usr/lib/simurgh/simurgh/__main__.py": (
                "from . import __version__\nprint(f'Simurgh Tunnel v{__version__}')\n".encode()
            ),
            "usr/lib/simurgh/simurgh/marker.txt": marker.encode(),
            "usr/share/doc/simurgh/LICENSE": b"MIT License\n",
        }
        for name, payload in members.items():
            info = tarfile.TarInfo("./" + name)
            info.size = len(payload)
            info.mode = 0o644
            archive.addfile(info, io.BytesIO(payload))
    return hashlib.sha256(path.read_bytes()).hexdigest()


def release_for(archive: Path, version: str, digest: str) -> dict:
    return {
        "tag": f"v{version}",
        "version": version,
        "name": f"Simurgh v{version}",
        "notes_url": "https://github.com/Dark-Sky07/Simurgh-Tunnel/releases/tag/v" + version,
        "asset_name": f"simurgh-{version}-noarch.tar.gz",
        "asset_url": "https://example.invalid/package.tar.gz",
        "sha256": digest,
    }


class ReleaseMetadataTests(unittest.TestCase):
    def test_release_metadata_requires_digest_and_official_asset_url(self):
        payload = {
            "tag_name": "v3.2.0",
            "draft": False,
            "prerelease": False,
            "assets": [{
                "name": "simurgh-3.2.0-noarch.tar.gz",
                "digest": "sha256:" + "a" * 64,
                "browser_download_url": (
                    "https://github.com/Dark-Sky07/Simurgh-Tunnel/releases/download/"
                    "v3.2.0/simurgh-3.2.0-noarch.tar.gz"
                ),
            }],
        }
        parsed = updater.parse_release(json.dumps(payload))
        self.assertEqual(parsed["version"], "3.2.0")
        self.assertEqual(parsed["sha256"], "a" * 64)

        payload["assets"][0]["browser_download_url"] = "https://example.invalid/package.tar.gz"
        with self.assertRaises(updater.UpdateError):
            updater.parse_release(json.dumps(payload))

        payload["assets"][0]["browser_download_url"] = (
            "https://github.com/Dark-Sky07/Simurgh-Tunnel/releases/download/"
            "v3.2.0/simurgh-3.2.0-noarch.tar.gz"
        )
        payload["assets"][0]["digest"] = ""
        with self.assertRaises(updater.UpdateError):
            updater.parse_release(json.dumps(payload))


class JsonUpdaterCLITests(unittest.TestCase):
    def test_check_json_reports_update_without_modifying_the_package(self):
        with tempfile.TemporaryDirectory() as temporary:
            app_dir = Path(temporary) / "app"
            write_package(app_dir / "lib", "3.2.6", "keep")
            release = {
                "version": "3.2.7",
                "tag": "v3.2.7",
                "notes_url": "https://github.com/Dark-Sky07/Simurgh-Tunnel/releases/tag/v3.2.7",
            }
            output = io.StringIO()
            with patch.object(updater.os, "geteuid", return_value=0), \
                    patch.object(updater, "running_in_docker", return_value=False), \
                    patch.object(updater, "fetch_latest_release", return_value=release), \
                    redirect_stdout(output):
                code = updater.main(["--check", "--json", "--app-dir", str(app_dir)])

            payload = json.loads(output.getvalue())
            self.assertEqual(code, 0)
            self.assertTrue(payload["ok"])
            self.assertTrue(payload["update_available"])
            self.assertEqual(payload["version"], "3.2.6")
            self.assertEqual(payload["latest"], "3.2.7")
            self.assertFalse(payload["restart_required"])
            self.assertEqual(updater.read_installed_version(app_dir / "lib" / "simurgh"), "3.2.6")
            self.assertEqual((app_dir / "lib" / "simurgh" / "marker.txt").read_text(), "keep")


class ArchiveSafetyTests(unittest.TestCase):
    def test_extracts_only_package_and_license(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive = base / "package.tar.gz"
            digest = make_archive(archive, "3.2.0")
            self.assertEqual(len(digest), 64)
            package, license_file = updater.extract_package(archive, base / "stage")
            self.assertEqual(updater.read_installed_version(package), "3.2.0")
            self.assertEqual((package / "marker.txt").read_text(), "new")
            self.assertEqual(license_file.read_text(), "MIT License\n")

    def test_rejects_path_traversal(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive_path = base / "unsafe.tar.gz"
            with tarfile.open(archive_path, "w:gz") as archive:
                payload = b"no"
                info = tarfile.TarInfo("./usr/lib/simurgh/simurgh/../../outside")
                info.size = len(payload)
                archive.addfile(info, io.BytesIO(payload))
            with self.assertRaises(updater.UpdateError):
                updater.extract_package(archive_path, base / "stage")


class DockerBuildPackageTests(unittest.TestCase):
    def test_build_helper_stages_verified_latest_package(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive = base / "latest.tar.gz"
            digest = make_archive(archive, "3.2.0")
            release = release_for(archive, "3.2.0", digest)
            destination = base / "image" / "lib"

            version = builder.install_latest_package(
                destination,
                release=release,
                download=lambda _url, target: target.write_bytes(archive.read_bytes()),
            )

            self.assertEqual(version, "3.2.0")
            self.assertEqual(updater.read_installed_version(destination / "simurgh"), "3.2.0")
            self.assertEqual((destination / "LICENSE").read_text(), "MIT License\n")
            self.assertFalse(list(destination.parent.glob(".simurgh-build-*")))

    def test_build_helper_rejects_bad_digest_without_leaving_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            archive = base / "latest.tar.gz"
            make_archive(archive, "3.2.0")
            release = release_for(archive, "3.2.0", "0" * 64)
            destination = base / "image" / "lib"

            with self.assertRaises(updater.UpdateError):
                builder.install_latest_package(
                    destination,
                    release=release,
                    download=lambda _url, target: target.write_bytes(archive.read_bytes()),
                )

            self.assertFalse((destination / "simurgh").exists())
            self.assertFalse((destination / "LICENSE").exists())
            self.assertFalse(list(destination.parent.glob(".simurgh-build-*")))


class InstallerBootstrapTests(unittest.TestCase):
    def test_fresh_install_uses_latest_verified_release_and_requires_ui_health(self):
        installer = Path(__file__).with_name("install-simurgh-tunnel.sh").read_text(encoding="utf-8")
        self.assertIn("build_latest_simurgh.py", installer)
        self.assertIn('python3 "$work/build_latest_simurgh.py" "$APP_DIR/lib"', installer)
        self.assertNotIn('SIMURGH_VERSION="3.1.1"', installer)
        self.assertIn("verify_panel", installer)
        self.assertIn('"/healthz"', installer)
        self.assertIn('"/app.js"', installer)
        self.assertIn('"/app.css"', installer)
        self.assertIn('Simurgh did not pass its health and UI checks', installer)

    def test_installer_passes_the_exact_phoenix_release_to_bootstrap(self):
        root = Path(__file__).resolve().parents[1]
        for name in ("install.sh", "install-phoenix.sh"):
            installer = (root / name).read_text(encoding="utf-8")
            self.assertIn('PHOENIX_REF="$tag_version" PHOENIX_REPO="$PHOENIX_REPO" bash "$simurgh_installer"', installer)
        self.assertEqual((root / "install.sh").read_bytes(), (root / "install-phoenix.sh").read_bytes())

    def test_docker_entrypoint_checks_simurgh_without_blocking_phoenix(self):
        root = Path(__file__).resolve().parents[1]
        entrypoint = (root / "DockerEntrypoint.sh").read_text(encoding="utf-8")
        for path in ("/healthz", "/app.js", "/app.css"):
            self.assertIn(path, entrypoint)
        self.assertIn('d.get("product") == "Simurgh Tunnel"', entrypoint)
        self.assertIn(") &", entrypoint)
        self.assertIn("exec /app/x-ui", entrypoint)

    def test_simurgh_navigation_and_browser_title_use_the_requested_name(self):
        root = Path(__file__).resolve().parents[1]
        for name in (
            "frontend/src/layouts/AppSidebar.tsx",
            "frontend/src/components/CommandPalette.tsx",
            "frontend/src/hooks/usePageTitle.ts",
        ):
            source = (root / name).read_text(encoding="utf-8")
            self.assertIn("Simurgh Tunnel", source)
        page_title = (root / "frontend/src/hooks/usePageTitle.ts").read_text(encoding="utf-8")
        self.assertIn("document.title = isSimurghPage ? pageTitle", page_title)


class TransactionTests(unittest.TestCase):
    def test_success_updates_simurgh_only_and_restarts_only_its_service(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            app_dir = base / "opt" / "phoenix-simurgh"
            lib_dir = app_dir / "lib"
            write_package(lib_dir, "3.1.1", "old")
            (lib_dir / "LICENSE").write_text("Old MIT license\n", encoding="utf-8")
            state = base / "etc" / "phoenix-simurgh" / "relay.toml"
            state.parent.mkdir(parents=True)
            state.write_text("keep = true\n", encoding="utf-8")

            archive = base / "package.tar.gz"
            digest = make_archive(archive, "3.2.0")
            release = release_for(archive, "3.2.0", digest)
            manager = FakeServiceManager(active=True)

            result = updater.apply_update(
                app_dir, release, manager,
                download=lambda _url, destination: destination.write_bytes(archive.read_bytes()),
                health_check=lambda: True,
            )

            self.assertIn("updated from v3.1.1 to v3.2.0", result)
            self.assertEqual(updater.read_installed_version(lib_dir / "simurgh"), "3.2.0")
            self.assertEqual((lib_dir / "simurgh" / "marker.txt").read_text(), "new")
            self.assertEqual((lib_dir / "LICENSE").read_text(), "MIT License\n")
            self.assertEqual(state.read_text(), "keep = true\n")
            self.assertEqual((manager.stops, manager.starts, manager.active), (1, 1, True))
            self.assertFalse(list(app_dir.glob(".simurgh-update-*")))

    def test_bad_digest_never_stops_the_running_service_or_changes_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            app_dir = base / "app"
            lib_dir = app_dir / "lib"
            write_package(lib_dir, "3.1.1", "old")
            archive = base / "package.tar.gz"
            make_archive(archive, "3.2.0")
            release = release_for(archive, "3.2.0", "0" * 64)
            manager = FakeServiceManager(active=True)

            with self.assertRaisesRegex(updater.UpdateError, "SHA-256"):
                updater.apply_update(
                    app_dir, release, manager,
                    download=lambda _url, destination: destination.write_bytes(archive.read_bytes()),
                )

            self.assertEqual(updater.read_installed_version(lib_dir / "simurgh"), "3.1.1")
            self.assertEqual((lib_dir / "simurgh" / "marker.txt").read_text(), "old")
            self.assertEqual((manager.stops, manager.starts, manager.active), (0, 0, True))

    def test_failed_new_start_rolls_back_package_and_restarts_old_service(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary)
            app_dir = base / "app"
            lib_dir = app_dir / "lib"
            write_package(lib_dir, "3.1.1", "old")
            (lib_dir / "LICENSE").write_text("Old MIT license\n", encoding="utf-8")
            archive = base / "package.tar.gz"
            digest = make_archive(archive, "3.2.0")
            release = release_for(archive, "3.2.0", digest)
            manager = FakeServiceManager(active=True, fail_first_start=True)

            with self.assertRaisesRegex(updater.UpdateError, "previous files were restored"):
                updater.apply_update(
                    app_dir, release, manager,
                    download=lambda _url, destination: destination.write_bytes(archive.read_bytes()),
                )

            self.assertEqual(updater.read_installed_version(lib_dir / "simurgh"), "3.1.1")
            self.assertEqual((lib_dir / "simurgh" / "marker.txt").read_text(), "old")
            self.assertEqual((lib_dir / "LICENSE").read_text(), "Old MIT license\n")
            self.assertTrue(manager.active)
            self.assertEqual(manager.starts, 2)


if __name__ == "__main__":
    unittest.main()

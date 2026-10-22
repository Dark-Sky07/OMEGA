#!/usr/bin/env python3
"""Offline safety tests for the PHOENIX Simurgh updater."""
from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("update_simurgh.py")
SPEC = importlib.util.spec_from_file_location("update_simurgh", SCRIPT)
assert SPEC and SPEC.loader
updater = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(updater)


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

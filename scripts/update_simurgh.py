#!/usr/bin/env python3
"""Safely update PHOENIX's separately installed Simurgh Python service."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path, PurePosixPath

API_LATEST = "https://api.github.com/repos/Dark-Sky07/Simurgh-Tunnel/releases/latest"
REPO = "Dark-Sky07/Simurgh-Tunnel"
APP_DIR = Path(os.environ.get("PHOENIX_SIMURGH_DIR", "/opt/phoenix-simurgh"))
SERVICE_SYSTEMD = "phoenix-simurgh-relay.service"
SERVICE_OPENRC = "phoenix-simurgh"
MAX_ARCHIVE_BYTES = 20 * 1024 * 1024
MAX_EXTRACTED_BYTES = 40 * 1024 * 1024
VERSION_RE = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)$")


class UpdateError(RuntimeError):
    """The update could not be completed safely."""


def version_tuple(value: str) -> tuple[int, int, int]:
    match = VERSION_RE.fullmatch(value.strip())
    if not match:
        raise UpdateError(f"unsupported Simurgh version format: {value!r}")
    return tuple(int(part) for part in match.groups())


def parse_release(payload: bytes | str) -> dict:
    try:
        release = json.loads(payload)
    except (TypeError, ValueError) as exc:
        raise UpdateError(f"GitHub returned invalid release metadata: {exc}") from exc

    if not isinstance(release, dict):
        raise UpdateError("GitHub returned an invalid release object")
    tag = str(release.get("tag_name") or "")
    version = tag.removeprefix("v")
    version_tuple(version)
    if release.get("draft") or release.get("prerelease"):
        raise UpdateError("the newest Simurgh release is not a published stable release")

    expected_name = f"simurgh-{version}-noarch.tar.gz"
    release_assets = release.get("assets") or []
    if not isinstance(release_assets, list):
        raise UpdateError("GitHub returned an invalid release asset list")
    assets = [asset for asset in release_assets if isinstance(asset, dict)
              and asset.get("name") == expected_name]
    if len(assets) != 1:
        raise UpdateError(f"release {tag} must contain exactly one {expected_name} asset")
    asset = assets[0]
    digest = str(asset.get("digest") or "")
    if not re.fullmatch(r"sha256:[0-9a-fA-F]{64}", digest):
        raise UpdateError(f"release {tag} has no valid GitHub SHA-256 digest for {expected_name}")

    url = str(asset.get("browser_download_url") or "")
    parsed = urllib.parse.urlparse(url)
    expected_path = f"/{REPO}/releases/download/{tag}/{expected_name}"
    if parsed.scheme != "https" or parsed.netloc != "github.com" or parsed.path != expected_path:
        raise UpdateError("GitHub returned an unexpected Simurgh download URL")

    return {
        "tag": tag,
        "version": version,
        "name": str(release.get("name") or tag),
        "notes_url": str(release.get("html_url") or ""),
        "asset_name": expected_name,
        "asset_url": url,
        "sha256": digest.split(":", 1)[1].lower(),
    }


def fetch_latest_release() -> dict:
    request = urllib.request.Request(
        API_LATEST,
        headers={"User-Agent": "PHOENIX-Simurgh-Updater/1", "Accept": "application/vnd.github+json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return parse_release(response.read(2 * 1024 * 1024))
    except urllib.error.HTTPError as exc:
        raise UpdateError(f"GitHub returned HTTP {exc.code} while checking Simurgh updates") from exc
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        raise UpdateError(f"cannot check for Simurgh updates ({exc}); nothing was changed") from exc


def read_installed_version(package_dir: Path) -> str:
    init_file = package_dir / "__init__.py"
    if package_dir.is_symlink() or not init_file.is_file():
        raise UpdateError(f"Simurgh's installed Python package was not found at {package_dir}")
    text = init_file.read_text(encoding="utf-8")
    match = re.search(r"^__version__\s*=\s*['\"]([^'\"]+)['\"]", text, re.MULTILINE)
    if not match:
        raise UpdateError("could not read the installed Simurgh version")
    version_tuple(match.group(1))
    return match.group(1)


def download_asset(url: str, destination: Path) -> None:
    request = urllib.request.Request(url, headers={"User-Agent": "PHOENIX-Simurgh-Updater/1"})
    total = 0
    try:
        with urllib.request.urlopen(request, timeout=60) as response, destination.open("wb") as out:
            while True:
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_ARCHIVE_BYTES:
                    raise UpdateError("Simurgh package exceeds the safe download size limit")
                out.write(chunk)
    except UpdateError:
        raise
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, OSError) as exc:
        raise UpdateError(f"could not download the Simurgh package ({exc})") from exc
    if total == 0:
        raise UpdateError("GitHub returned an empty Simurgh package")


def extract_package(archive_path: Path, stage_dir: Path) -> tuple[Path, Path]:
    package_dir = stage_dir / "simurgh"
    license_file = stage_dir / "LICENSE"
    package_prefix = PurePosixPath("usr/lib/simurgh/simurgh")
    license_path = PurePosixPath("usr/share/doc/simurgh/LICENSE")
    seen: set[str] = set()
    extracted_bytes = 0
    found_init = False
    found_license = False

    try:
        archive = tarfile.open(archive_path, mode="r:gz")
    except (tarfile.TarError, OSError) as exc:
        raise UpdateError(f"downloaded Simurgh package is not a valid gzip tar archive: {exc}") from exc

    with archive:
        members = archive.getmembers()
        if len(members) > 20000:
            raise UpdateError("Simurgh package contains too many archive entries")
        for member in members:
            name = member.name
            while name.startswith("./"):
                name = name[2:]
            path = PurePosixPath(name)
            if "\\" in name or path.is_absolute() or ".." in path.parts:
                raise UpdateError("Simurgh package contains an unsafe path")
            if not (member.isdir() or member.isfile()):
                raise UpdateError("Simurgh package contains an unsupported link or special file")
            if name in seen:
                raise UpdateError("Simurgh package contains duplicate paths")
            seen.add(name)

            is_package = path == package_prefix or package_prefix in path.parents
            is_license = path == license_path
            if not (is_package or is_license):
                continue

            if member.size < 0:
                raise UpdateError("Simurgh package contains an invalid file size")
            extracted_bytes += member.size
            if extracted_bytes > MAX_EXTRACTED_BYTES:
                raise UpdateError("Simurgh package exceeds the safe extraction size limit")

            if is_package:
                suffix = path.relative_to(package_prefix)
                target = package_dir.joinpath(*suffix.parts) if suffix.parts else package_dir
            else:
                target = license_file

            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue

            target.parent.mkdir(parents=True, exist_ok=True)
            source = archive.extractfile(member)
            if source is None:
                raise UpdateError(f"cannot read {member.name} from the Simurgh package")
            with source, target.open("wb") as out:
                shutil.copyfileobj(source, out)
            target.chmod(0o644)
            if is_package and suffix == PurePosixPath("__init__.py"):
                found_init = True
            if is_license:
                found_license = True

    if not found_init or not found_license:
        raise UpdateError("Simurgh package is missing its Python entry point or MIT license")
    for directory in (package_dir, *[path for path in package_dir.rglob("*") if path.is_dir()]):
        directory.chmod(0o755)
    return package_dir, license_file


def verify_staged_package(package_dir: Path, expected_version: str) -> None:
    actual_version = read_installed_version(package_dir)
    if actual_version != expected_version:
        raise UpdateError(
            f"package version mismatch: release says {expected_version}, archive contains {actual_version}"
        )
    env = os.environ.copy()
    env["PYTHONPATH"] = str(package_dir.parent)
    try:
        result = subprocess.run(
            [sys.executable, "-m", "simurgh", "--version"],
            env=env, text=True, capture_output=True, timeout=20, check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise UpdateError(f"could not validate the staged Simurgh package: {exc}") from exc
    expected_line = f"Simurgh Tunnel v{expected_version}"
    if result.returncode != 0 or expected_line not in result.stdout:
        raise UpdateError("staged Simurgh package failed its version smoke test")


def wait_for_health(timeout: float = 20.0) -> bool:
    deadline = time.monotonic() + timeout
    url = "http://127.0.0.1:18787/healthz"
    while time.monotonic() < deadline:
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "PHOENIX-Simurgh-Health/1"})
            with urllib.request.urlopen(request, timeout=2) as response:
                if response.status == 200:
                    return True
        except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, OSError):
            pass
        time.sleep(0.5)
    return False


class ServiceManager:
    def __init__(self, kind: str):
        self.kind = kind

    @classmethod
    def detect(cls) -> "ServiceManager":
        if shutil.which("systemctl"):
            result = subprocess.run(
                ["systemctl", "show", "--property=Version"],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                timeout=10, check=False,
            )
            if result.returncode == 0:
                return cls("systemd")
        if shutil.which("rc-service") and shutil.which("rc-update"):
            return cls("openrc")
        raise UpdateError("no supported systemd/OpenRC manager found; Simurgh was not changed")

    def _run(self, action: str) -> subprocess.CompletedProcess:
        if self.kind == "systemd":
            command = ["systemctl", action, SERVICE_SYSTEMD]
        else:
            command = ["rc-service", SERVICE_OPENRC, action]
        try:
            return subprocess.run(command, text=True, capture_output=True, timeout=30, check=False)
        except (OSError, subprocess.SubprocessError) as exc:
            raise UpdateError(f"could not {action} the Simurgh service: {exc}") from exc

    def is_active(self) -> bool:
        if self.kind == "systemd":
            command = ["systemctl", "is-active", "--quiet", SERVICE_SYSTEMD]
        else:
            command = ["rc-service", SERVICE_OPENRC, "status"]
        try:
            return subprocess.run(
                command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                timeout=10, check=False,
            ).returncode == 0
        except (OSError, subprocess.SubprocessError) as exc:
            raise UpdateError(f"could not check the Simurgh service state: {exc}") from exc

    def stop(self) -> None:
        result = self._run("stop")
        if result.returncode != 0:
            raise UpdateError(f"could not stop the Simurgh service: {(result.stderr or result.stdout).strip()}")

    def start(self) -> None:
        result = self._run("start")
        if result.returncode != 0:
            raise UpdateError(f"could not start the Simurgh service: {(result.stderr or result.stdout).strip()}")


def apply_update(
    app_dir: Path,
    release: dict,
    manager,
    download=download_asset,
    health_check=wait_for_health,
) -> str:
    package_dir = app_dir / "lib" / "simurgh"
    if app_dir.is_symlink() or (app_dir / "lib").is_symlink():
        raise UpdateError("Simurgh install directories contain a symlink; refusing to update them")
    current_version = read_installed_version(package_dir)
    if version_tuple(release["version"]) <= version_tuple(current_version):
        return f"Simurgh v{current_version} is already up to date; no files or services were changed."

    lib_dir = package_dir.parent
    license_path = lib_dir / "LICENSE"
    if license_path.is_symlink():
        raise UpdateError("the installed Simurgh license path is a symlink; refusing to update")
    active = manager.is_active()
    app_dir.mkdir(parents=True, exist_ok=True)
    work_dir = Path(tempfile.mkdtemp(prefix=".simurgh-update-", dir=app_dir))
    old_package = work_dir / "old-simurgh"
    old_license = work_dir / "old-LICENSE"
    staged_root = work_dir / "new"
    staged_root.mkdir()
    package_moved = False
    new_package_moved = False
    license_moved = False
    new_license_moved = False
    stopped = False
    rollback_errors: list[str] = []

    try:
        archive_path = work_dir / release["asset_name"]
        download(release["asset_url"], archive_path)
        digest = hashlib.sha256(archive_path.read_bytes()).hexdigest()
        if digest != release["sha256"]:
            raise UpdateError("Simurgh package SHA-256 does not match GitHub release metadata")

        staged_package, staged_license = extract_package(archive_path, staged_root)
        verify_staged_package(staged_package, release["version"])

        if active:
            manager.stop()
            stopped = True
            if manager.is_active():
                raise UpdateError("Simurgh is still active after the stop request")
        os.replace(package_dir, old_package)
        package_moved = True
        os.replace(staged_package, package_dir)
        new_package_moved = True
        if license_path.exists():
            os.replace(license_path, old_license)
            license_moved = True
        os.replace(staged_license, license_path)
        new_license_moved = True
        verify_staged_package(package_dir, release["version"])

        if active:
            manager.start()
            if not manager.is_active():
                raise UpdateError("Simurgh did not become active after the update")
            if not health_check():
                raise UpdateError("Simurgh's local health endpoint did not recover after the update")
        return (
            f"Simurgh updated from v{current_version} to v{release['version']}. "
            + ("The Simurgh service restarted; PHOENIX and Xray were not restarted."
               if active else "The service was already stopped and remains stopped.")
        )
    except Exception as exc:
        if active and stopped:
            try:
                if manager.is_active():
                    manager.stop()
            except Exception as rollback_exc:
                rollback_errors.append(f"could not stop the new service: {rollback_exc}")

        try:
            if new_package_moved and package_dir.exists():
                shutil.rmtree(package_dir)
            if package_moved and old_package.exists():
                os.replace(old_package, package_dir)
            if new_license_moved and license_path.exists():
                license_path.unlink()
            if license_moved and old_license.exists():
                os.replace(old_license, license_path)
        except Exception as rollback_exc:
            rollback_errors.append(f"could not restore the previous files: {rollback_exc}")

        if active and stopped:
            try:
                manager.start()
                if not manager.is_active():
                    rollback_errors.append("the previous Simurgh service did not become active")
            except Exception as rollback_exc:
                rollback_errors.append(f"could not restart the previous service: {rollback_exc}")

        if rollback_errors:
            raise UpdateError(
                f"Simurgh update failed ({exc}); rollback needs attention: "
                + "; ".join(rollback_errors)
                + f". Recovery files, if present, are in {work_dir}"
            ) from exc
        raise UpdateError(f"Simurgh update failed; previous files were restored: {exc}") from exc
    finally:
        if not rollback_errors:
            shutil.rmtree(work_dir, ignore_errors=True)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Update only PHOENIX's bundled Simurgh service")
    parser.add_argument("--check", action="store_true", help="check for updates without installing")
    parser.add_argument("--yes", action="store_true", help="confirm the Simurgh service restart")
    parser.add_argument("--app-dir", type=Path, default=APP_DIR, help=argparse.SUPPRESS)
    args = parser.parse_args(argv)

    if os.geteuid() != 0:
        print("Run this command as root; no changes were made.", file=sys.stderr)
        return 1
    if os.environ.get("XUI_IN_DOCKER", "").lower() == "true" or Path("/.dockerenv").exists():
        print("Simurgh is bundled in the Docker image. Update/recreate the image; this command made no changes.")
        return 1

    try:
        current = read_installed_version(args.app_dir / "lib" / "simurgh")
        release = fetch_latest_release()
        print(f"Simurgh installed: v{current}; latest stable: v{release['version']}")
        print(f"Release notes: {release['notes_url']}")
        latest_version = version_tuple(release["version"])
        installed_version = version_tuple(current)
        if latest_version == installed_version:
            print("Simurgh is already up to date.")
            return 0
        if latest_version < installed_version:
            raise UpdateError("latest upstream is older than the installed version; refusing to downgrade")
        if args.check:
            print("A Simurgh update is available. No files or services were changed.")
            return 0

        if not args.yes:
            print("Updating restarts only the Simurgh service and may briefly interrupt its tunnel.")
            print("PHOENIX and Xray will not be restarted. Configuration is kept in /etc/phoenix-simurgh.")
            answer = input("Continue? [y/N]: ").strip().lower()
            if answer not in {"y", "yes"}:
                print("Cancelled; no files or services were changed.")
                return 0

        manager = ServiceManager.detect()
        print(apply_update(args.app_dir, release, manager))
        return 0
    except (UpdateError, OSError, subprocess.SubprocessError) as exc:
        print(f"Simurgh update failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

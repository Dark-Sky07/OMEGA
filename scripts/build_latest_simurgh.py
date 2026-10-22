#!/usr/bin/env python3
"""Fetch and safely stage the latest stable Simurgh package in Docker builds."""
from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import sys
import tempfile
from pathlib import Path

from update_simurgh import (
    UpdateError,
    download_asset,
    extract_package,
    fetch_latest_release,
    verify_staged_package,
)


def install_latest_package(
    destination: Path,
    release: dict | None = None,
    download=download_asset,
    fetch=fetch_latest_release,
) -> str:
    """Install a verified noarch package into an empty image build directory."""
    destination = Path(destination)
    if destination.is_symlink():
        raise UpdateError("Simurgh build destination is a symlink")
    destination.mkdir(parents=True, exist_ok=True)
    package_target = destination / "simurgh"
    license_target = destination / "LICENSE"
    if package_target.exists() or package_target.is_symlink() or license_target.exists() or license_target.is_symlink():
        raise UpdateError("Simurgh build destination is not empty; refusing to overwrite files")

    release = release or fetch()
    work_dir = Path(tempfile.mkdtemp(prefix=".simurgh-build-", dir=destination.parent))
    package_installed = False
    try:
        archive_path = work_dir / release["asset_name"]
        download(release["asset_url"], archive_path)
        digest = hashlib.sha256(archive_path.read_bytes()).hexdigest()
        if digest != release["sha256"]:
            raise UpdateError("Simurgh package SHA-256 does not match GitHub release metadata")

        staged_root = work_dir / "new"
        staged_root.mkdir()
        staged_package, staged_license = extract_package(archive_path, staged_root)
        verify_staged_package(staged_package, release["version"])

        os.replace(staged_package, package_target)
        package_installed = True
        os.replace(staged_license, license_target)
        return release["version"]
    except Exception:
        if package_installed and package_target.exists():
            shutil.rmtree(package_target, ignore_errors=True)
        if license_target.exists():
            license_target.unlink()
        raise
    finally:
        shutil.rmtree(work_dir, ignore_errors=True)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("destination", type=Path)
    args = parser.parse_args(argv)
    try:
        version = install_latest_package(args.destination)
    except (UpdateError, OSError, KeyError) as exc:
        print(f"Could not stage the latest Simurgh package: {exc}", file=sys.stderr)
        return 1
    print(f"Staged latest Simurgh v{version} for the Docker image.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

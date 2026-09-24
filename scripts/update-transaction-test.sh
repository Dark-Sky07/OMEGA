#!/usr/bin/env bash
# Static guard for update.sh's failure boundary. The real update is intentionally
# not run in CI: it downloads release assets and controls a host service.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="${root_dir}/update.sh"

bash -n "$script"

python3 - "$script" <<'PY'
from pathlib import Path
import sys

text = Path(sys.argv[1]).read_text()

def require(needle: str) -> None:
    if needle not in text:
        raise SystemExit(f"missing update transaction guard: {needle}")

def order(first: str, second: str) -> None:
    a = text.index(first)
    b = text.index(second)
    if a >= b:
        raise SystemExit(f"expected {first!r} before {second!r}")

# A failed candidate must have an EXIT trap and a database snapshot independent
# of the panel-directory swap. The snapshot covers SQLite sidecars and the
# PostgreSQL dump/restore pair.
require("trap 'rollback_update' EXIT")
require("snapshot_update_database()")
require("restore_update_database()")
require('pg_dump --format=custom')
require('pg_restore --clean --if-exists')
require('for suffix in "" "-wal" "-shm" "-journal"; do')
require("update_db_snapshot_ready=0")

# The old service is stopped before the snapshot, migration is checked before
# the candidate is started, and rollback restores DB state before restarting the
# old service.
order("if ! service_stop_for_update; then return 1; fi", "if ! snapshot_update_database; then")
order("if ! snapshot_update_database; then", "if ! mv \"$xui_folder\" \"$update_backup_dir\"; then")
order("if ! config_after_update; then", "if ! service_start_after_update || ! service_is_active; then")
order("if ! restore_update_database; then", "service_start_after_update >/dev/null 2>&1 || true")

# Migration/configuration failures must escape config_after_update instead of
# being printed and ignored, and helper-driven SSL paths must not start the
# candidate while the transaction is deferred.
require('if ! ${xui_folder}/x-ui migrate; then')
require('return 1\n    fi\n\n    # Properly detect empty cert')
require('update_defer_service_start=1')
require('service_start_for_config()')
require('service_restart_for_config()')

print("update transaction static checks passed")
PY

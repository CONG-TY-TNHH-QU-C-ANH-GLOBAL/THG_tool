#!/usr/bin/env bash
#
# Pre-deploy SQLite online backup — FAIL CLOSED.
#
# Runs ON THE PRODUCTION HOST before any binary replacement, service restart, or
# (startup-embedded) migration. Any failure exits non-zero so the calling deploy
# step (set -euo pipefail) aborts BEFORE `systemctl restart` — services are never
# restarted on top of an un-backed-up database.
#
# Invoked from .github/workflows/deploy.yml "Install and restart" step as:
#   sudo bash /tmp/thg-deploy/scripts/deploy/predeploy_sqlite_backup.sh
#
# It does NOT touch application code, schema, or run any SQL beyond the read-only
# online .backup and PRAGMA quick_check. No destructive SQL. No `rm` outside the
# backup directory; emergency/retention cleanup only ever match the strict
# pattern scraper_predeploy_*.db inside BACKUP_DIR.
set -euo pipefail

# ── 1. Path definitions ───────────────────────────────────────────────────
DB="/opt/thg-scraper/data/scraper.db"
WAL="/opt/thg-scraper/data/scraper.db-wal"
BACKUP_DIR="/opt/thg-scraper/data/backups/deploy_backups"
OWNER="thg-scraper:thg-scraper"   # production data ownership (best-effort chown)
KEEP_MIN=10                       # retention: always keep at least this many newest backups
RETAIN_DAYS=14                    # retention: prune backups older than this BEYOND the newest KEEP_MIN
EMERGENCY_MIN_AGE_DAYS=14         # emergency cleanup only deletes backups older than this

TS="$(date +%Y%m%d_%H%M%S)"
FINAL_FILE="${BACKUP_DIR}/scraper_predeploy_${TS}.db"
TMP_FILE="${FINAL_FILE}.tmp"

log() { echo "[predeploy-backup] $*"; }

log "Pre-deploy SQLite backup starting"
log "target DB path  : ${DB}"
log "backup directory: ${BACKUP_DIR}"

# ── 2. Preconditions (fail closed) ────────────────────────────────────────
if ! command -v sqlite3 >/dev/null 2>&1; then
  log "ABORT: sqlite3 CLI not found on host (required for online .backup + quick_check)"
  exit 1
fi
if [ ! -f "${DB}" ]; then
  log "ABORT: database not found at ${DB}"
  exit 1
fi
if [ ! -s "${DB}" ]; then
  log "ABORT: database at ${DB} is empty (0 bytes)"
  exit 1
fi

mkdir -p "${BACKUP_DIR}"
if [ ! -w "${BACKUP_DIR}" ]; then
  log "ABORT: backup directory ${BACKUP_DIR} is not writable"
  exit 1
fi

# available_bytes echoes the free bytes on the filesystem hosting BACKUP_DIR.
available_bytes() {
  df -PB1 "${BACKUP_DIR}" | awk 'NR==2 {print $4}'
}

# ── 3. Disk-space preflight (deadlock prevention) ─────────────────────────
DB_BYTES="$(stat -c %s "${DB}")"
WAL_BYTES=0
if [ -f "${WAL}" ]; then
  WAL_BYTES="$(stat -c %s "${WAL}")"
fi
SOURCE_BYTES=$(( DB_BYTES + WAL_BYTES ))
REQUIRED_BYTES=$(( SOURCE_BYTES * 3 / 2 ))   # 1.5x headroom (integer-safe)
AVAILABLE_BYTES="$(available_bytes)"

# Defensive: df must yield a positive integer, else we cannot reason about space.
case "${AVAILABLE_BYTES}" in
  ''|*[!0-9]*) log "ABORT: could not determine available bytes for ${BACKUP_DIR}"; exit 1 ;;
esac

log "DB bytes        : ${DB_BYTES}"
log "WAL bytes       : ${WAL_BYTES}"
log "source bytes    : ${SOURCE_BYTES}"
log "required bytes  : ${REQUIRED_BYTES} (1.5x source)"
log "available bytes (before emergency cleanup): ${AVAILABLE_BYTES}"

# ── 4. Emergency cleanup — ONLY if space is insufficient ──────────────────
# Bounded, oldest-first, only files matching scraper_predeploy_*.db inside
# BACKUP_DIR AND older than EMERGENCY_MIN_AGE_DAYS. Recent backups are never
# deleted just to force the deploy through. Filenames are produced solely by
# this script (scraper_predeploy_<TS>.db, no spaces), and the timestamp encodes
# chronological order, so a plain lexical sort is oldest-first.
if [ "${AVAILABLE_BYTES}" -lt "${REQUIRED_BYTES}" ]; then
  log "CRITICAL: Insufficient disk space for SQLite pre-deploy backup"
  log "Emergency cleanup scope: ${BACKUP_DIR}/scraper_predeploy_*.db older than ${EMERGENCY_MIN_AGE_DAYS} days (oldest first)"

  mapfile -t EMERGENCY_CANDIDATES < <(
    find "${BACKUP_DIR}" -maxdepth 1 -type f -name 'scraper_predeploy_*.db' \
         -mtime "+${EMERGENCY_MIN_AGE_DAYS}" | sort
  )

  if [ "${#EMERGENCY_CANDIDATES[@]}" -eq 0 ]; then
    log "ABORT: no backups older than ${EMERGENCY_MIN_AGE_DAYS} days to reclaim; refusing to delete recent backups to force the deploy through"
    exit 1
  fi

  for f in "${EMERGENCY_CANDIDATES[@]}"; do
    # Never delete the file this run is about to create.
    if [ "${f}" = "${TMP_FILE}" ] || [ "${f}" = "${FINAL_FILE}" ]; then
      continue
    fi
    log "Emergency cleanup: deleting ${f} ($(stat -c %s "${f}") bytes)"
    rm -f -- "${f}"
    AVAILABLE_BYTES="$(available_bytes)"
    log "available bytes after deletion: ${AVAILABLE_BYTES}"
    if [ "${AVAILABLE_BYTES}" -ge "${REQUIRED_BYTES}" ]; then
      break
    fi
  done

  if [ "${AVAILABLE_BYTES}" -lt "${REQUIRED_BYTES}" ]; then
    log "ABORT: still insufficient space after emergency cleanup (available=${AVAILABLE_BYTES} required=${REQUIRED_BYTES})"
    exit 1
  fi
  log "available bytes (after emergency cleanup): ${AVAILABLE_BYTES}"
fi

# ── 5. Atomic online backup + verify ──────────────────────────────────────
# Until the final file is committed, clean any partial tmp file on ANY exit.
trap 'rm -f -- "${TMP_FILE}"' EXIT

log "temp backup path: ${TMP_FILE}"
# Online backup via the SQLite backup API (consistent snapshot of a LIVE WAL-mode
# DB; includes committed WAL frames). `-cmd ".timeout 5000"` sets a 5s busy
# timeout BEFORE .backup so a concurrent writer does not fail it. -cmd is used
# instead of passing two trailing dot-commands (the sqlite3 CLI only runs the
# last positional command, so ".timeout" as a positional would be ignored).
sqlite3 -cmd ".timeout 5000" "${DB}" ".backup '${TMP_FILE}'"

if [ ! -s "${TMP_FILE}" ]; then
  log "ABORT: backup temp file missing or zero bytes after .backup"
  exit 1   # trap removes TMP_FILE
fi

QUICK_CHECK="$(sqlite3 "${TMP_FILE}" 'PRAGMA quick_check;')"
if [ "${QUICK_CHECK}" != "ok" ]; then
  log "ABORT: quick_check did not return exactly 'ok' (got: ${QUICK_CHECK})"
  exit 1   # trap removes TMP_FILE
fi
log "quick_check=ok"

# Commit: atomic rename, then disarm the cleanup trap so the final file survives.
mv -- "${TMP_FILE}" "${FINAL_FILE}"
trap - EXIT

chmod 600 "${FINAL_FILE}"
# chown is best-effort: a verified root-owned 0600 backup is still a valid backup,
# so a missing user/group must not abort an otherwise-good deploy.
chown "${OWNER}" "${FINAL_FILE}" 2>/dev/null || log "WARN: chown ${OWNER} failed (backup kept root-owned, mode 600)"

FINAL_BYTES="$(stat -c %s "${FINAL_FILE}")"
log "final backup path: ${FINAL_FILE}"
log "final backup size: ${FINAL_BYTES} bytes"

# ── 6. Normal retention — ONLY after a verified backup ────────────────────
# Keep the newest KEEP_MIN backups ALWAYS; among the rest, delete those older
# than RETAIN_DAYS. The just-created backup is the newest, so it is always kept.
mapfile -t ALL_BACKUPS < <(
  find "${BACKUP_DIR}" -maxdepth 1 -type f -name 'scraper_predeploy_*.db' | sort -r
)

deleted=0
idx=0
for f in "${ALL_BACKUPS[@]}"; do
  idx=$(( idx + 1 ))
  [ "${idx}" -le "${KEEP_MIN}" ] && continue          # always keep newest KEEP_MIN
  [ "${f}" = "${FINAL_FILE}" ] && continue            # never the one just created
  if [ -n "$(find "${f}" -maxdepth 0 -mtime "+${RETAIN_DAYS}" -print 2>/dev/null)" ]; then
    rm -f -- "${f}"
    deleted=$(( deleted + 1 ))
  fi
done
log "retention cleanup: deleted ${deleted} old backup(s) (kept newest ${KEEP_MIN}; pruned > ${RETAIN_DAYS} days beyond that)"

log "Pre-deploy SQLite backup completed successfully"

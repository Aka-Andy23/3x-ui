#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_ROOT="${XUI_BACKUP_ROOT:-/var/backups/x-ui}"
STATE_ROOT="${XUI_STATE_ROOT:-/var/log/x-ui-maintenance}"
LOG_PATH="${STATE_ROOT}/rollback-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/rollback-${RUN_ID}.report"
BACKUP_PATH="${BACKUP_ROOT}/rollback-${RUN_ID}"
SOURCE_ARCHIVE="${1:-}"

mkdir -p -- "${STATE_ROOT}" "${BACKUP_PATH}"
exec > >(tee -a "${LOG_PATH}") 2>&1

write_report() {
  local status="$1"
  {
    echo "status=${status}"
    echo "operation=rollback"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
    echo "backup=${BACKUP_PATH}"
    echo "source=${SOURCE_ARCHIVE}"
  } > "${REPORT_PATH}"
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  set +e
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  write_report "FAILED"
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  echo "BACKUP=${BACKUP_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM

echo "STEP 1/5 preflight"
[[ "${EUID}" -eq 0 ]]
for command_name in bash find mkdir sort systemctl tee timeout; do
  command -v "${command_name}" >/dev/null
done
[[ -x "${SCRIPT_DIR}/restore.sh" ]]
if [[ -z "${SOURCE_ARCHIVE}" ]]; then
  SOURCE_ARCHIVE="$(find "${BACKUP_ROOT}" -maxdepth 3 -type f \( -name 'x-ui-backup-*.tar.gz' -o -name 'pre-update.tar.gz' -o -name 'pre-restore.tar.gz' \) | sort | tail -n 1)"
fi
[[ -n "${SOURCE_ARCHIVE}" && -f "${SOURCE_ARCHIVE}" ]]

echo "STEP 2/5 create safety backup"
XUI_BACKUP_ARCHIVE="${BACKUP_PATH}/pre-rollback.tar.gz" timeout 300 "${SCRIPT_DIR}/backup.sh"
[[ -s "${BACKUP_PATH}/pre-rollback.tar.gz" ]]

echo "STEP 3/5 restore selected backup"
timeout 300 "${SCRIPT_DIR}/restore.sh" "${SOURCE_ARCHIVE}"

echo "STEP 4/5 selfcheck"
timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'
timeout 15 "${XUI_INSTALL_DIR:-/usr/local/x-ui}/x-ui" -v

echo "STEP 5/5 finalize"
write_report "DONE"
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"
echo "BACKUP=${BACKUP_PATH}"

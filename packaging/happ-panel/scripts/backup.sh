#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
INSTALL_DIR="${XUI_INSTALL_DIR:-/usr/local/x-ui}"
DATA_DIR="${XUI_DATA_DIR:-/etc/x-ui}"
SERVICE_PATH="${XUI_SERVICE_PATH:-/etc/systemd/system/x-ui.service}"
BACKUP_ROOT="${XUI_BACKUP_ROOT:-/var/backups/x-ui}"
STATE_ROOT="${XUI_STATE_ROOT:-/var/log/x-ui-maintenance}"
LOG_PATH="${STATE_ROOT}/backup-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/backup-${RUN_ID}.report"
BACKUP_PATH="${BACKUP_ROOT}/backup-${RUN_ID}"
ARCHIVE_PATH="${XUI_BACKUP_ARCHIVE:-${BACKUP_ROOT}/x-ui-backup-${RUN_ID}.tar.gz}"
WORK_PATH=""
SERVICE_WAS_ACTIVE=0

mkdir -p -- "${STATE_ROOT}" "${BACKUP_ROOT}" "${BACKUP_PATH}"
exec > >(tee -a "${LOG_PATH}") 2>&1

write_report() {
  local status="$1"
  {
    echo "status=${status}"
    echo "operation=backup"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
    echo "backup=${BACKUP_PATH}"
    echo "archive=${ARCHIVE_PATH}"
  } > "${REPORT_PATH}"
}

resume_service() {
  if [[ "${SERVICE_WAS_ACTIVE}" -eq 1 ]]; then
    timeout 30 systemctl start x-ui.service >/dev/null 2>&1 || true
  fi
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  set +e
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  resume_service
  [[ -n "${WORK_PATH}" && -d "${WORK_PATH}" ]] && find "${WORK_PATH}" -mindepth 1 -delete
  [[ -n "${WORK_PATH}" && -d "${WORK_PATH}" ]] && rmdir "${WORK_PATH}" 2>/dev/null
  [[ -f "${ARCHIVE_PATH}" ]] && mv -- "${ARCHIVE_PATH}" "${ARCHIVE_PATH}.failed"
  write_report "FAILED"
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  echo "BACKUP=${BACKUP_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM

echo "STEP 1/6 preflight"
[[ "${EUID}" -eq 0 ]]
[[ "${INSTALL_DIR}" == /* && "${INSTALL_DIR}" != "/" ]]
[[ "${DATA_DIR}" == /* && "${DATA_DIR}" != "/" ]]
for command_name in bash dirname find install mkdir mktemp mv sha256sum sort systemctl tar tee timeout xargs; do
  command -v "${command_name}" >/dev/null
done
if [[ "${XUI_DB_TYPE:-sqlite}" == "postgres" ]]; then
  command -v pg_dump >/dev/null
  [[ -n "${XUI_DB_DSN:-}" ]]
fi

echo "STEP 2/6 prepare consistent snapshot"
WORK_PATH="$(mktemp -d /tmp/x-ui-backup.XXXXXX)"
mkdir -p -- "${WORK_PATH}/payload"
if systemctl is-active --quiet x-ui.service; then
  SERVICE_WAS_ACTIVE=1
  timeout 30 systemctl stop x-ui.service
fi

echo "STEP 3/6 back up application and database"
if [[ -d "${INSTALL_DIR}" ]]; then
  tar -czf "${WORK_PATH}/payload/app.tar.gz" -C "$(dirname -- "${INSTALL_DIR}")" "$(basename -- "${INSTALL_DIR}")"
fi
if [[ -d "${DATA_DIR}" ]]; then
  tar -czf "${WORK_PATH}/payload/data.tar.gz" -C "$(dirname -- "${DATA_DIR}")" "$(basename -- "${DATA_DIR}")"
fi
if [[ -f "${SERVICE_PATH}" ]]; then
  install -m 0600 "${SERVICE_PATH}" "${WORK_PATH}/payload/x-ui.service"
fi
if [[ "${XUI_DB_TYPE:-sqlite}" == "postgres" ]]; then
  timeout 120 pg_dump --format=custom --file="${WORK_PATH}/payload/postgres.dump" "${XUI_DB_DSN}"
fi

echo "STEP 4/6 restart and selfcheck"
resume_service
if [[ "${SERVICE_WAS_ACTIVE}" -eq 1 ]]; then
  timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'
fi

echo "STEP 5/6 create archive and checksums"
(
  cd -- "${WORK_PATH}/payload"
  find . -type f -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
)
mkdir -p -- "$(dirname -- "${ARCHIVE_PATH}")"
tar -czf "${ARCHIVE_PATH}" -C "${WORK_PATH}" payload
sha256sum "${ARCHIVE_PATH}" > "${ARCHIVE_PATH}.sha256"
install -m 0600 "${ARCHIVE_PATH}" "${BACKUP_PATH}/$(basename -- "${ARCHIVE_PATH}")"
install -m 0600 "${ARCHIVE_PATH}.sha256" "${BACKUP_PATH}/$(basename -- "${ARCHIVE_PATH}.sha256")"

echo "STEP 6/6 finalize"
find "${WORK_PATH}" -mindepth 1 -delete
rmdir "${WORK_PATH}"
WORK_PATH=""
write_report "DONE"
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"
echo "BACKUP=${BACKUP_PATH}"
echo "ARCHIVE=${ARCHIVE_PATH}"

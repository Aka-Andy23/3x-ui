#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_ROOT="${XUI_BACKUP_ROOT:-/var/backups/x-ui}"
STATE_ROOT="${XUI_STATE_ROOT:-/var/log/x-ui-maintenance}"
LOG_PATH="${STATE_ROOT}/update-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/update-${RUN_ID}.report"
BACKUP_PATH="${BACKUP_ROOT}/update-${RUN_ID}"
PREUPDATE_ARCHIVE="${BACKUP_PATH}/pre-update.tar.gz"
ROLLBACK_READY=0

mkdir -p -- "${STATE_ROOT}" "${BACKUP_PATH}"
exec > >(tee -a "${LOG_PATH}") 2>&1

write_report() {
  local status="$1"
  {
    echo "status=${status}"
    echo "operation=update"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
    echo "backup=${BACKUP_PATH}"
    echo "preupdate_archive=${PREUPDATE_ARCHIVE}"
  } > "${REPORT_PATH}"
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  set +e
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  if [[ "${ROLLBACK_READY}" -eq 1 && -f "${PREUPDATE_ARCHIVE}" ]]; then
    XUI_SKIP_SAFETY_BACKUP=1 timeout 300 "${SCRIPT_DIR}/restore.sh" "${PREUPDATE_ARCHIVE}" || true
  fi
  write_report "FAILED"
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  echo "BACKUP=${BACKUP_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM

echo "STEP 1/6 preflight"
[[ "${EUID}" -eq 0 ]]
for command_name in bash mkdir tee timeout systemctl; do
  command -v "${command_name}" >/dev/null
done
for required in backup.sh install.sh restore.sh; do
  [[ -x "${SCRIPT_DIR}/${required}" ]]
done

echo "STEP 2/6 create pre-update backup"
XUI_BACKUP_ARCHIVE="${PREUPDATE_ARCHIVE}" timeout 300 "${SCRIPT_DIR}/backup.sh"
[[ -s "${PREUPDATE_ARCHIVE}" ]]
ROLLBACK_READY=1

echo "STEP 3/6 validate candidate"
timeout 15 "${SCRIPT_DIR}/../bin/x-ui" -v
timeout 15 "${SCRIPT_DIR}/../bin/xray-linux-amd64" version
if [[ -s "${XUI_DATA_DIR:-/etc/x-ui}/config.json" ]]; then
  timeout 25 "${SCRIPT_DIR}/../bin/xray-linux-amd64" run -test -c "${XUI_DATA_DIR:-/etc/x-ui}/config.json"
fi

echo "STEP 4/6 install update"
XUI_OPERATION=update timeout 300 "${SCRIPT_DIR}/install.sh"

echo "STEP 5/6 selfcheck"
timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'
timeout 15 "${XUI_INSTALL_DIR:-/usr/local/x-ui}/x-ui" -v
timeout 15 "${XUI_INSTALL_DIR:-/usr/local/x-ui}/bin/xray-linux-amd64" version

echo "STEP 6/6 finalize"
ROLLBACK_READY=0
write_report "DONE"
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"
echo "BACKUP=${BACKUP_PATH}"

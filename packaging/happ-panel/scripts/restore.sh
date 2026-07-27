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
LOG_PATH="${STATE_ROOT}/restore-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/restore-${RUN_ID}.report"
BACKUP_PATH="${BACKUP_ROOT}/restore-${RUN_ID}"
SAFETY_ARCHIVE="${BACKUP_PATH}/pre-restore.tar.gz"
SOURCE_ARCHIVE="${1:-}"
WORK_PATH=""
ROLLBACK_READY=0
SERVICE_WAS_ACTIVE=0

mkdir -p -- "${STATE_ROOT}" "${BACKUP_PATH}"
exec > >(tee -a "${LOG_PATH}") 2>&1

write_report() {
  local status="$1"
  {
    echo "status=${status}"
    echo "operation=restore"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
    echo "backup=${BACKUP_PATH}"
    echo "source=${SOURCE_ARCHIVE}"
    echo "safety_archive=${SAFETY_ARCHIVE}"
  } > "${REPORT_PATH}"
}

extract_archive() {
  local archive="$1"
  local destination="$2"
  local entry
  while IFS= read -r entry; do
    [[ "${entry}" != /* ]]
    [[ "/${entry}/" != *"/../"* ]]
    [[ "${entry}" != *\\* ]]
  done < <(tar -tzf "${archive}")
  mkdir -p -- "${destination}"
  tar -xzf "${archive}" -C "${destination}"
  [[ -d "${destination}/payload" ]]
  (
    cd -- "${destination}/payload"
    timeout 120 sha256sum -c SHA256SUMS
  )
}

apply_payload() {
  local payload="$1"
  timeout 30 systemctl stop x-ui.service >/dev/null 2>&1 || true
  if [[ -f "${payload}/app.tar.gz" ]]; then
    if [[ -d "${INSTALL_DIR}" ]]; then
      find "${INSTALL_DIR}" -mindepth 1 -delete
      rmdir "${INSTALL_DIR}"
    fi
    mkdir -p -- "$(dirname -- "${INSTALL_DIR}")"
    tar -xzf "${payload}/app.tar.gz" -C "$(dirname -- "${INSTALL_DIR}")"
  fi
  if [[ -f "${payload}/data.tar.gz" ]]; then
    if [[ -d "${DATA_DIR}" ]]; then
      find "${DATA_DIR}" -mindepth 1 -delete
      rmdir "${DATA_DIR}"
    fi
    mkdir -p -- "$(dirname -- "${DATA_DIR}")"
    tar -xzf "${payload}/data.tar.gz" -C "$(dirname -- "${DATA_DIR}")"
  fi
  if [[ -f "${payload}/x-ui.service" ]]; then
    install -m 0644 "${payload}/x-ui.service" "${SERVICE_PATH}"
  fi
  if [[ -f "${payload}/postgres.dump" ]]; then
    [[ -n "${XUI_DB_DSN:-}" ]]
    timeout 180 pg_restore --clean --if-exists --no-owner --dbname="${XUI_DB_DSN}" "${payload}/postgres.dump"
  fi
  systemctl daemon-reload
  timeout 30 systemctl start x-ui.service
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  set +e
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  if [[ "${ROLLBACK_READY}" -eq 1 && -f "${SAFETY_ARCHIVE}" ]]; then
    local rollback_work
    rollback_work="$(mktemp -d /tmp/x-ui-restore-rollback.XXXXXX)"
    extract_archive "${SAFETY_ARCHIVE}" "${rollback_work}" || true
    apply_payload "${rollback_work}/payload" || true
    find "${rollback_work}" -mindepth 1 -delete
    rmdir "${rollback_work}" 2>/dev/null || true
  elif [[ "${SERVICE_WAS_ACTIVE}" -eq 1 ]]; then
    systemctl start x-ui.service >/dev/null 2>&1 || true
  fi
  [[ -n "${WORK_PATH}" && -d "${WORK_PATH}" ]] && find "${WORK_PATH}" -mindepth 1 -delete
  [[ -n "${WORK_PATH}" && -d "${WORK_PATH}" ]] && rmdir "${WORK_PATH}" 2>/dev/null
  write_report "FAILED"
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  echo "BACKUP=${BACKUP_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM

echo "STEP 1/8 preflight"
[[ "${EUID}" -eq 0 ]]
[[ -n "${SOURCE_ARCHIVE}" && -f "${SOURCE_ARCHIVE}" ]]
[[ "${INSTALL_DIR}" == /* && "${INSTALL_DIR}" != "/" ]]
[[ "${DATA_DIR}" == /* && "${DATA_DIR}" != "/" ]]
for command_name in bash basename dirname find grep install mkdir mktemp sha256sum systemctl tar tee timeout; do
  command -v "${command_name}" >/dev/null
done
if tar -tzf "${SOURCE_ARCHIVE}" | grep -q '^payload/postgres.dump$'; then
  command -v pg_restore >/dev/null
fi

echo "STEP 2/8 verify source archive"
if [[ -f "${SOURCE_ARCHIVE}.sha256" ]]; then
  (
    cd -- "$(dirname -- "${SOURCE_ARCHIVE}")"
    timeout 120 sha256sum -c "$(basename -- "${SOURCE_ARCHIVE}.sha256")"
  )
fi
WORK_PATH="$(mktemp -d /tmp/x-ui-restore.XXXXXX)"
extract_archive "${SOURCE_ARCHIVE}" "${WORK_PATH}"

echo "STEP 3/8 create safety backup"
if systemctl is-active --quiet x-ui.service; then
  SERVICE_WAS_ACTIVE=1
fi
if [[ "${XUI_SKIP_SAFETY_BACKUP:-0}" != "1" ]]; then
  XUI_BACKUP_ARCHIVE="${SAFETY_ARCHIVE}" timeout 300 "${SCRIPT_DIR}/backup.sh"
  [[ -s "${SAFETY_ARCHIVE}" ]]
  ROLLBACK_READY=1
fi

echo "STEP 4/8 stop service"
timeout 30 systemctl stop x-ui.service >/dev/null 2>&1 || true

echo "STEP 5/8 restore payload"
apply_payload "${WORK_PATH}/payload"

echo "STEP 6/8 validate restored Xray configuration"
timeout 15 "${INSTALL_DIR}/x-ui" -v
timeout 15 "${INSTALL_DIR}/bin/xray-linux-amd64" version
if [[ -s "${DATA_DIR}/config.json" ]]; then
  timeout 25 "${INSTALL_DIR}/bin/xray-linux-amd64" run -test -c "${DATA_DIR}/config.json"
fi

echo "STEP 7/8 service selfcheck"
timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'

echo "STEP 8/8 finalize"
find "${WORK_PATH}" -mindepth 1 -delete
rmdir "${WORK_PATH}"
WORK_PATH=""
ROLLBACK_READY=0
write_report "DONE"
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"
echo "BACKUP=${BACKUP_PATH}"

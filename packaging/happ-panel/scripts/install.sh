#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
PACKAGE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd -P)"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
INSTALL_DIR="${XUI_INSTALL_DIR:-/usr/local/x-ui}"
DATA_DIR="${XUI_DATA_DIR:-/etc/x-ui}"
SERVICE_PATH="${XUI_SERVICE_PATH:-/etc/systemd/system/x-ui.service}"
BACKUP_ROOT="${XUI_BACKUP_ROOT:-/var/backups/x-ui}"
STATE_ROOT="${XUI_STATE_ROOT:-/var/log/x-ui-maintenance}"
LOG_PATH="${STATE_ROOT}/install-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/install-${RUN_ID}.report"
BACKUP_PATH="${BACKUP_ROOT}/install-${RUN_ID}"
STAGE_PATH=""
SERVICE_WAS_ACTIVE=0
SERVICE_EXISTED=0
ROLLBACK_READY=0

mkdir -p -- "${STATE_ROOT}" "${BACKUP_PATH}"
exec > >(tee -a "${LOG_PATH}") 2>&1

write_report() {
  local status="$1"
  {
    echo "status=${status}"
    echo "operation=${XUI_OPERATION:-install}"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
    echo "backup=${BACKUP_PATH}"
    echo "install_dir=${INSTALL_DIR}"
    echo "data_dir=${DATA_DIR}"
  } > "${REPORT_PATH}"
}

restore_previous() {
  set +e
  if [[ "${ROLLBACK_READY}" -eq 1 ]]; then
    systemctl stop x-ui.service >/dev/null 2>&1 || true
    if [[ -d "${INSTALL_DIR}" ]]; then
      find "${INSTALL_DIR}" -mindepth 1 -delete
      rmdir "${INSTALL_DIR}" 2>/dev/null || true
    fi
    if [[ -f "${BACKUP_PATH}/app.tar.gz" ]]; then
      mkdir -p -- "$(dirname -- "${INSTALL_DIR}")"
      tar -xzf "${BACKUP_PATH}/app.tar.gz" -C "$(dirname -- "${INSTALL_DIR}")"
    fi
    if [[ -f "${BACKUP_PATH}/x-ui.service" ]]; then
      install -m 0644 "${BACKUP_PATH}/x-ui.service" "${SERVICE_PATH}"
    elif [[ "${SERVICE_EXISTED}" -eq 0 && -f "${SERVICE_PATH}" ]]; then
      find "${SERVICE_PATH}" -maxdepth 0 -type f -delete
    fi
    systemctl daemon-reload >/dev/null 2>&1 || true
    if [[ "${SERVICE_WAS_ACTIVE}" -eq 1 ]]; then
      systemctl start x-ui.service >/dev/null 2>&1 || true
    fi
  fi
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  restore_previous
  write_report "FAILED"
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  echo "BACKUP=${BACKUP_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM

echo "STEP 1/8 preflight"
[[ "${EUID}" -eq 0 ]]
[[ "${INSTALL_DIR}" == /* && "${INSTALL_DIR}" != "/" ]]
[[ "${DATA_DIR}" == /* && "${DATA_DIR}" != "/" ]]
for command_name in bash basename cp dirname find install mkdir mktemp mv sha256sum systemctl tar tee timeout; do
  command -v "${command_name}" >/dev/null
done
for required in bin/x-ui bin/xray-linux-amd64 bin/geoip.dat bin/geosite.dat x-ui.service x-ui.sh SHA256SUMS; do
  [[ -f "${PACKAGE_DIR}/${required}" ]]
done

echo "STEP 2/8 verify package checksums"
(
  cd -- "${PACKAGE_DIR}"
  timeout 120 sha256sum -c SHA256SUMS
)
timeout 15 "${PACKAGE_DIR}/bin/x-ui" -v
timeout 15 "${PACKAGE_DIR}/bin/xray-linux-amd64" version

echo "STEP 3/8 back up current installation"
if systemctl is-active --quiet x-ui.service; then
  SERVICE_WAS_ACTIVE=1
fi
if [[ -d "${INSTALL_DIR}" ]]; then
  tar -czf "${BACKUP_PATH}/app.tar.gz" -C "$(dirname -- "${INSTALL_DIR}")" "$(basename -- "${INSTALL_DIR}")"
fi
if [[ -f "${SERVICE_PATH}" ]]; then
  SERVICE_EXISTED=1
  install -m 0600 "${SERVICE_PATH}" "${BACKUP_PATH}/x-ui.service"
fi
ROLLBACK_READY=1

echo "STEP 4/8 validate Xray against current configuration"
if [[ -s "${DATA_DIR}/config.json" ]]; then
  timeout 25 "${PACKAGE_DIR}/bin/xray-linux-amd64" run -test -c "${DATA_DIR}/config.json"
fi

echo "STEP 5/8 stop service and stage files"
systemctl stop x-ui.service >/dev/null 2>&1 || true
mkdir -p -- "$(dirname -- "${INSTALL_DIR}")" "${DATA_DIR}"
STAGE_PATH="$(mktemp -d "$(dirname -- "${INSTALL_DIR}")/.x-ui-stage.XXXXXX")"
install -m 0755 "${PACKAGE_DIR}/bin/x-ui" "${STAGE_PATH}/x-ui"
install -m 0755 "${PACKAGE_DIR}/x-ui.sh" "${STAGE_PATH}/x-ui.sh"
mkdir -p -- "${STAGE_PATH}/bin"
install -m 0755 "${PACKAGE_DIR}/bin/xray-linux-amd64" "${STAGE_PATH}/bin/xray-linux-amd64"
install -m 0644 "${PACKAGE_DIR}/bin/geoip.dat" "${STAGE_PATH}/bin/geoip.dat"
install -m 0644 "${PACKAGE_DIR}/bin/geosite.dat" "${STAGE_PATH}/bin/geosite.dat"
mkdir -p -- "${STAGE_PATH}/scripts"
for maintenance_script in "${PACKAGE_DIR}"/scripts/*.sh; do
  install -m 0755 "${maintenance_script}" "${STAGE_PATH}/scripts/$(basename -- "${maintenance_script}")"
done
if [[ -d "${PACKAGE_DIR}/docs" ]]; then
  cp -a -- "${PACKAGE_DIR}/docs" "${STAGE_PATH}/docs"
fi

echo "STEP 6/8 install atomically"
if [[ -d "${INSTALL_DIR}" ]]; then
  find "${INSTALL_DIR}" -mindepth 1 -delete
  rmdir "${INSTALL_DIR}"
fi
mv -- "${STAGE_PATH}" "${INSTALL_DIR}"
STAGE_PATH=""
install -m 0644 "${PACKAGE_DIR}/x-ui.service" "${SERVICE_PATH}"
systemctl daemon-reload
systemctl enable x-ui.service >/dev/null

echo "STEP 7/8 start and selfcheck"
timeout 30 systemctl start x-ui.service
timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'
timeout 15 "${INSTALL_DIR}/x-ui" -v
timeout 15 "${INSTALL_DIR}/bin/xray-linux-amd64" version
if [[ -s "${DATA_DIR}/config.json" ]]; then
  timeout 25 "${INSTALL_DIR}/bin/xray-linux-amd64" run -test -c "${DATA_DIR}/config.json"
fi

echo "STEP 8/8 finalize"
ROLLBACK_READY=0
write_report "DONE"
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"
echo "BACKUP=${BACKUP_PATH}"

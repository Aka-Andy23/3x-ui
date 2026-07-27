#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

usage() {
  echo "Usage: sudo $0 [--yes] [--check-only] [release.tar.gz]"
  echo "Without an archive, the release and its SHA-256 file are downloaded from GitHub."
  echo "Online:  sudo $0 --yes"
  echo "Offline: sudo $0 --yes ./xray-panel-3x-ui-v3.5.0-happ.1-linux-amd64-release.tar.gz"
}

RELEASE_VERSION="v3.5.0-happ.1"
REPOSITORY="Aka-Andy23/3x-ui"
ARCHIVE_NAME="xray-panel-3x-ui-v3.5.0-happ.1-linux-amd64-release.tar.gz"
RELEASE_BASE_URL="https://github.com/${REPOSITORY}/releases/download/${RELEASE_VERSION}"
ARCHIVE_URL="${XUI_RELEASE_ARCHIVE_URL:-${RELEASE_BASE_URL}/${ARCHIVE_NAME}}"
CHECKSUM_URL="${XUI_RELEASE_CHECKSUM_URL:-${ARCHIVE_URL}.sha256}"

ASSUME_YES=0
CHECK_ONLY=0
ARCHIVE_INPUT=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --yes|-y)
      ASSUME_YES=1
      ;;
    --check-only)
      CHECK_ONLY=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "${ARCHIVE_INPUT}" ]]; then
        echo "Only one archive may be specified" >&2
        usage >&2
        exit 2
      fi
      ARCHIVE_INPUT="$1"
      ;;
  esac
  shift
done

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
STATE_ROOT="${XUI_STATE_ROOT:-/var/log/x-ui-maintenance}"
if [[ "${CHECK_ONLY}" -eq 1 && "${EUID}" -ne 0 && "${XUI_STATE_ROOT+x}" != "x" ]]; then
  STATE_ROOT="/tmp/x-ui-maintenance"
fi
LOG_PATH="${STATE_ROOT}/archive-install-${RUN_ID}.log"
REPORT_PATH="${STATE_ROOT}/archive-install-${RUN_ID}.report"
WORK_PATH=""
PACKAGE_PATH=""
ARCHIVE_PATH=""
CHECKSUM_PATH=""
STATUS="FAILED"
INSTALL_TIMEOUT="${XUI_INSTALL_TIMEOUT:-900}"
DOWNLOAD_TIMEOUT="${XUI_DOWNLOAD_TIMEOUT:-300}"

mkdir -p -- "${STATE_ROOT}"
exec > >(tee -a "${LOG_PATH}") 2>&1

download_https() {
  local url="$1"
  local destination="$2"
  local temporary="${destination}.part"

  [[ "${url}" == https://* ]] || fail "Only HTTPS download URLs are allowed: ${url}"
  if command -v curl >/dev/null; then
    timeout --preserve-status "${DOWNLOAD_TIMEOUT}" \
      curl --fail --silent --show-error --location \
        --proto '=https' --proto-redir '=https' --tlsv1.2 \
        --connect-timeout 20 --max-time "${DOWNLOAD_TIMEOUT}" \
        --retry 3 --retry-delay 2 --output "${temporary}" "${url}"
  elif command -v wget >/dev/null; then
    timeout --preserve-status "${DOWNLOAD_TIMEOUT}" \
      wget --https-only --secure-protocol=TLSv1_2 \
        --timeout=20 --tries=3 --output-document="${temporary}" "${url}"
  else
    fail "curl or wget is required to download the release"
  fi
  [[ -s "${temporary}" ]] || fail "Downloaded file is empty: ${url}"
  mv -- "${temporary}" "${destination}"
}

write_report() {
  {
    echo "status=${STATUS}"
    echo "operation=archive-install"
    echo "mode=$([[ "${CHECK_ONLY}" -eq 1 ]] && echo check-only || echo install)"
    echo "archive=${ARCHIVE_PATH:-${ARCHIVE_INPUT}}"
    echo "archive_url=$([[ -z "${ARCHIVE_INPUT}" ]] && echo "${ARCHIVE_URL}" || echo local)"
    echo "checksum=${CHECKSUM_PATH}"
    echo "package=${PACKAGE_PATH}"
    echo "log=${LOG_PATH}"
    echo "report=${REPORT_PATH}"
  } > "${REPORT_PATH}"
}

cleanup() {
  set +e
  if [[ -n "${WORK_PATH}" && -d "${WORK_PATH}" ]]; then
    find "${WORK_PATH}" -mindepth 1 -delete
    rmdir "${WORK_PATH}" 2>/dev/null || true
  fi
}

fail() {
  echo "$*" >&2
  return 1
}

on_error() {
  local code="$?"
  trap - ERR INT TERM
  set +e
  echo "FAILED at line ${BASH_LINENO[0]:-unknown}, exit ${code}"
  echo "The nested installer performs its own rollback if installation started."
  cleanup
  write_report
  echo "LOG=${LOG_PATH}"
  echo "REPORT=${REPORT_PATH}"
  exit "${code}"
}

trap on_error ERR INT TERM
trap cleanup EXIT

echo "STEP 1/9 preflight"
if [[ "${CHECK_ONLY}" -eq 0 ]]; then
  if [[ "${EUID}" -ne 0 ]]; then
    fail "Run installation as root: sudo bash $0 --yes ${ARCHIVE_INPUT}"
  fi
fi
if [[ "$(uname -s)" != "Linux" ]]; then
  fail "Only Linux is supported"
fi
case "$(uname -m)" in
  x86_64|amd64) ;;
  *)
    fail "This release requires linux/amd64; detected $(uname -m)"
    ;;
esac
[[ "${INSTALL_TIMEOUT}" =~ ^[0-9]+$ ]]
(( INSTALL_TIMEOUT >= 60 && INSTALL_TIMEOUT <= 3600 ))
[[ "${DOWNLOAD_TIMEOUT}" =~ ^[0-9]+$ ]]
(( DOWNLOAD_TIMEOUT >= 30 && DOWNLOAD_TIMEOUT <= 1800 ))
for command_name in awk bash basename date df dirname find grep mkdir mktemp mv pwd rmdir sha256sum stat systemctl tar tee timeout uname; do
  command -v "${command_name}" >/dev/null
done
if [[ "${CHECK_ONLY}" -eq 0 && ! -d /run/systemd/system ]]; then
  fail "A running systemd instance is required"
fi

echo "STEP 2/9 resolve archive and checksum"
WORK_PATH="$(mktemp -d /tmp/xray-panel-install.XXXXXX)"
if [[ -z "${ARCHIVE_INPUT}" ]]; then
  ARCHIVE_PATH="${WORK_PATH}/${ARCHIVE_NAME}"
  CHECKSUM_PATH="${WORK_PATH}/${ARCHIVE_NAME}.sha256"
  echo "Downloading ${ARCHIVE_URL}"
  download_https "${ARCHIVE_URL}" "${ARCHIVE_PATH}"
  echo "Downloading ${CHECKSUM_URL}"
  download_https "${CHECKSUM_URL}" "${CHECKSUM_PATH}"
else
  if [[ ! -f "${ARCHIVE_INPUT}" ]]; then
    fail "Archive not found: ${ARCHIVE_INPUT}"
  fi
  ARCHIVE_PATH="$(cd -- "$(dirname -- "${ARCHIVE_INPUT}")" && pwd -P)/$(basename -- "${ARCHIVE_INPUT}")"
  CHECKSUM_PATH="${XUI_ARCHIVE_CHECKSUM_FILE:-${ARCHIVE_PATH}.sha256}"
  if [[ ! -f "${CHECKSUM_PATH}" ]]; then
    fail "Checksum file not found: ${CHECKSUM_PATH}"
  fi
fi
archive_size="$(stat -c '%s' "${ARCHIVE_PATH}")"
if (( archive_size <= 0 || archive_size > 1073741824 )); then
  fail "Archive size is outside the allowed range: ${archive_size} bytes"
fi

echo "STEP 3/9 verify outer SHA-256"
expected_hash="$(awk 'NR == 1 {print tolower($1)}' "${CHECKSUM_PATH}")"
if [[ ! "${expected_hash}" =~ ^[0-9a-f]{64}$ ]]; then
  fail "Invalid SHA-256 file: ${CHECKSUM_PATH}"
fi
actual_hash="$(sha256sum "${ARCHIVE_PATH}" | awk '{print tolower($1)}')"
if [[ "${actual_hash}" != "${expected_hash}" ]]; then
  fail "Archive SHA-256 mismatch"
fi
echo "SHA256=${actual_hash}"

echo "STEP 4/9 inspect archive safety"
timeout 120 tar -tzf "${ARCHIVE_PATH}" > "${WORK_PATH}/entries.txt"
timeout 120 tar -tvzf "${ARCHIVE_PATH}" > "${WORK_PATH}/entries-verbose.txt"
[[ -s "${WORK_PATH}/entries.txt" ]]
while IFS= read -r entry; do
  [[ -n "${entry}" ]]
  [[ "${entry}" != /* ]]
  [[ "/${entry}/" != *"/../"* ]]
  [[ "${entry}" != *\\* ]]
done < "${WORK_PATH}/entries.txt"
if grep -Eq '^[^d-]' "${WORK_PATH}/entries-verbose.txt"; then
  fail "Only regular files and directories are allowed in the archive"
fi
available_kib="$(df -Pk "${WORK_PATH}" | awk 'NR == 2 {print $4}')"
if (( available_kib < 1048576 )); then
  fail "At least 1 GiB of free temporary space is required"
fi

echo "STEP 5/9 extract into private staging"
mkdir -p -- "${WORK_PATH}/extract"
timeout 180 tar --no-same-owner --no-same-permissions -xzf "${ARCHIVE_PATH}" -C "${WORK_PATH}/extract"
mapfile -t top_directories < <(find "${WORK_PATH}/extract" -mindepth 1 -maxdepth 1 -type d -print)
if [[ "${#top_directories[@]}" -ne 1 ]]; then
  fail "Archive must contain exactly one top-level directory"
fi
PACKAGE_PATH="${top_directories[0]}"
[[ -f "${PACKAGE_PATH}/SHA256SUMS" ]]
[[ -x "${PACKAGE_PATH}/scripts/install.sh" ]]
[[ -x "${PACKAGE_PATH}/bin/x-ui" ]]
[[ -x "${PACKAGE_PATH}/bin/xray-linux-amd64" ]]

echo "STEP 6/9 verify internal manifest"
(
  cd -- "${PACKAGE_PATH}"
  timeout 300 sha256sum -c SHA256SUMS
)
timeout 15 "${PACKAGE_PATH}/bin/x-ui" -v
timeout 15 "${PACKAGE_PATH}/bin/xray-linux-amd64" version
bash -n "${PACKAGE_PATH}"/scripts/*.sh

echo "STEP 7/9 installation gate"
if [[ "${CHECK_ONLY}" -eq 1 ]]; then
  echo "CHECK-ONLY: package is valid; no server files were changed"
else
  if [[ "${ASSUME_YES}" -ne 1 ]]; then
    read -r -p "Install or update x-ui on this server? Type INSTALL to continue: " confirmation
    [[ "${confirmation}" == "INSTALL" ]] || {
      echo "Installation cancelled"
      STATUS="DONE"
      write_report
      exit 0
    }
  fi
  echo "STEP 8/9 install with backup and rollback"
  timeout --preserve-status "${INSTALL_TIMEOUT}" "${PACKAGE_PATH}/scripts/install.sh"
fi

if [[ "${CHECK_ONLY}" -eq 1 ]]; then
  echo "STEP 8/9 mutation skipped"
fi

echo "STEP 9/9 final selfcheck"
if [[ "${CHECK_ONLY}" -eq 0 ]]; then
  timeout 30 bash -c 'until systemctl is-active --quiet x-ui.service; do sleep 1; done'
  timeout 15 "${XUI_INSTALL_DIR:-/usr/local/x-ui}/x-ui" -v
  timeout 15 "${XUI_INSTALL_DIR:-/usr/local/x-ui}/bin/xray-linux-amd64" version
fi
STATUS="DONE"
write_report
cleanup
WORK_PATH=""
trap - EXIT
echo "DONE"
echo "LOG=${LOG_PATH}"
echo "REPORT=${REPORT_PATH}"

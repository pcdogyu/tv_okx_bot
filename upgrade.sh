#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${TV_OKX_WORKDIR:-${APP_DIR:-/opt/tv_okx_bot}}"
BINARY_PATH="${TV_OKX_BINARY:-${BINARY_PATH:-${APP_DIR}/tv-okx-bot}}"
SERVICE_NAME="${TV_OKX_SERVICE:-${SERVICE_NAME:-tv-okx-bot.service}}"
APP_USER="${APP_USER:-tvokx}"
GO_CMD="${GO_CMD:-go}"
RESTART_CMD="${TV_OKX_RESTART_CMD:-${RESTART_CMD:-}}"
RUN_AS_APP_USER="${RUN_AS_APP_USER:-true}"
TMP_BINARY="${BINARY_PATH}.new"

shell_quote() {
  printf "%q" "$1"
}

log_step() {
  printf "\n==> %s\n" "$1"
}

run_in_app_dir() {
  local command="$1"
  if [[ "$(id -u)" -eq 0 && "${RUN_AS_APP_USER}" != "false" ]] && id "${APP_USER}" >/dev/null 2>&1; then
    sudo --preserve-env=PATH,GOPROXY,GOSUMDB,GONOSUMDB,GONOPROXY,GOPRIVATE -H -u "${APP_USER}" \
      bash -lc "cd $(shell_quote "${APP_DIR}") && ${command}"
    return
  fi
  bash -lc "cd $(shell_quote "${APP_DIR}") && ${command}"
}

cleanup() {
  run_in_app_dir "rm -f $(shell_quote "${TMP_BINARY}")" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -d "${APP_DIR}/.git" ]]; then
  echo "APP_DIR is not a git checkout: ${APP_DIR}" >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git is required on PATH" >&2
  exit 1
fi

if ! command -v "${GO_CMD}" >/dev/null 2>&1; then
  echo "go is required on PATH" >&2
  exit 1
fi

log_step "pull code"
run_in_app_dir "git pull --ff-only"

log_step "run tests"
run_in_app_dir "$(shell_quote "${GO_CMD}") test ./..."

log_step "build binary"
run_in_app_dir "$(shell_quote "${GO_CMD}") build -o $(shell_quote "${TMP_BINARY}") ./cmd/tv-okx-bot"

log_step "replace binary"
run_in_app_dir "mv -f $(shell_quote "${TMP_BINARY}") $(shell_quote "${BINARY_PATH}")"

log_step "restart service"
if [[ -n "${RESTART_CMD}" ]]; then
  bash -lc "${RESTART_CMD}"
elif [[ "$(id -u)" -eq 0 ]]; then
  systemctl restart "${SERVICE_NAME}"
else
  sudo /bin/systemctl restart "${SERVICE_NAME}"
fi

sleep 2
systemctl is-active "${SERVICE_NAME}"

log_step "deployed commit"
run_in_app_dir "git rev-parse --short HEAD"

#!/usr/bin/env bash
set -euo pipefail

APP_USER="${APP_USER:-tvokx}"
APP_GROUP="${APP_GROUP:-tvokx}"
APP_DIR="${APP_DIR:-/opt/tv_okx_bot}"
CONFIG_DIR="${CONFIG_DIR:-/etc/tv-okx-bot}"
STATE_DIR="${STATE_DIR:-/var/lib/tv-okx-bot}"
SERVICE_NAME="${SERVICE_NAME:-tv-okx-bot.service}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/ubuntu/install-service.sh" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go is required on PATH" >&2
  exit 1
fi

if ! command -v setcap >/dev/null 2>&1; then
  echo "setcap is required. Install libcap2-bin first." >&2
  exit 1
fi

if ! getent group "${APP_GROUP}" >/dev/null 2>&1; then
  groupadd --system "${APP_GROUP}"
fi

if ! id "${APP_USER}" >/dev/null 2>&1; then
  useradd --system --gid "${APP_GROUP}" --create-home --home-dir "${APP_DIR}" --shell /usr/sbin/nologin "${APP_USER}"
fi

mkdir -p "${APP_DIR}" "${CONFIG_DIR}" "${STATE_DIR}"
rsync -a --delete \
  --exclude 'data' \
  --exclude 'tmp' \
  --exclude 'dist' \
  ./ "${APP_DIR}/"

if [[ ! -f "${CONFIG_DIR}/config.json" ]]; then
  cp "${APP_DIR}/config.example.json" "${CONFIG_DIR}/config.json"
fi

if [[ ! -f "${CONFIG_DIR}/tv-okx-bot.env" ]]; then
  cp "${APP_DIR}/deploy/ubuntu/tv-okx-bot.env.example" "${CONFIG_DIR}/tv-okx-bot.env"
  chmod 600 "${CONFIG_DIR}/tv-okx-bot.env"
fi

chown -R "${APP_USER}:${APP_GROUP}" "${APP_DIR}" "${CONFIG_DIR}" "${STATE_DIR}"

sudo --preserve-env=PATH -u "${APP_USER}" bash -lc "cd '${APP_DIR}' && go test ./... && go build -o '${APP_DIR}/tv-okx-bot' ./cmd/tv-okx-bot"

install -m 0644 "${APP_DIR}/deploy/ubuntu/tv-okx-bot.service" "/etc/systemd/system/${SERVICE_NAME}"
cat > "/etc/sudoers.d/tv-okx-bot-upgrade" <<EOF
${APP_USER} ALL=(root) NOPASSWD: /bin/systemctl restart ${SERVICE_NAME}
EOF
chmod 0440 "/etc/sudoers.d/tv-okx-bot-upgrade"

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl restart "${SERVICE_NAME}"
systemctl --no-pager --full status "${SERVICE_NAME}"

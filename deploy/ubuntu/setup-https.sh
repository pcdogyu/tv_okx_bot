#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${DOMAIN:-tvbot.lmitis.com}"
EMAIL="${EMAIL:-Yuhao@jiansutech.com}"
APP_DIR="${APP_DIR:-/opt/tv_okx_bot}"
NGINX_SITE="/etc/nginx/sites-available/tv-okx-bot.conf"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/ubuntu/setup-https.sh" >&2
  exit 1
fi

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y nginx certbot python3-certbot-nginx

sed "s/server_name tvbot\\.lmitis\\.com;/server_name ${DOMAIN};/" \
  "${APP_DIR}/deploy/ubuntu/nginx-tv-okx-bot.conf" > "${NGINX_SITE}"
ln -sfn "${NGINX_SITE}" /etc/nginx/sites-enabled/tv-okx-bot.conf
rm -f /etc/nginx/sites-enabled/default

nginx -t
systemctl enable nginx
systemctl restart nginx

certbot --nginx \
  --non-interactive \
  --agree-tos \
  --email "${EMAIL}" \
  --redirect \
  -d "${DOMAIN}"

systemctl reload nginx
certbot renew --dry-run


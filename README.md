# TradingView OKX Bot

Go service that receives TradingView webhook JSON at `/tvorder`, validates a Base64-wrapped HMAC token, and places OKX USDT perpetual swap orders. TradingView supplies `action` and `coinpair`; the token is not bound to either field.

## Environment

Required for template generation and webhook validation:

```powershell
$env:TV_TOKEN_SECRET = "replace-with-a-long-random-secret"
```

Required for `/tvbot` management APIs:

```powershell
$env:ADMIN_USER = "admin"
$env:ADMIN_PASSWORD = "Admin123"
$env:ADMIN_TOKEN = "optional-api-token"
```

Required for OKX trading or checks:

```powershell
$env:OKX_API_KEY = "..."
$env:OKX_SECRET_KEY = "..."
$env:OKX_PASSPHRASE = "..."
```

OKX API credentials can also be configured from the `/tvbot/` browser dashboard. Multiple OKX APIs are supported; each account has an `api_id`, one account can be marked as the active trading API, and TradingView templates can include `api_id` to select a specific API for that order. Order amount, leverage, fixed TP/SL or trailing stop, and long/short limit price multipliers are configured on the server. Credentials are saved outside Git, masked in API responses, and take effect without restarting the service.

Demo trading is the default. Live trading requires both config `"env": "live"` and:

```powershell
$env:OKX_ENV = "live"
$env:ALLOW_LIVE_TRADING = "true"
```

## Commands

```powershell
go test ./...
go run ./cmd/tv-okx-bot template --price-source close
go run ./cmd/tv-okx-bot serve --config config.example.json
go run ./cmd/tv-okx-bot check-okx --config config.example.json
```

## Routes

- `GET /` returns `302` to `https://www.mext.go.jp/`.
- `POST /tvorder` accepts TradingView alerts.
- `/tvbot/` is the browser dashboard. `/tvbot/config`, `/tvbot/api-keys`, `/tvbot/api-keys/test`, `/tvbot/templates`, `/tvbot/orders`, and `/tvbot/check-okx` remain JSON APIs. Admin access accepts browser Basic Auth. Default credentials are `admin` / `Admin123`. `X-Admin-Token` is still supported when `ADMIN_TOKEN` is set.
- `TradingView` alert JSON can include optional `api_id`; when omitted the active OKX API is used. New templates do not include order amount or leverage because those values are read from `/tvbot` order settings.
- Orders are submitted as OKX limit orders. By default long orders use `TradingView price * 0.997`, and short orders use `TradingView price * 1.003`.
- `POST /upgrade` runs `git pull --ff-only`, `go test ./...`, `go build`, replaces the service binary, and restarts the Ubuntu systemd service.
- `GET /upgrade` returns the latest upgrade status.
- Every other path returns local JSON `404`.

## Ubuntu Service

On Ubuntu, clone the repo and run:

```bash
sudo bash deploy/ubuntu/install-service.sh
```

Then edit `/etc/tv-okx-bot/tv-okx-bot.env` and `/etc/tv-okx-bot/config.json`, then restart:

```bash
sudo systemctl restart tv-okx-bot.service
sudo systemctl status tv-okx-bot.service
```

The Go service listens on `127.0.0.1:18080` by default. Nginx listens on public `80/443` and proxies to the Go service. To install Nginx and a Let's Encrypt certificate for `tvbot.lmitis.com`, run:

```bash
sudo bash deploy/ubuntu/setup-https.sh
```

The `/upgrade` endpoint writes status to `/var/lib/tv-okx-bot/upgrade-status.json`, then schedules `/usr/bin/sudo /bin/systemctl restart tv-okx-bot.service`; the installer adds a narrow sudoers rule for that restart command.

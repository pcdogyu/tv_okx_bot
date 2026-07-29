# TradingView OKX/Binance Bot

Go service that receives TradingView webhook JSON at `/tvorder`, validates a Base64-wrapped HMAC token, and places OKX USDT perpetual swap or Binance USD-M Futures orders. TradingView supplies `action` and `coinpair`; the token is not bound to either field.

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

Required for Binance USD-M Futures trading or checks:

```powershell
$env:BINANCE_API_KEY = "..."
$env:BINANCE_SECRET_KEY = "..."
$env:BINANCE_CREDENTIALS_FILE = "data/binance-credentials.json"
```

OKX and Binance API credentials can also be configured from the `/tvbot/` browser dashboard. Multiple APIs are supported per exchange; each account has an `api_id`, one account can be marked as the active trading API, and TradingView templates can include `api_id` and `target_exchange` to select the destination for that order. Order amount, leverage, order type (market by default or limit), fixed TP/SL or trailing stop, and long/short limit price multipliers are configured on the server. Credentials are saved outside Git, masked in API responses, and take effect without restarting the service.

Demo trading is the default. Live trading requires both config `"env": "live"` and:

```powershell
$env:OKX_ENV = "live"
$env:BINANCE_ENV = "live"
$env:ALLOW_LIVE_TRADING = "true"
```

## Commands

```powershell
go test ./...
go run ./cmd/tv-okx-bot template --price-source close
go run ./cmd/tv-okx-bot serve --config config.example.json
go run ./cmd/tv-okx-bot check-okx --config config.example.json
```

## AI Agent Workflow

Future AI agents should read `AGENTS.md` before making changes. It records the default delivery flow: verify, commit task-related files, push the current branch to `origin`, trigger `POST https://tvbot.lmitis.com/upgrade`, then check upgrade status.

## Routes

- `GET /` returns `302` to `https://www.mext.go.jp/`.
- `POST /tvorder` accepts TradingView alerts.
- `/tvbot/` is the browser dashboard. `/tvbot/config`, `/tvbot/api-keys`, `/tvbot/api-keys/test`, `/tvbot/templates`, `/tvbot/orders`, `/tvbot/trade-monitor`, and `/tvbot/check-okx` remain JSON APIs. Admin access accepts browser Basic Auth. Default credentials are `admin` / `Admin123`. `X-Admin-Token` is still supported when `ADMIN_TOKEN` is set.
- `TradingView` alert JSON can include optional `api_id` and `target_exchange`; when omitted the active OKX API is used for backward compatibility. The existing `exchange` field remains the TradingView signal source.
- OKX orders keep the existing behavior. Binance USD-M Futures supports market and limit entries; `tp_sl` places protective Binance algo orders after the main order, while Binance trailing stop is explicitly rejected in this version.
- Binance USD-M fills are monitored server-side through REST polling when `trading.fill_monitor.enabled` is true. The monitor persists fills, checkpoints, lifecycle state, and events in SQLite; `trading.auto_reentry.enabled` defaults to false.
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

For manual upgrades on the Ubuntu server, run:

```bash
sudo bash upgrade.sh
```

The script uses `/opt/tv_okx_bot`, `/opt/tv_okx_bot/tv-okx-bot`, and `tv-okx-bot.service` by default. Override them with `TV_OKX_WORKDIR`, `TV_OKX_BINARY`, and `TV_OKX_SERVICE` when needed.

The Go service listens on `127.0.0.1:18080` by default. Nginx listens on public `80/443` and proxies to the Go service. To install Nginx and a Let's Encrypt certificate for `tvbot.lmitis.com`, run:

```bash
sudo bash deploy/ubuntu/setup-https.sh
```

The `/upgrade` endpoint writes status to `/var/lib/tv-okx-bot/upgrade-status.json`, then schedules `/usr/bin/sudo /bin/systemctl restart tv-okx-bot.service`; the installer adds a narrow sudoers rule for that restart command.

# TradingView OKX Bot

Go service that receives TradingView webhook JSON at `/tvorder`, validates a 64-character HMAC token, and places OKX USDT perpetual swap orders.

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

Demo trading is the default. Live trading requires both config `"env": "live"` and:

```powershell
$env:OKX_ENV = "live"
$env:ALLOW_LIVE_TRADING = "true"
```

## Commands

```powershell
go test ./...
go run ./cmd/tv-okx-bot template --action long --coinpair BTC --leverage 5 --amount 100 --tp-pct 2 --sl-pct 1
go run ./cmd/tv-okx-bot serve --config config.example.json
go run ./cmd/tv-okx-bot check-okx --config config.example.json
```

## Routes

- `GET /` returns `302` to `https://www.mext.go.jp/`.
- `POST /tvorder` accepts TradingView alerts.
- `/tvbot/*` is a JSON management API and accepts browser Basic Auth. Default credentials are `admin` / `Admin123`. `X-Admin-Token` is still supported when `ADMIN_TOKEN` is set.
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

The service listens on `0.0.0.0:18080` by default. The `/upgrade` endpoint uses `/usr/bin/sudo /bin/systemctl restart tv-okx-bot.service`; the installer adds a narrow sudoers rule for that restart command.

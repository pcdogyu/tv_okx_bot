package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/env"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/security"
	"github.com/pcdogyu/tv_okx_bot/internal/server"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	"github.com/pcdogyu/tv_okx_bot/internal/upgrade"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "template":
		return runTemplate(args[1:])
	case "check-okx":
		return runCheckOKX(args[1:])
	case "help", "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%w", args[0], usage())
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "config file path")
	addr := fs.String("addr", "", "override listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Server.Addr = *addr
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	secrets := env.Load()
	if err := secrets.RequireTVTokenSecret(); err != nil {
		return err
	}
	if err := secrets.RequireAdminToken(); err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := secrets.RequireOKXCredentials(); err != nil {
		logger.Warn("OKX credentials are incomplete; /tvorder accepts requests but execution will fail until credentials are set", "error", err)
	}
	orderStore, err := storage.NewOrderStore(resolveDataPath(*configPath, cfg.DataFile))
	if err != nil {
		return err
	}
	upgradeRunner, err := upgrade.NewShellRunnerFromEnv()
	if err != nil {
		return err
	}
	upgradeStatusFile := os.Getenv("TV_OKX_UPGRADE_STATUS_FILE")
	if upgradeStatusFile == "" {
		upgradeStatusFile = filepath.Join(upgradeRunner.WorkDir, "data", "upgrade-status.json")
	}
	handler := &server.Server{
		ConfigStore: config.NewStore(*configPath, cfg),
		Orders:      orderStore,
		Token:       security.NewTokenService(secrets.TVTokenSecret),
		Executor: okx.Trader{
			Credentials: okx.Credentials{
				APIKey:     secrets.OKXAPIKey,
				SecretKey:  secrets.OKXSecretKey,
				Passphrase: secrets.OKXPassphrase,
			},
			HTTPClient: &http.Client{Timeout: 15 * time.Second},
			Logger:     logger,
		},
		AdminToken: secrets.AdminToken,
		AdminUser:  secrets.AdminUser,
		AdminPass:  secrets.AdminPassword,
		Logger:     logger,
		Upgrade:    upgrade.NewManager(upgradeRunner, upgrade.WithStatusFile(upgradeStatusFile)),
	}
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("tv okx bot listening", "addr", cfg.Server.Addr, "env", cfg.Trading.Env)
	return srv.ListenAndServe()
}

func runTemplate(args []string) error {
	fs := flag.NewFlagSet("template", flag.ContinueOnError)
	action := fs.String("action", "", "long or short")
	coinpair := fs.String("coinpair", "", "BTC or ETH")
	priceSource := fs.String("price-source", "close", "close, high or low")
	leverage := fs.Int("leverage", 0, "order leverage")
	amount := fs.Float64("amount", 0, "USDT notional amount")
	tpPct := fs.Float64("tp-pct", 0, "take profit percent")
	slPct := fs.Float64("sl-pct", 0, "stop loss percent")
	trailingPct := fs.Float64("trailing-pct", 0, "trailing stop percent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	secrets := env.Load()
	if err := secrets.RequireTVTokenSecret(); err != nil {
		return err
	}
	req := trading.TemplateRequest{
		Action:      trading.Side(*action),
		Coinpair:    *coinpair,
		PriceSource: *priceSource,
		Leverage:    *leverage,
		Amount:      trading.NewFlexibleFloat(*amount),
		Risk:        trading.Risk{Type: trading.RiskNone},
	}
	if *trailingPct > 0 {
		v := trading.NewFlexibleFloat(*trailingPct)
		req.Risk = trading.Risk{Type: trading.RiskTrailing, TrailingPct: &v}
	} else if *tpPct > 0 || *slPct > 0 {
		tp := trading.NewFlexibleFloat(*tpPct)
		sl := trading.NewFlexibleFloat(*slPct)
		req.Risk = trading.Risk{Type: trading.RiskTPSL, TPPct: &tp, SLPct: &sl}
	}
	resp, err := trading.BuildTemplate(req, security.NewTokenService(secrets.TVTokenSecret))
	if err != nil {
		return err
	}
	fmt.Println(resp.JSON)
	return nil
}

func runCheckOKX(args []string) error {
	fs := flag.NewFlagSet("check-okx", flag.ContinueOnError)
	configPath := fs.String("config", "config.json", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	secrets := env.Load()
	if err := secrets.RequireOKXCredentials(); err != nil {
		return err
	}
	trader := okx.Trader{
		Credentials: okx.Credentials{
			APIKey:     secrets.OKXAPIKey,
			SecretKey:  secrets.OKXSecretKey,
			Passphrase: secrets.OKXPassphrase,
		},
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := trader.Check(ctx, cfg)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func resolveDataPath(configPath, dataFile string) string {
	if filepath.IsAbs(dataFile) || configPath == "" {
		return dataFile
	}
	dir := filepath.Dir(configPath)
	if dir == "." || dir == "" {
		return dataFile
	}
	return filepath.Join(dir, dataFile)
}

func usage() error {
	return fmt.Errorf(`usage:
  tv-okx-bot serve --config config.json [--addr :8080]
  tv-okx-bot template --action long --coinpair BTC --leverage 5 --amount 100 [--tp-pct 2 --sl-pct 1]
  tv-okx-bot check-okx --config config.json`)
}

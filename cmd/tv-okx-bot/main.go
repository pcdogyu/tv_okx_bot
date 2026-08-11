package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/env"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/security"
	"github.com/pcdogyu/tv_okx_bot/internal/server"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	"github.com/pcdogyu/tv_okx_bot/internal/upgrade"
)

var (
	commitTime   string
	commitHash   string
	commitBranch string
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
	databaseFile := resolveDataPath(*configPath, cfg.DatabaseFile)
	legacyOrdersFile := resolveDataPath(*configPath, cfg.DataFile)
	orderStore, err := storage.NewSQLiteOrderStore(databaseFile, legacyOrdersFile)
	if err != nil {
		return err
	}
	upgradeRunner, err := upgrade.NewShellRunnerFromEnv()
	if err != nil {
		return err
	}
	credentialsFile := os.Getenv("OKX_CREDENTIALS_FILE")
	if credentialsFile == "" {
		credentialsFile = filepath.Join(upgradeRunner.WorkDir, "data", "okx-credentials.json")
	}
	credentialStore, err := okx.NewCredentialStore(credentialsFile, okx.Credentials{
		APIKey:     secrets.OKXAPIKey,
		SecretKey:  secrets.OKXSecretKey,
		Passphrase: secrets.OKXPassphrase,
	})
	if err != nil {
		return err
	}
	if !credentialStore.Status().Configured {
		logger.Warn("OKX credentials are incomplete; /tvorder accepts requests but execution will fail until credentials are set")
	}
	binanceCredentialsFile := os.Getenv("BINANCE_CREDENTIALS_FILE")
	if binanceCredentialsFile == "" {
		binanceCredentialsFile = filepath.Join(upgradeRunner.WorkDir, "data", "binance-credentials.json")
	}
	binanceCredentialStore, err := binance.NewCredentialStore(binanceCredentialsFile, binance.Credentials{
		APIKey:    secrets.BinanceAPIKey,
		SecretKey: secrets.BinanceSecretKey,
	})
	if err != nil {
		return err
	}
	if !binanceCredentialStore.Status().Configured {
		logger.Warn("Binance credentials are incomplete; target_exchange=binance execution will fail until credentials are set")
	}
	upgradeStatusFile := os.Getenv("TV_OKX_UPGRADE_STATUS_FILE")
	if upgradeStatusFile == "" {
		upgradeStatusFile = filepath.Join(upgradeRunner.WorkDir, "data", "upgrade-status.json")
	}
	buildInfo := resolveBuildInfo(upgradeRunner.WorkDir)
	handler := &server.Server{
		ConfigStore: config.NewStore(*configPath, cfg),
		Orders:      orderStore,
		Token:       security.NewTokenService(secrets.TVTokenSecret),
		Executor: server.ExchangeExecutor{
			OKX: okx.Trader{
				CredentialProvider: credentialStore,
				HTTPClient:         &http.Client{Timeout: 15 * time.Second},
				Logger:             logger,
			},
			Binance: binance.Trader{
				CredentialProvider: binanceCredentialStore,
				HTTPClient:         &http.Client{Timeout: 15 * time.Second},
				Logger:             logger,
			},
		},
		OKXCredentials:     credentialStore,
		BinanceCredentials: binanceCredentialStore,
		OKXHTTPClient:      &http.Client{Timeout: 15 * time.Second},
		BinanceHTTPClient:  &http.Client{Timeout: 15 * time.Second},
		AdminToken:         secrets.AdminToken,
		AdminUser:          secrets.AdminUser,
		AdminPass:          secrets.AdminPassword,
		Logger:             logger,
		Upgrade:            upgrade.NewManager(upgradeRunner, upgrade.WithStatusFile(upgradeStatusFile)),
		BuildInfo:          buildInfo,
	}
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	handler.StartUSDTBalanceSampler(context.Background())
	handler.StartLowMarginPositionMonitor(context.Background())
	handler.StartAutoProfitPositionCloseMonitor(context.Background())
	handler.StartTradeFillMonitor(context.Background())
	handler.StartSymbolCatalogSyncer(context.Background())
	logger.Info("tv okx bot listening", "addr", cfg.Server.Addr, "env", cfg.Trading.Env, "commit", buildInfo.CommitHash, "branch", buildInfo.CommitBranch)
	return srv.ListenAndServe()
}

func resolveBuildInfo(workDir string) server.BuildInfo {
	info := server.BuildInfo{
		CommitTime:   strings.TrimSpace(commitTime),
		CommitHash:   strings.TrimSpace(commitHash),
		CommitBranch: strings.TrimSpace(commitBranch),
	}
	if info.CommitTime == "" {
		info.CommitTime = gitOutput(workDir, "log", "-1", "--format=%cI")
	}
	if info.CommitHash == "" {
		info.CommitHash = gitOutput(workDir, "rev-parse", "--short", "HEAD")
	}
	if info.CommitBranch == "" {
		info.CommitBranch = gitOutput(workDir, "rev-parse", "--abbrev-ref", "HEAD")
	}
	return info
}

func gitOutput(workDir string, args ...string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmdArgs := append([]string{"-C", workDir}, args...)
	out, err := exec.CommandContext(ctx, "git", cmdArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runTemplate(args []string) error {
	fs := flag.NewFlagSet("template", flag.ContinueOnError)
	priceSource := fs.String("price-source", "close", "close, high or low")
	tradeEnv := fs.String("trade-env", trading.TradeEnvDemo, "demo or live")
	leverage := fs.Int("leverage", 0, "deprecated; order leverage is configured on the server")
	amount := fs.Float64("amount", 0, "deprecated; USDT notional amount is configured on the server")
	if err := fs.Parse(args); err != nil {
		return err
	}
	secrets := env.Load()
	if err := secrets.RequireTVTokenSecret(); err != nil {
		return err
	}
	req := trading.TemplateRequest{
		PriceSource: *priceSource,
		TradeEnv:    *tradeEnv,
		Leverage:    *leverage,
		Amount:      trading.NewFlexibleFloat(*amount),
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
  tv-okx-bot template [--price-source close]
  tv-okx-bot check-okx --config config.json`)
}

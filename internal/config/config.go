package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	EnvDemo = "demo"
	EnvLive = "live"

	MarginIsolated = "isolated"
	MarginCross    = "cross"

	PositionNet       = "net"
	PositionLongShort = "long_short"
)

type Config struct {
	Server   ServerConfig            `json:"server"`
	DataFile string                  `json:"data_file"`
	Trading  TradingConfig           `json:"trading"`
	Symbols  map[string]SymbolConfig `json:"symbols"`
}

type ServerConfig struct {
	Addr string `json:"addr"`
}

type TradingConfig struct {
	Env               string `json:"env"`
	AllowLiveTrading  bool   `json:"allow_live_trading"`
	BaseURL           string `json:"base_url"`
	DefaultMarginMode string `json:"default_margin_mode"`
	PositionMode      string `json:"position_mode"`
	SignalTTLSeconds  int    `json:"signal_ttl_seconds"`
}

type SymbolConfig struct {
	Coinpair string  `json:"coinpair"`
	InstID   string  `json:"inst_id"`
	CtVal    float64 `json:"ct_val"`
	LotSz    float64 `json:"lot_sz"`
	MinSz    float64 `json:"min_sz"`
}

func Default() Config {
	return Config{
		Server:   ServerConfig{Addr: ":8080"},
		DataFile: "data/orders.json",
		Trading: TradingConfig{
			Env:               EnvDemo,
			AllowLiveTrading:  false,
			BaseURL:           "https://www.okx.com",
			DefaultMarginMode: MarginIsolated,
			PositionMode:      PositionNet,
			SignalTTLSeconds:  120,
		},
		Symbols: map[string]SymbolConfig{
			"BTC": {
				Coinpair: "BTC",
				InstID:   "BTC-USDT-SWAP",
				CtVal:    0.01,
				LotSz:    0.01,
				MinSz:    0.01,
			},
			"ETH": {
				Coinpair: "ETH",
				InstID:   "ETH-USDT-SWAP",
				CtVal:    0.1,
				LotSz:    0.01,
				MinSz:    0.01,
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		cfg.Normalize()
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.Normalize()
			return cfg, nil
		}
		return Config{}, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("config path is required")
	}
	cfg.Normalize()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) Normalize() {
	if strings.TrimSpace(c.Server.Addr) == "" {
		c.Server.Addr = ":8080"
	}
	if strings.TrimSpace(c.DataFile) == "" {
		c.DataFile = "data/orders.json"
	}
	c.Trading.Env = strings.ToLower(strings.TrimSpace(c.Trading.Env))
	if c.Trading.Env == "" {
		c.Trading.Env = EnvDemo
	}
	c.Trading.BaseURL = strings.TrimRight(strings.TrimSpace(c.Trading.BaseURL), "/")
	if c.Trading.BaseURL == "" {
		c.Trading.BaseURL = "https://www.okx.com"
	}
	c.Trading.DefaultMarginMode = strings.ToLower(strings.TrimSpace(c.Trading.DefaultMarginMode))
	if c.Trading.DefaultMarginMode == "" {
		c.Trading.DefaultMarginMode = MarginIsolated
	}
	c.Trading.PositionMode = strings.ToLower(strings.TrimSpace(c.Trading.PositionMode))
	if c.Trading.PositionMode == "" {
		c.Trading.PositionMode = PositionNet
	}
	if c.Trading.SignalTTLSeconds <= 0 {
		c.Trading.SignalTTLSeconds = 120
	}
	if c.Symbols == nil {
		c.Symbols = map[string]SymbolConfig{}
	}
	normalized := make(map[string]SymbolConfig, len(c.Symbols))
	for key, sym := range c.Symbols {
		coin := strings.ToUpper(strings.TrimSpace(sym.Coinpair))
		if coin == "" {
			coin = strings.ToUpper(strings.TrimSpace(key))
		}
		sym.Coinpair = coin
		sym.InstID = strings.ToUpper(strings.TrimSpace(sym.InstID))
		normalized[coin] = sym
	}
	c.Symbols = normalized
}

func (c Config) Validate() error {
	switch c.Trading.Env {
	case EnvDemo, EnvLive:
	default:
		return fmt.Errorf("unsupported trading env %q", c.Trading.Env)
	}
	switch c.Trading.DefaultMarginMode {
	case MarginIsolated, MarginCross:
	default:
		return fmt.Errorf("unsupported margin mode %q", c.Trading.DefaultMarginMode)
	}
	switch c.Trading.PositionMode {
	case PositionNet, PositionLongShort:
	default:
		return fmt.Errorf("unsupported position mode %q", c.Trading.PositionMode)
	}
	if len(c.Symbols) == 0 {
		return errors.New("at least one symbol is required")
	}
	for key, sym := range c.Symbols {
		if sym.Coinpair == "" || sym.InstID == "" {
			return fmt.Errorf("symbol %q requires coinpair and inst_id", key)
		}
		if sym.CtVal <= 0 || sym.LotSz <= 0 || sym.MinSz <= 0 {
			return fmt.Errorf("symbol %q requires positive ct_val, lot_sz and min_sz", key)
		}
	}
	return nil
}

func (c Config) Symbol(coinpair string) (SymbolConfig, bool) {
	sym, ok := c.Symbols[strings.ToUpper(strings.TrimSpace(coinpair))]
	return sym, ok
}

func (c Config) SymbolMeta(coinpair string) (trading.SymbolInfo, bool) {
	sym, ok := c.Symbol(coinpair)
	if !ok {
		return trading.SymbolInfo{}, false
	}
	return trading.SymbolInfo{
		Coinpair: sym.Coinpair,
		InstID:   sym.InstID,
		CtVal:    sym.CtVal,
		LotSz:    sym.LotSz,
		MinSz:    sym.MinSz,
	}, true
}

func (c Config) DemoTradingHeaderEnabled() bool {
	return c.Trading.Env != EnvLive
}

func (c Config) OKXBaseURL() string {
	return c.Trading.BaseURL
}

func (c Config) MarginMode() string {
	return c.Trading.DefaultMarginMode
}

func (c Config) PositionMode() string {
	return c.Trading.PositionMode
}

func (c Config) LiveTradingAllowedByEnvironment() bool {
	if c.Trading.Env != EnvLive {
		return true
	}
	return strings.EqualFold(os.Getenv("OKX_ENV"), EnvLive) &&
		strings.EqualFold(os.Getenv("ALLOW_LIVE_TRADING"), "true") &&
		c.Trading.AllowLiveTrading
}

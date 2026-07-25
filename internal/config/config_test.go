package config

import "testing"

func TestNormalizeDefaultTab(t *testing.T) {
	cfg := Default()
	cfg.UI.DefaultTab = ""
	cfg.Normalize()
	if cfg.UI.DefaultTab != DefaultHomeTab {
		t.Fatalf("blank default tab should fall back to %q, got %q", DefaultHomeTab, cfg.UI.DefaultTab)
	}

	cfg.UI.DefaultTab = "positions"
	cfg.Normalize()
	if cfg.UI.DefaultTab != "positions" {
		t.Fatalf("known default tab should be preserved, got %q", cfg.UI.DefaultTab)
	}

	cfg.UI.DefaultTab = "missing-tab"
	cfg.Normalize()
	if cfg.UI.DefaultTab != DefaultHomeTab {
		t.Fatalf("unknown default tab should fall back to %q, got %q", DefaultHomeTab, cfg.UI.DefaultTab)
	}
}

func TestNormalizeBinanceBaseURLs(t *testing.T) {
	cfg := Default()
	cfg.Trading.BinanceBaseURL = "https://fapi.binance.com/"
	cfg.Trading.BinanceDemoBaseURL = ""
	cfg.Normalize()
	if cfg.Trading.BinanceBaseURL != "https://fapi.binance.com" {
		t.Fatalf("Binance base URL should be trimmed, got %q", cfg.Trading.BinanceBaseURL)
	}
	if cfg.Trading.BinanceDemoBaseURL != "https://demo-fapi.binance.com" {
		t.Fatalf("Binance demo base URL should default, got %q", cfg.Trading.BinanceDemoBaseURL)
	}
}

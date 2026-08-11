package config

import (
	"reflect"
	"testing"
)

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

func TestNormalizeTableColumns(t *testing.T) {
	cfg := Default()
	cfg.UI.TableColumns = TableColumnsConfig{}
	cfg.Normalize()
	if !reflect.DeepEqual(cfg.UI.TableColumns.Positions, DefaultPositionTableColumns) {
		t.Fatalf("blank position columns should use defaults: %#v", cfg.UI.TableColumns.Positions)
	}
	if !reflect.DeepEqual(cfg.UI.TableColumns.PendingOrders, DefaultPendingOrderTableColumns) {
		t.Fatalf("blank pending order columns should use defaults: %#v", cfg.UI.TableColumns.PendingOrders)
	}
	if !reflect.DeepEqual(cfg.UI.TableColumns.Symbols, DefaultSymbolTableColumns) {
		t.Fatalf("blank symbol columns should use defaults: %#v", cfg.UI.TableColumns.Symbols)
	}

	cfg.UI.TableColumns = TableColumnsConfig{
		Positions:     []string{"upl", "unknown", "exchange", "upl"},
		PendingOrders: []string{"actions", "symbol", "bad", "actions"},
		Symbols:       []string{"turnover", "symbol", "bad", "turnover"},
	}
	cfg.Normalize()
	if len(cfg.UI.TableColumns.Positions) != len(DefaultPositionTableColumns) ||
		cfg.UI.TableColumns.Positions[0] != "upl" ||
		cfg.UI.TableColumns.Positions[1] != "exchange" {
		t.Fatalf("position columns should keep known unique order then append defaults: %#v", cfg.UI.TableColumns.Positions)
	}
	if len(cfg.UI.TableColumns.PendingOrders) != len(DefaultPendingOrderTableColumns) ||
		cfg.UI.TableColumns.PendingOrders[0] != "actions" ||
		cfg.UI.TableColumns.PendingOrders[1] != "symbol" {
		t.Fatalf("pending order columns should keep known unique order then append defaults: %#v", cfg.UI.TableColumns.PendingOrders)
	}
	if len(cfg.UI.TableColumns.Symbols) != len(DefaultSymbolTableColumns) ||
		cfg.UI.TableColumns.Symbols[0] != "turnover" ||
		cfg.UI.TableColumns.Symbols[1] != "symbol" {
		t.Fatalf("symbol columns should keep known unique order then append defaults: %#v", cfg.UI.TableColumns.Symbols)
	}
	if containsString(cfg.UI.TableColumns.Positions, "unknown") || containsString(cfg.UI.TableColumns.PendingOrders, "bad") || containsString(cfg.UI.TableColumns.Symbols, "bad") {
		t.Fatalf("unknown columns should be removed: %#v", cfg.UI.TableColumns)
	}
	if countString(cfg.UI.TableColumns.Positions, "upl") != 1 || countString(cfg.UI.TableColumns.PendingOrders, "actions") != 1 || countString(cfg.UI.TableColumns.Symbols, "turnover") != 1 {
		t.Fatalf("duplicate columns should be removed: %#v", cfg.UI.TableColumns)
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

func TestNormalizePositionMonitorDefaultsAndValidation(t *testing.T) {
	cfg := Default()
	cfg.Trading.PositionMonitor = PositionMonitorConfig{}
	cfg.Normalize()
	if cfg.Trading.PositionMonitor.OKXEnabled || cfg.Trading.PositionMonitor.BinanceEnabled {
		t.Fatalf("position monitor should default disabled: %#v", cfg.Trading.PositionMonitor)
	}
	if cfg.Trading.PositionMonitor.PollIntervalSeconds != 300 || cfg.Trading.PositionMonitor.TakeProfitPct != 5 || cfg.Trading.PositionMonitor.StopLossPct != 8 {
		t.Fatalf("bad position monitor defaults: %#v", cfg.Trading.PositionMonitor)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaulted position monitor should validate: %v", err)
	}

	cfg.Trading.PositionMonitor.PollIntervalSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("zero position monitor interval should be rejected")
	}
	cfg.Trading.PositionMonitor.PollIntervalSeconds = 300
	cfg.Trading.PositionMonitor.TakeProfitPct = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative position monitor take profit should be rejected")
	}
	cfg.Trading.PositionMonitor.TakeProfitPct = 5
	cfg.Trading.PositionMonitor.StopLossPct = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative position monitor stop loss should be rejected")
	}
}

func containsString(values []string, want string) bool {
	return countString(values, want) > 0
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

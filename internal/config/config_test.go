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

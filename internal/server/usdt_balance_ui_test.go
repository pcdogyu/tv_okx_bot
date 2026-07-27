package server

import (
	"strings"
	"testing"
)

func TestTVBotUSDTBalanceLayoutAndWindowButtons(t *testing.T) {
	if got := strings.Count(tvbotHTML, `class="analysis-metrics symbol-metrics exchange-balance-metrics"`); got != 2 {
		t.Fatalf("exchange balance metric blocks=%d, want 2", got)
	}
	if got := strings.Count(tvbotHTML, `class="balance-table-wrap"`); got != 2 {
		t.Fatalf("balance table wrappers=%d, want 2", got)
	}
	for _, marker := range []string{
		`.exchange-balance-metrics`,
		`.balance-table-wrap`,
		`.balance-window-toolbar .balance-window-btn`,
		`font-size: 16px`,
		`data-balance-minutes="15"`,
		`data-balance-minutes="60"`,
		`data-balance-minutes="240"`,
		`data-balance-minutes="480"`,
		`data-balance-minutes="720"`,
		`data-balance-minutes="1440"`,
		`data-balance-minutes="2880"`,
		`data-balance-minutes="4320"`,
		`data-balance-minutes="10080"`,
	} {
		if !strings.Contains(tvbotHTML, marker) {
			t.Fatalf("tvbot ui missing %s", marker)
		}
	}
	for _, old := range []string{
		`OKX USDT 余额表`,
		`Binance USDT 余额表`,
		`OKX USDT估值`,
		`Binance USDT估值`,
		`估值 USD`,
		`analysis-total-eq`,
	} {
		if strings.Contains(tvbotHTML, old) {
			t.Fatalf("tvbot ui should not include %s", old)
		}
	}
	if !strings.Contains(tvbotHTML, `analysis-asset-count`) {
		t.Fatal("tvbot ui should keep OKX asset count")
	}
	if !strings.Contains(tvbotHTML, `String(row.ccy || "").toUpperCase() === "USDT"`) {
		t.Fatal("balance rows should filter to USDT")
	}
	if !strings.Contains(tvbotHTML, `colspan="5"`) {
		t.Fatal("empty balance row should match five visible columns")
	}
}

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
	if got := strings.Count(tvbotHTML, `class="analysis-exchange-block balance-pnl-block"`); got != 2 {
		t.Fatalf("embedded pnl blocks=%d, want 2", got)
	}
	for _, marker := range []string{
		`.exchange-balance-metrics`,
		`.balance-pnl-block`,
		`.analysis-period-row`,
		`.analysis-time-status`,
		`#analysis-trade-history-section`,
		`grid-column: 1 / -1`,
		`.analysis-usdt-chart-card`,
		`订单时间`,
		`OKX 订单`,
		`Binance 订单`,
		`USDT 估值表`,
		`OKX 盈亏分析`,
		`Binance 盈亏分析`,
		`.balance-table-wrap`,
		"height: auto;\n      max-height: 188px;\n      overflow: auto;",
		`#analysis .mini-usdt-chart`,
		`height: 360px`,
		`height: 346px`,
		`miniMinHeight = isAnalysisChart ? 360 : 240`,
		`miniFallbackHeight = isAnalysisChart ? 360 : 250`,
		`function formatUSDTBalance(v)`,
		`Math.round(n).toLocaleString`,
		`const formatted = formatUSDTBalance(v)`,
		`formatUSDTBalance(row.eq)`,
		`formatUSDTBalance(row.avail_bal || row.avail_eq)`,
		`formatUSDTBalance(row.cash_bal)`,
		`formatUSDTBalance(row.frozen_bal)`,
		`function usdtBalanceRawValue(row)`,
		`for (const key of ["eq_usd", "eq", "cash_bal", "avail_bal"])`,
		`function usdtBalancePoints(balancePoints, balance)`,
		`"USDT 估值表 " + balanceWindowLabel(state.balanceWindowMinutes)`,
		`暂无 USDT估值数据`,
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
		`usdtValuationPoints`,
		`估值 USD`,
		`analysis-total-eq`,
		`analysis-avail-eq`,
		`analysis-adj-eq`,
		`analysis-asset-count`,
		`analysis-binance-avail`,
		`analysis-binance-api`,
		`id="analysis-symbol-section"`,
		".balance-table-wrap {\n      height: 188px;",
	} {
		if strings.Contains(tvbotHTML, old) {
			t.Fatalf("tvbot ui should not include %s", old)
		}
	}
	analysisUpdated := strings.Index(tvbotHTML, `id="analysis-updated"`)
	windowToolbar := strings.Index(tvbotHTML, `class="balance-window-toolbar"`)
	analysisGrid := strings.Index(tvbotHTML, `class="analysis-balance-grid"`)
	if analysisUpdated < 0 || windowToolbar < 0 || analysisGrid < 0 {
		t.Fatal("analysis top layout markers are missing")
	}
	if !(analysisUpdated < windowToolbar && windowToolbar < analysisGrid) {
		t.Fatal("analysis should show order time and period switch before exchange columns")
	}
	okxOrder := strings.Index(tvbotHTML, `OKX 订单`)
	binanceOrder := strings.Index(tvbotHTML, `Binance 订单`)
	okxBalanceRows := strings.Index(tvbotHTML, `id="analysis-balance-rows"`)
	binanceBalanceRows := strings.Index(tvbotHTML, `id="analysis-binance-balance-rows"`)
	tradeSection := strings.Index(tvbotHTML, `id="analysis-trade-history-section"`)
	okxChart := strings.Index(tvbotHTML, `id="usdt-chart" class="mini-usdt-chart"`)
	binanceChart := strings.Index(tvbotHTML, `id="analysis-binance-usdt-chart" class="mini-usdt-chart"`)
	okxPNL := strings.Index(tvbotHTML, `OKX 盈亏分析`)
	binancePNL := strings.Index(tvbotHTML, `Binance 盈亏分析`)
	if okxOrder < 0 || binanceOrder < 0 || okxBalanceRows < 0 || binanceBalanceRows < 0 || tradeSection < 0 || okxChart < 0 || binanceChart < 0 || okxPNL < 0 || binancePNL < 0 {
		t.Fatal("analysis layout markers are missing")
	}
	if !(okxOrder < binanceOrder && binanceOrder < tradeSection) {
		t.Fatal("OKX and Binance order columns should appear before trade history")
	}
	if !(okxBalanceRows < okxChart && okxChart < okxPNL) {
		t.Fatal("OKX column should show USDT valuation table before pnl analysis")
	}
	if !(binanceBalanceRows < binanceChart && binanceChart < binancePNL) {
		t.Fatal("Binance column should show USDT valuation table before pnl analysis")
	}
	if tradeSection < okxPNL || tradeSection < binancePNL {
		t.Fatal("trade history should appear after both pnl analysis tables")
	}
	if got := strings.Count(tvbotHTML, `id="analysis-trade-history-section"`); got != 1 {
		t.Fatalf("trade history sections=%d, want 1", got)
	}
	if got := strings.Count(tvbotHTML, `class="analysis-trade-table"`); got != 1 {
		t.Fatalf("trade history tables=%d, want 1", got)
	}
	if !strings.Contains(tvbotHTML, `String(row.ccy || "").toUpperCase() === "USDT"`) {
		t.Fatal("balance rows should filter to USDT")
	}
	if !strings.Contains(tvbotHTML, `colspan="5"`) {
		t.Fatal("empty balance row should match five visible columns")
	}
}

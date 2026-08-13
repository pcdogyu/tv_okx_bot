package server

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestBalanceWindowQuerySupportsMinutesAndLegacyDays(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantMinutes int
		wantDays    int
	}{
		{name: "current", target: "/tvbot/balances/overview?minutes=0", wantMinutes: 0, wantDays: 0},
		{name: "one hour", target: "/tvbot/balances/overview?minutes=60", wantMinutes: 60, wantDays: 1},
		{name: "ninety days", target: "/tvbot/balances/overview?minutes=129600", wantMinutes: 129600, wantDays: 90},
		{name: "clamped", target: "/tvbot/balances/overview?minutes=999999", wantMinutes: 129600, wantDays: 90},
		{name: "legacy days", target: "/tvbot/balances/overview?days=3", wantMinutes: 4320, wantDays: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			gotMinutes, gotDays := balanceWindowQuery(req)
			if gotMinutes != tc.wantMinutes || gotDays != tc.wantDays {
				t.Fatalf("window=(%d,%d), want (%d,%d)", gotMinutes, gotDays, tc.wantMinutes, tc.wantDays)
			}
		})
	}
}

func TestCompactBalancePointsUsesCurrentAndHourlyLongWindows(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 15, 0, 0, time.UTC)
	points := []analysisBalancePoint{
		{Time: now.Add(-2 * time.Hour), TS: now.Add(-2 * time.Hour).UnixMilli(), Value: 100},
		{Time: now.Add(-90 * time.Minute), TS: now.Add(-90 * time.Minute).UnixMilli(), Value: 110},
		{Time: now.Add(-30 * time.Minute), TS: now.Add(-30 * time.Minute).UnixMilli(), Value: 120},
	}
	current := compactBalancePoints(points, 0)
	if len(current) != 1 || current[0].Value != 120 {
		t.Fatalf("current points=%#v", current)
	}
	longWindow := compactBalancePoints(points, 30*24*60)
	if len(longWindow) != 2 || longWindow[0].Value != 110 || longWindow[1].Value != 120 {
		t.Fatalf("hourly points len=%d points=%#v", len(longWindow), longWindow)
	}
	for _, point := range longWindow {
		if time.UnixMilli(point.TS).Minute() != 0 {
			t.Fatalf("point should be hour-bucketed: %#v", point)
		}
	}
}

func TestSnapshotBalanceValueUsesEquityOnly(t *testing.T) {
	tests := []struct {
		name     string
		snapshot storage.USDTBalanceSnapshot
		want     float64
	}{
		{
			name: "equity preferred over USD valuation and available balances",
			snapshot: storage.USDTBalanceSnapshot{
				CashBal:  "4818.20",
				AvailBal: "4820.00",
				Eq:       "4864.23",
				EqUsd:    "4852.64",
			},
			want: 4864.23,
		},
		{
			name: "missing equity does not fallback to cash balance",
			snapshot: storage.USDTBalanceSnapshot{
				EqUsd:    "4852.64",
				CashBal:  "4818.20",
				AvailBal: "4820.00",
			},
			want: 0,
		},
		{
			name: "invalid equity does not fallback to cash balance",
			snapshot: storage.USDTBalanceSnapshot{
				EqUsd:    "bad",
				Eq:       "bad",
				CashBal:  "4818.20",
				AvailBal: "4820.00",
			},
			want: 0,
		},
		{
			name: "invalid equity does not fallback to available balance",
			snapshot: storage.USDTBalanceSnapshot{
				EqUsd:    "bad",
				Eq:       "bad",
				CashBal:  "bad",
				AvailBal: "4820.00",
			},
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotBalanceValue(tc.snapshot); math.Abs(got-tc.want) > 0.0000001 {
				t.Fatalf("snapshotBalanceValue=%f, want %f", got, tc.want)
			}
		})
	}
}

func TestBalancePointsFromSnapshotsUsesEquityForChartValue(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	points := balancePointsFromSnapshots([]storage.USDTBalanceSnapshot{
		{
			BucketTS:   now.UnixMilli(),
			ObservedAt: now,
			EqUsd:      "4852.64",
			Eq:         "4858.04",
			AvailBal:   "4818.20",
			CashBal:    "4818.20",
			FrozenBal:  "39.83",
		},
	})
	if len(points) != 1 {
		t.Fatalf("points len=%d", len(points))
	}
	if math.Abs(points[0].Value-4858.04) > 0.0000001 {
		t.Fatalf("chart value should use USDT equity, points=%#v", points)
	}
	if points[0].EqUsd != "4852.64" || points[0].Eq != "4858.04" || points[0].FrozenBal != "39.83" {
		t.Fatalf("snapshot fields should be preserved: %#v", points[0])
	}
}

func TestAnalysisBalanceFromBinanceIncludesUSDTPositionUnrealizedPnL(t *testing.T) {
	got := analysisBalanceFromBinance([]binance.Balance{
		{
			Asset:              "USDT",
			Balance:            "10306",
			AvailableBalance:   "9900.25",
			CrossWalletBalance: "10000",
			UpdateTime:         1785522000000,
		},
	}, []binance.Position{
		{
			Symbol:           "MANTAUSDT",
			PositionAmt:      "63222.3",
			MarginAsset:      "USDT",
			UnRealizedProfit: "-340.12345678",
		},
		{
			Symbol:           "MEWUSDT",
			PositionAmt:      "-15342129",
			MarginAsset:      "USDT",
			UnRealizedProfit: "20",
		},
		{
			Symbol:           "ETHUSDT",
			PositionAmt:      "0",
			MarginAsset:      "USDT",
			UnRealizedProfit: "999",
		},
		{
			Symbol:           "BTCUSDC",
			PositionAmt:      "1",
			MarginAsset:      "USDC",
			UnRealizedProfit: "999",
		},
	})
	if got.TotalEq != "9985.87654322" || got.AvailEq != "9900.25" {
		t.Fatalf("bad Binance balance: %#v", got)
	}
	if len(got.Details) != 1 || got.Details[0].Eq != "9985.87654322" || got.Details[0].EqUsd != "9985.87654322" || got.Details[0].CashBal != "10000" {
		t.Fatalf("bad Binance USDT detail: %#v", got.Details)
	}
}

func TestEnrichAnalysisTradesUsesMatchedOrderLeverageAndComputesNetPnL(t *testing.T) {
	cfg := config.Config{Symbols: map[string]config.SymbolConfig{
		"BTC": {InstID: "BTC-USDT-SWAP", CtVal: 0.01},
	}}
	trades := []analysisTrade{
		{
			Exchange: trading.ExchangeOKX,
			APIID:    "default",
			InstID:   "BTC-USDT-SWAP",
			OrdID:    "okx-order",
			FillPx:   "50000",
			FillSz:   "2",
			FillPnl:  "3",
			Fee:      "-0.12",
			FeeCcy:   "USDT",
		},
		{
			Exchange: trading.ExchangeBinance,
			APIID:    "binance-alt",
			InstID:   "BTCUSDT",
			OrdID:    "8002",
			FillPx:   "64000",
			FillSz:   "0.03",
			FillPnl:  "5",
			Fee:      "-0.25",
			FeeCcy:   "USDT",
		},
		{
			Exchange: trading.ExchangeOKX,
			APIID:    "default",
			InstID:   "MISSING-USDT-SWAP",
			OrdID:    "missing-ctval",
			FillPx:   "10",
			FillSz:   "1",
			FillPnl:  "1",
			Fee:      "-0.01",
			FeeCcy:   "USDT",
		},
	}
	records := []storage.OrderRecord{
		{
			APIID:          "default",
			TargetExchange: trading.ExchangeOKX,
			Result:         trading.OrderResult{APIID: "default", TargetExchange: trading.ExchangeOKX, InstID: "BTC-USDT-SWAP", OrdID: "okx-order", Leverage: 5},
		},
		{
			APIID:          "binance-alt",
			TargetExchange: trading.ExchangeBinance,
			Result:         trading.OrderResult{APIID: "binance-alt", TargetExchange: trading.ExchangeBinance, InstID: "BTCUSDT", OrdID: "8001 / 8002", Leverage: 10},
		},
		{
			APIID:          "default",
			TargetExchange: trading.ExchangeOKX,
			Result:         trading.OrderResult{APIID: "default", TargetExchange: trading.ExchangeOKX, InstID: "MISSING-USDT-SWAP", OrdID: "missing-ctval"},
			Leverage:       3,
		},
	}
	enrichAnalysisTrades(cfg, trades, records)
	if trades[0].Leverage != 5 || trades[0].Margin != "200" || trades[0].NetPnL != "2.88" {
		t.Fatalf("bad OKX enrichment: %#v", trades[0])
	}
	if trades[1].Leverage != 10 || trades[1].Margin != "192" || trades[1].NetPnL != "4.75" {
		t.Fatalf("bad Binance enrichment: %#v", trades[1])
	}
	if trades[2].Leverage != 3 || trades[2].Margin != "" || trades[2].NetPnL != "0.99" {
		t.Fatalf("missing ctVal should keep leverage/net pnl but omit margin: %#v", trades[2])
	}
}

func TestDeriveBinanceAnalysisSymbolSupportsUSDCContracts(t *testing.T) {
	cases := []struct {
		candidates []string
		want       string
	}{
		{candidates: []string{"PENGUUSDC"}, want: "PENGUUSDC"},
		{candidates: []string{"BINANCE:PENGUUSDC.P"}, want: "PENGUUSDC"},
		{candidates: []string{"PENGU-USDC-SWAP"}, want: "PENGUUSDC"},
		{candidates: []string{"", "PENGU"}, want: "PENGUUSDT"},
	}
	for _, tc := range cases {
		got, ok := deriveBinanceAnalysisSymbol(tc.candidates...)
		if !ok || got != tc.want {
			t.Fatalf("deriveBinanceAnalysisSymbol(%#v)=%q ok=%v, want %q", tc.candidates, got, ok, tc.want)
		}
	}
}

func TestBuildAnalysisPositionTradesCountsClosedPositionsOnly(t *testing.T) {
	cfg := config.Config{Symbols: map[string]config.SymbolConfig{
		"TEST": {InstID: "TEST-USDT-SWAP", CtVal: 0.01},
	}}
	periodSince := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	trades := []analysisTrade{
		{
			Exchange:  trading.ExchangeOKX,
			APIID:     "default",
			InstID:    "TEST-USDT-SWAP",
			TradeID:   "long-open",
			OrdID:     "long-open-order",
			Side:      "buy",
			PosSide:   "long",
			FillPx:    "100",
			FillSz:    "10",
			Fee:       "-0.1",
			FeeCcy:    "USDT",
			FillTime:  periodSince.Add(-time.Hour),
			FillTS:    periodSince.Add(-time.Hour).UnixMilli(),
			FillCount: 1,
		},
		{
			Exchange:  trading.ExchangeOKX,
			APIID:     "default",
			InstID:    "TEST-USDT-SWAP",
			TradeID:   "long-close",
			OrdID:     "long-close-order",
			Side:      "sell",
			PosSide:   "long",
			FillPx:    "110",
			FillSz:    "10",
			FillPnl:   "10",
			Fee:       "-0.2",
			FeeCcy:    "USDT",
			FillTime:  periodSince.Add(time.Hour),
			FillTS:    periodSince.Add(time.Hour).UnixMilli(),
			FillCount: 1,
		},
		{
			Exchange:  trading.ExchangeOKX,
			APIID:     "default",
			InstID:    "TEST-USDT-SWAP",
			TradeID:   "short-open",
			OrdID:     "short-open-order",
			Side:      "sell",
			PosSide:   "short",
			FillPx:    "200",
			FillSz:    "5",
			Fee:       "-0.05",
			FeeCcy:    "USDT",
			FillTime:  periodSince.Add(2 * time.Hour),
			FillTS:    periodSince.Add(2 * time.Hour).UnixMilli(),
			FillCount: 1,
		},
		{
			Exchange:  trading.ExchangeOKX,
			APIID:     "default",
			InstID:    "TEST-USDT-SWAP",
			TradeID:   "short-close",
			OrdID:     "short-close-order",
			Side:      "buy",
			PosSide:   "short",
			FillPx:    "210",
			FillSz:    "5",
			FillPnl:   "-5",
			Fee:       "-0.07",
			FeeCcy:    "USDT",
			FillTime:  periodSince.Add(3 * time.Hour),
			FillTS:    periodSince.Add(3 * time.Hour).UnixMilli(),
			FillCount: 1,
		},
		{
			Exchange:  trading.ExchangeOKX,
			APIID:     "default",
			InstID:    "TEST-USDT-SWAP",
			TradeID:   "unclosed-open",
			OrdID:     "unclosed-open-order",
			Side:      "buy",
			PosSide:   "long",
			FillPx:    "300",
			FillSz:    "1",
			Fee:       "-0.01",
			FeeCcy:    "USDT",
			FillTime:  periodSince.Add(4 * time.Hour),
			FillTS:    periodSince.Add(4 * time.Hour).UnixMilli(),
			FillCount: 1,
		},
	}
	positions := buildAnalysisPositionTrades(cfg, trades, periodSince)
	if len(positions) != 2 {
		t.Fatalf("positions len=%d positions=%#v", len(positions), positions)
	}
	if positions[0].Side != "short" || positions[0].NetPnL != "-5.12" {
		t.Fatalf("bad short position: %#v", positions[0])
	}
	if positions[1].Side != "long" || positions[1].NetPnL != "9.7" {
		t.Fatalf("bad long position: %#v", positions[1])
	}
	summary, exchanges, _ := computeStats(cfg, positions, trades, periodSince)
	if summary.TradeCount != 2 || summary.Wins != 1 || summary.Losses != 1 || math.Abs(summary.WinRate-0.5) > 0.0000001 {
		t.Fatalf("bad summary counts: %#v", summary)
	}
	if math.Abs(summary.NetPnL-4.58) > 0.0000001 {
		t.Fatalf("bad summary net pnl: %#v", summary)
	}
	if math.Abs(summary.Fees-(-0.33)) > 0.0000001 || math.Abs(summary.Turnover-34.5) > 0.0000001 {
		t.Fatalf("period fees/turnover should only use in-window fills: %#v", summary)
	}
	if len(exchanges) != 1 || exchanges[0].TradeCount != 2 {
		t.Fatalf("bad exchange stats: %#v", exchanges)
	}
}

func TestBuildAnalysisPositionTradesMergesClosedFillsByEntryOrder(t *testing.T) {
	cfg := config.Config{Symbols: map[string]config.SymbolConfig{
		"TEST": {InstID: "TEST-USDT-SWAP", CtVal: 1},
	}}
	periodSince := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	tradeAt := func(hours int) (time.Time, int64) {
		ts := periodSince.Add(time.Duration(hours) * time.Hour)
		return ts, ts.UnixMilli()
	}
	fillTime, fillTS := tradeAt(-2)
	trades := []analysisTrade{
		{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "long-open-a", OrdID: "long-entry", Side: "buy", PosSide: "long", FillPx: "100", FillSz: "2", Fee: "-0.2", FeeCcy: "USDT", Leverage: 10, FillTime: fillTime, FillTS: fillTS, FillCount: 1},
	}
	fillTime, fillTS = tradeAt(-1)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "long-open-b", OrdID: "long-entry", Side: "buy", PosSide: "long", FillPx: "110", FillSz: "3", Fee: "-0.3", FeeCcy: "USDT", Leverage: 10, FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(1)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "long-close-a", OrdID: "long-exit-a", Side: "sell", PosSide: "long", FillPx: "120", FillSz: "1", FillPnl: "20", Fee: "-0.1", FeeCcy: "USDT", FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(2)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "long-close-b", OrdID: "long-exit-b", Side: "sell", PosSide: "long", FillPx: "130", FillSz: "4", FillPnl: "80", Fee: "-0.4", FeeCcy: "USDT", FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(3)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "short-open", OrdID: "short-entry", Side: "sell", PosSide: "short", FillPx: "200", FillSz: "2", Fee: "-0.2", FeeCcy: "USDT", Leverage: 10, FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(4)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "short-close-a", OrdID: "short-exit-a", Side: "buy", PosSide: "short", FillPx: "210", FillSz: "0.5", FillPnl: "-5", Fee: "-0.05", FeeCcy: "USDT", FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(5)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "short-close-b", OrdID: "short-exit-b", Side: "buy", PosSide: "short", FillPx: "205", FillSz: "1.5", FillPnl: "-7.5", Fee: "-0.15", FeeCcy: "USDT", FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(6)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "partial-open", OrdID: "partial-entry", Side: "buy", PosSide: "long", FillPx: "50", FillSz: "2", Fee: "-0.2", FeeCcy: "USDT", Leverage: 10, FillTime: fillTime, FillTS: fillTS, FillCount: 1})
	fillTime, fillTS = tradeAt(7)
	trades = append(trades, analysisTrade{Exchange: trading.ExchangeOKX, APIID: "default", InstID: "TEST-USDT-SWAP", TradeID: "partial-close", OrdID: "partial-exit", Side: "sell", PosSide: "long", FillPx: "55", FillSz: "1", FillPnl: "5", Fee: "-0.1", FeeCcy: "USDT", FillTime: fillTime, FillTS: fillTS, FillCount: 1})

	positions := buildAnalysisPositionTrades(cfg, trades, periodSince)
	if len(positions) != 2 {
		t.Fatalf("positions len=%d positions=%#v", len(positions), positions)
	}
	bySide := map[string]analysisPositionTrade{}
	for _, position := range positions {
		bySide[position.Side] = position
	}
	long := bySide[trading.PositionSideLong]
	if long.EntryOrdID != "long-entry" || long.ExitOrdID != "long-exit-a / long-exit-b" || long.Qty != "5" || long.EntryPx != "106" || long.ExitPx != "128" {
		t.Fatalf("bad merged long identity/prices: %#v", long)
	}
	if long.Margin != "53" || long.RealizedPnL != "100" || long.Fee != "-1" || long.NetPnL != "99" || long.Turnover != "1170" || long.FillCount != 4 {
		t.Fatalf("bad merged long totals: %#v", long)
	}
	short := bySide[trading.PositionSideShort]
	if short.EntryOrdID != "short-entry" || short.ExitOrdID != "short-exit-a / short-exit-b" || short.Qty != "2" || short.EntryPx != "200" || short.ExitPx != "206.25" {
		t.Fatalf("bad merged short identity/prices: %#v", short)
	}
	if short.Margin != "40" || short.RealizedPnL != "-12.5" || short.Fee != "-0.4" || short.NetPnL != "-12.9" || short.Turnover != "812.5" || short.FillCount != 3 {
		t.Fatalf("bad merged short totals: %#v", short)
	}
	summary, _, _ := computeStats(cfg, positions, trades, periodSince)
	if summary.TradeCount != 2 || summary.Wins != 1 || summary.Losses != 1 || math.Abs(summary.NetPnL-86.1) > 0.0000001 {
		t.Fatalf("bad merged summary: %#v", summary)
	}
}

func TestBalanceWindowStatsComputesChangeAndMaxDrawdown(t *testing.T) {
	base := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	got := balanceWindowStats([]analysisBalancePoint{
		{Time: base, TS: base.UnixMilli(), Value: 100},
		{Time: base.Add(time.Hour), TS: base.Add(time.Hour).UnixMilli(), Value: 120},
		{Time: base.Add(2 * time.Hour), TS: base.Add(2 * time.Hour).UnixMilli(), Value: 90},
		{Time: base.Add(3 * time.Hour), TS: base.Add(3 * time.Hour).UnixMilli(), Value: 110},
	}, analysisBalance{})
	if got.StartValue != 100 || got.CurrentValue != 110 || got.Change != 10 || math.Abs(got.ChangePct-0.1) > 0.0000001 {
		t.Fatalf("bad window change: %#v", got)
	}
	if got.MaxDrawdown != 30 || math.Abs(got.MaxDrawdownPct-0.25) > 0.0000001 {
		t.Fatalf("bad max drawdown: %#v", got)
	}
}

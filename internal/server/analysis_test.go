package server

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

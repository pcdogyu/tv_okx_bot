package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestCoinpairCooldownIdentityUsesFuzzyBaseKeyword(t *testing.T) {
	cases := map[string]string{
		"ETHUSDT":          "ETH",
		"ETH-USDT-SWAP":    "ETH",
		"OKX:ETHUSDT.P":    "ETH",
		"1000PEPEUSDT":     "1000PEPE",
		"BINANCE:BTCFDUSD": "BTC",
	}
	for input, want := range cases {
		got, _ := coinpairCooldownIdentity(input)
		if got != want {
			t.Fatalf("identity(%q)=%q want %q", input, got, want)
		}
	}
}

func TestNegativeRealizedPnLClassification(t *testing.T) {
	for raw, want := range map[string]bool{
		"-0.0001": true,
		"-5":      true,
		"0":       false,
		"2.5":     false,
		"invalid": false,
	} {
		if got := negativeRealizedPnL(raw); got != want {
			t.Fatalf("negativeRealizedPnL(%q)=%v want %v", raw, got, want)
		}
	}
}

func TestStopLossCloseSignalRequiresExplicitStopLoss(t *testing.T) {
	cases := []struct {
		signal trading.Signal
		want   bool
	}{
		{signal: trading.Signal{PositionEffect: trading.PositionEffectClose, OrderIntent: "sl_short"}, want: true},
		{signal: trading.Signal{PositionEffect: trading.PositionEffectClose, Condition: "空单止损"}, want: true},
		{signal: trading.Signal{PositionEffect: trading.PositionEffectClose, Condition: "空单止盈"}, want: false},
		{signal: trading.Signal{PositionEffect: trading.PositionEffectClose, Condition: "空单止盈止损"}, want: false},
		{signal: trading.Signal{PositionEffect: trading.PositionEffectOpen, Condition: "多单止损"}, want: false},
	}
	for _, tc := range cases {
		if got := isStopLossCloseSignal(tc.signal); got != tc.want {
			t.Fatalf("isStopLossCloseSignal(%#v)=%v want %v", tc.signal, got, tc.want)
		}
	}
}

func TestTVOrderCooldownBlocksBeforeExecutorAndListsRule(t *testing.T) {
	srv := newTestServer(t)
	now := srv.now()
	block, created, err := srv.recordCoinpairCooldown(
		"test-stop:1",
		"exchange_fill",
		trading.ExchangeOKX,
		"default",
		"2500",
		now,
		"ETH-USDT-SWAP",
	)
	if err != nil || !created || block.Keyword != "ETH" {
		t.Fatalf("add cooldown: created=%v block=%#v err=%v", created, block, err)
	}
	signal := validSignal(t, srv)
	signal.Coinpair = "ETHFI"
	signal.Ticker = "OKX:ETHFIUSDT.P"
	signal.Normalize()
	signal.Token = srv.Token.Generate(signal.CanonicalTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status       string    `json:"status"`
		Reason       string    `json:"reason"`
		SignalID     string    `json:"signal_id"`
		Keyword      string    `json:"keyword"`
		BlockedUntil time.Time `json:"blocked_until"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ignored" || resp.Reason != "coinpair_cooldown" || resp.Keyword != "ETH" || !resp.BlockedUntil.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("bad cooldown response: %#v", resp)
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		t.Fatalf("cooldown signal reached executor: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
	rec, found := srv.Orders.Get(resp.SignalID)
	if !found || rec.Status != storage.StatusIgnored || rec.ErrorCode != "coinpair_cooldown" || !strings.Contains(rec.Error, "ETH") {
		t.Fatalf("bad cooldown history: found=%v record=%#v", found, rec)
	}

	listRR := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/tvbot/coinpair-blocks", nil)
	listReq.SetBasicAuth("admin", "Admin123")
	srv.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), `"keyword":"ETH"`) || !strings.Contains(listRR.Body.String(), `"trigger_price":"2500"`) {
		t.Fatalf("bad coinpair block list: status=%d body=%s", listRR.Code, listRR.Body.String())
	}

	closeProbe := trading.Signal{Coinpair: "ETHFI", Ticker: "ETHFIUSDT", PositionEffect: trading.PositionEffectClose}
	if _, blocked, err := srv.activeCoinpairCooldown(closeProbe, now); err != nil || blocked {
		t.Fatalf("close signal should bypass cooldown: blocked=%v err=%v", blocked, err)
	}
}

func TestAutoReentryStopsBeforeExchangeWhenCooldownActive(t *testing.T) {
	srv := newTestServer(t)
	now := srv.now()
	rec := submitMonitorTestOrder(t, srv.Orders, now, "auto-entry")
	lifecycle, err := srv.Orders.UpsertTradeLifecycleFromOrder(rec, "", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.recordCoinpairCooldown("auto-stop:1", "exchange_fill", trading.ExchangeBinance, "main", "49000", now, "BTCUSDT"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Trading.AutoReentry.Enabled = true
	if err := srv.maybeSubmitAutoReentry(context.Background(), cfg, binance.Client{}, lifecycle, now); err != nil {
		t.Fatal(err)
	}
	updated, found, err := srv.Orders.FindTradeLifecycle(lifecycle.LifecycleID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || updated.Status != storage.TradeLifecycleCooldown || updated.CooldownUntil == "" {
		t.Fatalf("auto reentry was not cooled down: found=%v lifecycle=%#v", found, updated)
	}
}

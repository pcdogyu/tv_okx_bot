package okx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestTraderExecuteSignalPlacesLeverageAndOrderWithTPSL(t *testing.T) {
	var leverageSeen bool
	var orderReq PlaceOrderRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/set-leverage":
			leverageSeen = true
			var req SetLeverageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.InstID != "BTC-USDT-SWAP" || req.Lever != "5" || req.MgnMode != "isolated" {
				t.Fatalf("bad leverage request: %#v", req)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{}]}`))
		case "/api/v5/trade/order":
			if r.Header.Get("x-simulated-trading") != "1" {
				t.Fatal("missing demo header on order")
			}
			if err := json.NewDecoder(r.Body).Decode(&orderReq); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"clOrdId":"x","ordId":"123","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BaseURL = ts.URL
	tp := trading.NewFlexibleFloat(2)
	sl := trading.NewFlexibleFloat(1)
	signal := trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		SentAt:   "2026-07-24T03:00:00Z",
		Ticker:   "BTCUSDT",
		Leverage: 5,
		Amount:   trading.NewFlexibleFloat(100),
		Risk:     trading.Risk{Type: trading.RiskTPSL, TPPct: &tp, SLPct: &sl},
	}
	trader := Trader{
		Credentials: Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
		HTTPClient:  ts.Client(),
	}
	result, err := trader.ExecuteSignal(context.Background(), signal, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !leverageSeen {
		t.Fatal("expected leverage request")
	}
	if result.OrdID != "123" {
		t.Fatalf("ord id = %q", result.OrdID)
	}
	if orderReq.InstID != "BTC-USDT-SWAP" || orderReq.Side != "buy" || orderReq.OrdType != "market" || orderReq.Sz != "0.2" {
		t.Fatalf("bad order request: %#v", orderReq)
	}
	if len(orderReq.AttachAlgoOrds) != 1 {
		t.Fatalf("attach algo length = %d", len(orderReq.AttachAlgoOrds))
	}
	attach := orderReq.AttachAlgoOrds[0]
	if attach["tpTriggerRatio"] != "0.02" || attach["slTriggerRatio"] != "0.01" || attach["tpOrdPx"] != "-1" || attach["slOrdPx"] != "-1" {
		t.Fatalf("bad attach algo: %#v", attach)
	}
}

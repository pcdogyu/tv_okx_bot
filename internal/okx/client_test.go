package okx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSetLeverageSignsPrivateDemoRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	var saw bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = true
		if r.URL.Path != "/api/v5/account/set-leverage" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		if r.Header.Get("OK-ACCESS-KEY") != "key" || r.Header.Get("OK-ACCESS-PASSPHRASE") != "pass" {
			t.Fatal("missing OKX auth headers")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := json.Marshal(SetLeverageRequest{InstID: "BTC-USDT-SWAP", Lever: "5", MgnMode: "isolated"})
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodPost, "/api/v5/account/set-leverage", string(bodyBytes), secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		if body["instId"] != "BTC-USDT-SWAP" || body["lever"] != "5" || body["mgnMode"] != "isolated" {
			t.Fatalf("unexpected body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{}]}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	err := client.SetLeverage(context.Background(), SetLeverageRequest{
		InstID:  "BTC-USDT-SWAP",
		Lever:   "5",
		MgnMode: "isolated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saw {
		t.Fatal("server did not receive request")
	}
}

func TestClientAPIErrorKeepsDetailCodes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"1","msg":"All operations failed","data":[{"sCode":"59102","sMsg":"Leverage exceeds the maximum limit. Please lower the leverage."}]}`))
	}))
	defer ts.Close()
	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
		HTTPClient:  ts.Client(),
	}
	err := client.SetLeverage(context.Background(), SetLeverageRequest{
		InstID:  "MSFT-USDT-SWAP",
		Lever:   "10",
		MgnMode: "isolated",
	})
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T %v", err, err)
	}
	if !apiErr.HasCode("59102") {
		t.Fatalf("expected detail code 59102 in %#v", apiErr)
	}
	if got := apiErr.Error(); got != "okx code 1: All operations failed: 59102: Leverage exceeds the maximum limit. Please lower the leverage." {
		t.Fatalf("bad error text: %q", got)
	}
}

func TestClientMarketCandlesUsesUSDTUSDQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/candles" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instId") != "USDT-USD" || r.URL.Query().Get("bar") != "1H" || r.URL.Query().Get("limit") != "72" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("OK-ACCESS-KEY") != "" {
			t.Fatal("public candles request should not be signed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[["1784876400000","0.9992","0.9993","0.9991","0.9992","8125","8118","8118","1"]]}`))
	}))
	defer ts.Close()
	client := Client{BaseURL: ts.URL, HTTPClient: ts.Client()}
	candles, _, err := client.MarketCandles(context.Background(), "USDT-USD", "1H", 72)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 || candles[0].Close != "0.9992" || candles[0].Confirm != "1" {
		t.Fatalf("bad candles: %#v", candles)
	}
}

func TestClientSwapInstrumentsParsesPublicDemoResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/public/instruments" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instType") != "SWAP" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		if r.Header.Get("OK-ACCESS-KEY") != "" {
			t.Fatal("public instruments request should not be signed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","baseCcy":"BTC","quoteCcy":"USDT","settleCcy":"USDT","ctVal":"0.01","ctValCcy":"BTC","lotSz":"0.01","minSz":"0.01","lever":"100","state":"live"}]}`))
	}))
	defer ts.Close()
	client := Client{BaseURL: ts.URL, Demo: true, HTTPClient: ts.Client()}
	instruments, env, err := client.SwapInstruments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if env.Code != "0" || len(instruments) != 1 {
		t.Fatalf("bad instruments response env=%#v instruments=%#v", env, instruments)
	}
	got := instruments[0]
	if got.InstID != "BTC-USDT-SWAP" || got.BaseCcy != "BTC" || got.QuoteCcy != "USDT" || got.Lever != "100" || got.State != "live" {
		t.Fatalf("bad parsed instrument: %#v", got)
	}
}

func TestClientAccountBalanceSnapshotGetsAllAssets(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/account/balance" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("balance request should not filter ccy: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodGet, "/api/v5/account/balance", "", secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"totalEq":"80078.07","uTime":"1784880000000","details":[{"ccy":"BTC","eq":"1","eqUsd":"64973.4"},{"ccy":"USDT","eq":"5000","eqUsd":"4996.65"}]}]}`))
	}))
	defer ts.Close()
	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	balance, _, err := client.AccountBalanceSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if balance.TotalEq != "80078.07" || len(balance.Details) != 2 || balance.Details[0].Ccy != "BTC" {
		t.Fatalf("bad balance snapshot: %#v", balance)
	}
}

func TestClientPositionsSignsPrivateDemoRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/account/positions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instType") != "SWAP" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodGet, "/api/v5/account/positions?"+r.URL.Query().Encode(), "", secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"isolated","posId":"1","posSide":"long","pos":"0.5","availPos":"0.5","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","liqPx":"51000","notionalUsd":"32500","margin":"6500","mgnRatio":"100","uTime":"1784880000000"}]}`))
	}))
	defer ts.Close()
	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	positions, _, err := client.Positions(context.Background(), "SWAP")
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 || positions[0].InstID != "BTC-USDT-SWAP" || positions[0].Upl != "500" || positions[0].NotionalUsd != "32500" {
		t.Fatalf("bad positions: %#v", positions)
	}
}

func TestClientPendingOrdersSignsPrivateDemoRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/trade/orders-pending" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instType") != "SWAP" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodGet, "/api/v5/trade/orders-pending?"+r.URL.Query().Encode(), "", secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"buy","posSide":"long","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0.1","avgPx":"63950","state":"partially_filled","lever":"5","cTime":"1784880000000","uTime":"1784880060000"}]}`))
	}))
	defer ts.Close()
	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	orders, _, err := client.PendingOrders(context.Background(), "SWAP")
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].InstID != "BTC-USDT-SWAP" || orders[0].OrdID != "100" || orders[0].AccFillSz != "0.1" {
		t.Fatalf("bad pending orders: %#v", orders)
	}
}

func TestClientPlaceOrderSendsReduceOnly(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	var saw bool
	reqBody := PlaceOrderRequest{
		InstID:     "BTC-USDT-SWAP",
		TDMode:     "cross",
		ClOrdID:    "PC1784880000000000001",
		Side:       "sell",
		OrdType:    "market",
		Sz:         "2",
		ReduceOnly: true,
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = true
		if r.URL.Path != "/api/v5/trade/order" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := json.Marshal(reqBody)
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodPost, "/api/v5/trade/order", string(bodyBytes), secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		if body["reduceOnly"] != true || body["ordType"] != "market" || body["side"] != "sell" || body["sz"] != "2" {
			t.Fatalf("unexpected order body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"PC1784880000000000001","sCode":"0","sMsg":""}]}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: secret, Passphrase: "pass"},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return fixedNow },
	}
	ack, _, err := client.PlaceOrder(context.Background(), reqBody)
	if err != nil {
		t.Fatal(err)
	}
	if !saw || ack.OrdID != "100" || ack.ClOrdID != reqBody.ClOrdID {
		t.Fatalf("bad ack saw=%v ack=%#v", saw, ack)
	}
}

func TestClientCancelOrderSignsPrivateRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	reqBody := CancelOrderRequest{InstID: "BTC-USDT-SWAP", OrdID: "100"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/trade/cancel-order" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := json.Marshal(reqBody)
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodPost, "/api/v5/trade/cancel-order", string(bodyBytes), secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		if body["instId"] != "BTC-USDT-SWAP" || body["ordId"] != "100" {
			t.Fatalf("unexpected cancel body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"","sCode":"0","sMsg":""}]}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: secret, Passphrase: "pass"},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return fixedNow },
	}
	ack, _, err := client.CancelOrder(context.Background(), reqBody)
	if err != nil {
		t.Fatal(err)
	}
	if ack.OrdID != "100" {
		t.Fatalf("bad cancel ack: %#v", ack)
	}
}

func TestClientPendingAndCancelAlgoOrdersSignPrivateRequests(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	cancelReq := []CancelAlgoOrderRequest{{InstID: "BTC-USDT-SWAP", AlgoID: "900"}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		switch r.URL.Path {
		case "/api/v5/trade/orders-algo-pending":
			if r.URL.Query().Get("instType") != "SWAP" || r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
				t.Fatalf("bad algo pending query: %s", r.URL.RawQuery)
			}
			wantSign := sign(timestamp, http.MethodGet, "/api/v5/trade/orders-algo-pending?"+r.URL.Query().Encode(), "", secret)
			if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
				t.Fatal("invalid OKX algo pending signature headers")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","algoId":"900","algoClOrdId":"algo-900","side":"sell","posSide":"long","ordType":"conditional","sz":"1","state":"live"}]}`))
		case "/api/v5/trade/cancel-algos":
			var body []CancelAlgoOrderRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			bodyBytes, _ := json.Marshal(cancelReq)
			wantSign := sign(timestamp, http.MethodPost, "/api/v5/trade/cancel-algos", string(bodyBytes), secret)
			if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
				t.Fatal("invalid OKX cancel algo signature headers")
			}
			if len(body) != 1 || body[0].InstID != "BTC-USDT-SWAP" || body[0].AlgoID != "900" {
				t.Fatalf("unexpected cancel algo body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"algoId":"900","algoClOrdId":"algo-900","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: secret, Passphrase: "pass"},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return fixedNow },
	}
	orders, _, err := client.PendingAlgoOrders(context.Background(), "swap", "btc-usdt-swap")
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].AlgoID != "900" || orders[0].RawJSON == "" {
		t.Fatalf("bad pending algo orders: %#v", orders)
	}
	acks, _, err := client.CancelAlgoOrders(context.Background(), cancelReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(acks) != 1 || acks[0].AlgoID != "900" {
		t.Fatalf("bad cancel algo acks: %#v", acks)
	}
}

func TestClientAmendOrderSignsPrivateRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	reqBody := AmendOrderRequest{
		InstID: "BTC-USDT-SWAP",
		OrdID:  "100",
		NewPx:  "63999.9",
		AttachAlgoOrds: []map[string]string{{
			"attachAlgoClOrdId": "client-100A",
			"tpTriggerRatio":    "0.02",
			"tpOrdPx":           "-1",
			"tpTriggerPxType":   "mark",
			"slTriggerRatio":    "0.01",
			"slOrdPx":           "-1",
			"slTriggerPxType":   "mark",
		}},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/trade/amend-order" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := json.Marshal(reqBody)
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodPost, "/api/v5/trade/amend-order", string(bodyBytes), secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		if body["instId"] != "BTC-USDT-SWAP" || body["ordId"] != "100" || body["newPx"] != "63999.9" {
			t.Fatalf("unexpected amend body: %#v", body)
		}
		attach, ok := body["attachAlgoOrds"].([]any)
		if !ok || len(attach) != 1 {
			t.Fatalf("bad attach algo body: %#v", body)
		}
		first, ok := attach[0].(map[string]any)
		if !ok || first["tpTriggerRatio"] != "0.02" || first["slTriggerRatio"] != "0.01" || first["tpOrdPx"] != "-1" || first["slOrdPx"] != "-1" {
			t.Fatalf("bad attach algo fields: %#v", attach)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","reqId":"req-1","sCode":"0","sMsg":""}]}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: secret, Passphrase: "pass"},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return fixedNow },
	}
	ack, _, err := client.AmendOrder(context.Background(), reqBody)
	if err != nil {
		t.Fatal(err)
	}
	if ack.OrdID != "100" || ack.ClOrdID != "client-100" || ack.ReqID != "req-1" {
		t.Fatalf("bad amend ack: %#v", ack)
	}
}

func TestClientMarketTickerParsesBidAsk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/ticker" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
			t.Fatalf("bad ticker query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("OK-ACCESS-KEY") != "" {
			t.Fatal("public ticker request should not be signed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","bidPx":"99.9","askPx":"100.1","last":"100","ts":"1784880000000"}]}`))
	}))
	defer ts.Close()

	client := Client{BaseURL: ts.URL, HTTPClient: ts.Client()}
	ticker, _, err := client.MarketTicker(context.Background(), "btc-usdt-swap")
	if err != nil {
		t.Fatal(err)
	}
	if ticker.InstID != "BTC-USDT-SWAP" || ticker.BidPx != "99.9" || ticker.AskPx != "100.1" {
		t.Fatalf("bad ticker: %#v", ticker)
	}
}

func TestClientFillsHistorySignsPrivateDemoRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/trade/fills-history" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instType") != "SWAP" || r.URL.Query().Get("after") != "trade-0" || r.URL.Query().Get("limit") != "100" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		if r.Header.Get("OK-ACCESS-KEY") != "key" || r.Header.Get("OK-ACCESS-PASSPHRASE") != "pass" {
			t.Fatal("missing OKX auth headers")
		}
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodGet, "/api/v5/trade/fills-history?"+r.URL.Query().Encode(), "", secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","tradeId":"trade-1","ordId":"ord-1","side":"sell","fillPx":"50000","fillSz":"1","fillPnl":"2.5","fee":"-0.1","feeCcy":"USDT","fillTime":"1784876400000"}]}`))
	}))
	defer ts.Close()
	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	fills, _, err := client.FillsHistory(context.Background(), "SWAP", "trade-0", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 1 || fills[0].TradeID != "trade-1" || fills[0].RawJSON == "" {
		t.Fatalf("bad fills: %#v", fills)
	}
}

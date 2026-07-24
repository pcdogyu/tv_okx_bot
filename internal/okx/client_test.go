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

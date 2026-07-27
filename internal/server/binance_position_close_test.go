package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
)

func TestBinancePositionToOKXDerivesLeverageAndNotional(t *testing.T) {
	position := binancePositionToOKX(binance.Position{
		Symbol:                "DOGEUSDT",
		PositionSide:          "BOTH",
		PositionAmt:           "2500",
		EntryPrice:            "0.1",
		MarkPrice:             "0.12",
		UnRealizedProfit:      "50",
		Leverage:              "0",
		MarginType:            "isolated",
		LiquidationPrice:      "0.08",
		MarginAsset:           "USDT",
		IsolatedMargin:        "37",
		PositionInitialMargin: "30",
	})
	if position.NotionalUsd != "300" || position.Lever != "10" || position.Margin != "37" {
		t.Fatalf("expected derived notional/leverage, got %#v", position)
	}
}

func TestTVBotBinancePositionCloseMarketAndLimit(t *testing.T) {
	oldPoll := positionClosePollInterval
	oldTimeout := positionCloseLimitTimeout
	oldJobs := positionCloseJobs
	positionClosePollInterval = time.Hour
	positionCloseLimitTimeout = time.Hour
	positionCloseJobs = newPositionCloseRegistry()
	t.Cleanup(func() {
		positionClosePollInterval = oldPoll
		positionCloseLimitTimeout = oldTimeout
		positionCloseJobs = oldJobs
	})

	srv := newTestServer(t)
	var orderForms []url.Values
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/exchangeInfo" && r.URL.Path != "/fapi/v1/ticker/bookTicker" && r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/fapi/v3/positionRisk":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("bad positions query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"-2","entryPrice":"100","markPrice":"100.1","unRealizedProfit":"-0.2","liquidationPrice":"120","isolatedMargin":"20","notional":"200.2","marginAsset":"USDT","leverage":"10","marginType":"isolated","updateTime":1784880000000}]`))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v1/ticker/bookTicker":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("bad book ticker query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"100","bidQty":"1","askPrice":"100.2","askQty":"1","time":1784880000000}`))
		case "/fapi/v1/order":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			orderForms = append(orderForms, cloneValues(r.Form))
			_, _ = w.Write([]byte(`{"orderId":789,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"close-1","price":"0","origQty":"2","executedQty":"0","type":"MARKET","side":"BUY","positionSide":"BOTH"}`))
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = binanceServer.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:          "main",
		Active:      true,
		Credentials: binance.Credentials{APIKey: "binance-key", SecretKey: "binance-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		mode string
		code int
	}{
		{mode: "market", code: http.StatusOK},
		{mode: "limit", code: http.StatusAccepted},
	} {
		body := []byte(`{"exchange":"binance","api_id":"main","inst_id":"BTCUSDT","pos_side":"net","mode":"` + tc.mode + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/tvbot/positions/close", bytes.NewReader(body))
		req.SetBasicAuth("admin", "Admin123")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Fatalf("%s close status=%d body=%s", tc.mode, rr.Code, rr.Body.String())
		}
		var resp positionCloseResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if !resp.OK || resp.APIID != "main" || resp.InstID != "BTCUSDT" || resp.OrdID != "789" {
			t.Fatalf("bad %s close response: %#v", tc.mode, resp)
		}
	}
	if len(orderForms) != 2 {
		t.Fatalf("expected two close orders, got %#v", orderForms)
	}
	market := orderForms[0]
	if market.Get("symbol") != "BTCUSDT" || market.Get("side") != "BUY" || market.Get("type") != "MARKET" || market.Get("quantity") != "2" || market.Get("reduceOnly") != "true" {
		t.Fatalf("bad market close form: %#v", market)
	}
	limit := orderForms[1]
	if limit.Get("symbol") != "BTCUSDT" || limit.Get("side") != "BUY" || limit.Get("type") != "LIMIT" || limit.Get("timeInForce") != "GTC" || limit.Get("quantity") != "2" || limit.Get("price") != "100.2" || limit.Get("reduceOnly") != "true" {
		t.Fatalf("bad limit close form: %#v", limit)
	}
}

func TestTVBotBinanceLimitCloseFallsBackToMarketRemaining(t *testing.T) {
	oldPoll := positionClosePollInterval
	oldTimeout := positionCloseLimitTimeout
	oldJobs := positionCloseJobs
	positionClosePollInterval = time.Hour
	positionCloseLimitTimeout = 20 * time.Millisecond
	positionCloseJobs = newPositionCloseRegistry()
	t.Cleanup(func() {
		positionClosePollInterval = oldPoll
		positionCloseLimitTimeout = oldTimeout
		positionCloseJobs = oldJobs
	})

	srv := newTestServer(t)
	var mu sync.Mutex
	var orderForms []url.Values
	var cancelForms []url.Values
	marketSeen := make(chan struct{}, 1)
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/exchangeInfo" && r.URL.Path != "/fapi/v1/ticker/bookTicker" && r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"-10","entryPrice":"100","markPrice":"100.1","unRealizedProfit":"-1","isolatedMargin":"100","notional":"1001","marginAsset":"USDT","leverage":"10","marginType":"isolated","updateTime":1784880000000}]`))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v1/ticker/bookTicker":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"100","bidQty":"1","askPrice":"100.2","askQty":"1","time":1784880000000}`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":100,"clientOrderId":"limit-close","price":"100.2","origQty":"10","executedQty":"2","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","time":1784880000000,"updateTime":1784880000000,"reduceOnly":true}]`))
		case "/fapi/v1/order":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			switch r.Method {
			case http.MethodPost:
				form := cloneValues(r.Form)
				mu.Lock()
				orderForms = append(orderForms, form)
				mu.Unlock()
				if form.Get("type") == "MARKET" {
					select {
					case marketSeen <- struct{}{}:
					default:
					}
					_, _ = w.Write([]byte(`{"orderId":101,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"market-close","origQty":"8","executedQty":"0","type":"MARKET","side":"BUY","positionSide":"BOTH"}`))
					return
				}
				_, _ = w.Write([]byte(`{"orderId":100,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"limit-close","price":"100.2","origQty":"10","executedQty":"0","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			case http.MethodDelete:
				mu.Lock()
				cancelForms = append(cancelForms, cloneValues(r.Form))
				mu.Unlock()
				_, _ = w.Write([]byte(`{"orderId":100,"symbol":"BTCUSDT","status":"CANCELED","clientOrderId":"limit-close","price":"100.2","origQty":"10","executedQty":"2","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			default:
				t.Fatalf("unexpected Binance order method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = binanceServer.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:          "main",
		Active:      true,
		Credentials: binance.Credentials{APIKey: "binance-key", SecretKey: "binance-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"exchange":"binance","api_id":"main","inst_id":"BTCUSDT","pos_side":"net","mode":"limit"}`)
	req := httptest.NewRequest(http.MethodPost, "/tvbot/positions/close", bytes.NewReader(body))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("limit close status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-marketSeen:
	case <-time.After(time.Second):
		t.Fatal("expected Binance market fallback")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(cancelForms) != 1 || cancelForms[0].Get("orderId") != "100" {
		t.Fatalf("expected limit close cancel, got %#v", cancelForms)
	}
	if len(orderForms) < 2 {
		t.Fatalf("expected initial limit and fallback market orders, got %#v", orderForms)
	}
	market := orderForms[len(orderForms)-1]
	if market.Get("type") != "MARKET" || market.Get("side") != "BUY" || market.Get("quantity") != "8" || market.Get("reduceOnly") != "true" {
		t.Fatalf("bad market fallback form: %#v", market)
	}
}

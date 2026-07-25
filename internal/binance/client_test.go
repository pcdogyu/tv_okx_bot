package binance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestClientSignsPrivateBalanceRequest(t *testing.T) {
	const secret = "unit-secret"
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v3/balance" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-MBX-APIKEY") != "unit-key" {
			t.Fatalf("missing API key header")
		}
		q := r.URL.Query()
		if q.Get("timestamp") != strconv.FormatInt(now.UnixMilli(), 10) {
			t.Fatalf("bad timestamp: %s", r.URL.RawQuery)
		}
		gotSig := q.Get("signature")
		q.Del("signature")
		if gotSig != sign(q.Encode(), secret) {
			t.Fatalf("bad signature got=%s query=%s", gotSig, q.Encode())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"asset":"USDT","balance":"123.45","availableBalance":"120.00","crossWalletBalance":"122.00","updateTime":1784880000000}]`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "unit-key", SecretKey: secret},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return now },
	}
	balances, err := client.AccountBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usdt, ok := USDTBalanceFromAccount(balances)
	if !ok || usdt.Balance != "123.45" || usdt.AvailableBalance != "120.00" {
		t.Fatalf("bad balance: %#v ok=%v", usdt, ok)
	}
}

func TestClientParsesAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":-2015,"msg":"invalid api-key"}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL:     ts.URL,
		Credentials: Credentials{APIKey: "key", SecretKey: "secret"},
		HTTPClient:  ts.Client(),
		Now:         func() time.Time { return time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC) },
	}
	_, err := client.AccountBalance(context.Background())
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Code != -2015 || apiErr.Msg != "invalid api-key" {
		t.Fatalf("bad api error: %#v", err)
	}
}

func TestClientTradingEndpoints(t *testing.T) {
	seen := map[string]url.Values{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		seen[r.URL.Path] = r.Form
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionAmt":"0.1","entryPrice":"50000","markPrice":"50100","unRealizedProfit":"10","positionSide":"BOTH"}]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":123,"clientOrderId":"cid","price":"50000","origQty":"0.1","executedQty":"0","side":"BUY","type":"LIMIT","status":"NEW","time":1784880000000}]`))
		case "/fapi/v1/order":
			_, _ = w.Write([]byte(`{"orderId":456,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"50000","origQty":"0.1","executedQty":"0","type":"LIMIT","side":"BUY"}`))
		case "/fapi/v1/algoOrder":
			_, _ = w.Write([]byte(`{"algoId":789,"clientAlgoId":"tp","orderType":"TAKE_PROFIT_MARKET","symbol":"BTCUSDT","side":"SELL","quantity":"0.1","algoStatus":"NEW","triggerPrice":"51000"}`))
		case "/fapi/v1/leverage":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	client := Client{BaseURL: ts.URL, Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client(), Now: func() time.Time {
		return time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	}}

	positions, err := client.Positions(context.Background(), "BTCUSDT")
	if err != nil || len(positions) != 1 || positions[0].Symbol != "BTCUSDT" {
		t.Fatalf("bad positions err=%v positions=%#v", err, positions)
	}
	orders, err := client.OpenOrders(context.Background(), "BTCUSDT")
	if err != nil || len(orders) != 1 || orders[0].OrderID != 123 {
		t.Fatalf("bad open orders err=%v orders=%#v", err, orders)
	}
	if err := client.SetLeverage(context.Background(), "BTCUSDT", 10); err != nil {
		t.Fatal(err)
	}
	if err := client.ChangeMarginType(context.Background(), "BTCUSDT", "ISOLATED"); err != nil {
		t.Fatal(err)
	}
	ack, err := client.PlaceOrder(context.Background(), PlaceOrderRequest{
		Symbol:           "BTCUSDT",
		Side:             "BUY",
		Type:             "LIMIT",
		TimeInForce:      "GTC",
		Quantity:         "0.1",
		Price:            "50000",
		NewClientOrderID: "entry",
	})
	if err != nil || ack.OrderID != 456 {
		t.Fatalf("bad order ack err=%v ack=%#v", err, ack)
	}
	algo, err := client.NewAlgoOrder(context.Background(), AlgoOrderRequest{
		Symbol:           "BTCUSDT",
		Side:             "SELL",
		Type:             "TAKE_PROFIT_MARKET",
		Quantity:         "0.1",
		TriggerPrice:     "51000",
		WorkingType:      "MARK_PRICE",
		NewClientOrderID: "tp",
		ReduceOnly:       true,
	})
	if err != nil || algo.AlgoID != 789 {
		t.Fatalf("bad algo ack err=%v ack=%#v", err, algo)
	}
	if seen["/fapi/v1/order"].Get("timeInForce") != "GTC" || seen["/fapi/v1/order"].Get("newClientOrderId") != "entry" {
		t.Fatalf("bad order form: %#v", seen["/fapi/v1/order"])
	}
	if seen["/fapi/v1/algoOrder"].Get("algoType") != "CONDITIONAL" || seen["/fapi/v1/algoOrder"].Get("reduceOnly") != "true" {
		t.Fatalf("bad algo form: %#v", seen["/fapi/v1/algoOrder"])
	}
}

func TestExchangeInfoFiltersAndSymbolDerivation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/exchangeInfo" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":2,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
	}))
	defer ts.Close()
	client := Client{BaseURL: ts.URL, HTTPClient: ts.Client()}
	info, err := client.SymbolInfo(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	filters, err := info.TradingFilters()
	if err != nil {
		t.Fatal(err)
	}
	if filters.TickSize != 0.10 || filters.StepSize != 0.001 || filters.MinQty != "0.001" {
		t.Fatalf("bad filters: %#v", filters)
	}
	symbol, err := DeriveUSDMSymbol("BINANCE:ETHUSDT.P", "")
	if err != nil || symbol != "ETHUSDT" {
		t.Fatalf("bad derived symbol %q err=%v", symbol, err)
	}
}

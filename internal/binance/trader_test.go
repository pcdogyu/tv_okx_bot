package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestTraderPlacesLimitOrderAndTPSLAlgoOrders(t *testing.T) {
	var orderForm url.Values
	algoForms := []url.Values{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			if r.Form.Get("marginType") != "ISOLATED" {
				t.Fatalf("bad margin type form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			if r.Form.Get("leverage") != "10" {
				t.Fatalf("bad leverage form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		case "/fapi/v1/order":
			orderForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"49850","origQty":"0.002","executedQty":"0","type":"LIMIT","side":"BUY"}`))
		case "/fapi/v1/algoOrder":
			algoForms = append(algoForms, cloneValues(r.Form))
			_, _ = w.Write([]byte(`{"algoId":456,"clientAlgoId":"algo","orderType":"TAKE_PROFIT_MARKET","symbol":"BTCUSDT","side":"SELL","quantity":"0.002","algoStatus":"NEW","triggerPrice":"50847"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.OrderType = string(trading.OrderTypeLimit)
	cfg.Trading.LongLimitPriceMultiplier = 0.997
	cfg.Trading.RiskType = string(trading.RiskTPSL)
	cfg.Trading.TakeProfitPct = 2
	cfg.Trading.StopLossPct = 1
	signal := trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
		Risk: trading.Risk{
			Type:  trading.RiskTPSL,
			TPPct: ptrFlexible(2),
			SLPct: ptrFlexible(1),
		},
	}
	signal.Normalize()
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	result, err := trader.ExecuteSignal(context.Background(), signal, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetExchange != trading.ExchangeBinance || result.OrdID != "123" || result.Px != "49850" {
		t.Fatalf("bad result: %#v", result)
	}
	if orderForm.Get("symbol") != "BTCUSDT" || orderForm.Get("side") != "BUY" || orderForm.Get("type") != "LIMIT" || orderForm.Get("timeInForce") != "GTC" || orderForm.Get("quantity") != "0.002" || orderForm.Get("price") != "49850" {
		t.Fatalf("bad order form: %#v", orderForm)
	}
	if len(algoForms) != 2 {
		t.Fatalf("expected TP and SL algo orders, got %#v", algoForms)
	}
	types := map[string]url.Values{}
	for _, form := range algoForms {
		types[form.Get("type")] = form
	}
	tp := types["TAKE_PROFIT_MARKET"]
	sl := types["STOP_MARKET"]
	if tp.Get("side") != "SELL" || tp.Get("triggerPrice") != "50847" || tp.Get("workingType") != "MARK_PRICE" {
		t.Fatalf("bad TP form: %#v", tp)
	}
	if sl.Get("side") != "SELL" || sl.Get("triggerPrice") != "49351.5" || sl.Get("workingType") != "MARK_PRICE" {
		t.Fatalf("bad SL form: %#v", sl)
	}
}

func TestTraderPlacesMarketOrderAndTrailingAlgoOrder(t *testing.T) {
	var orderForm url.Values
	var trailingForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		case "/fapi/v1/order":
			orderForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"0","origQty":"0.002","executedQty":"0","type":"MARKET","side":"BUY"}`))
		case "/fapi/v1/algoOrder":
			trailingForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"algoId":456,"clientAlgoId":"trail","algoType":"CONDITIONAL","orderType":"TRAILING_STOP_MARKET","symbol":"BTCUSDT","side":"SELL","positionSide":"BOTH","quantity":"0.002","algoStatus":"NEW","callbackRate":"1.5"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskTrailing)
	cfg.Trading.TrailingPct = 1.5
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	result, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetExchange != trading.ExchangeBinance || result.OrdID != "123" {
		t.Fatalf("bad result: %#v", result)
	}
	if orderForm.Get("type") != "MARKET" || orderForm.Get("quantity") != "0.002" {
		t.Fatalf("bad order form: %#v", orderForm)
	}
	if trailingForm.Get("type") != "TRAILING_STOP_MARKET" ||
		trailingForm.Get("side") != "SELL" ||
		trailingForm.Get("quantity") != "0.002" ||
		trailingForm.Get("callbackRate") != "1.5" ||
		trailingForm.Get("workingType") != "MARK_PRICE" ||
		trailingForm.Get("triggerPrice") != "" ||
		trailingForm.Get("activatePrice") != "" ||
		trailingForm.Get("reduceOnly") != "true" {
		t.Fatalf("bad trailing form: %#v", trailingForm)
	}
}

func TestTraderKeepsSameDirectionPositionAndOrderAsAdd(t *testing.T) {
	var entryForm url.Values
	var cancelCalled bool
	var algoQueried bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0.1","entryPrice":"50000"}]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":100,"clientOrderId":"old-long","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","origQty":"0.1","executedQty":"0"}]`))
		case "/fapi/v1/openAlgoOrders":
			algoQueried = true
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/order":
			if r.Method == http.MethodDelete {
				cancelCalled = true
				_, _ = w.Write([]byte(`{"orderId":100,"status":"CANCELED"}`))
				return
			}
			entryForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"0","origQty":"0.002","executedQty":"0","type":"MARKET","side":"BUY"}`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	_, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cancelCalled || algoQueried {
		t.Fatalf("same direction add should not cancel or query algos cancelCalled=%v algoQueried=%v", cancelCalled, algoQueried)
	}
	if entryForm.Get("side") != "BUY" || entryForm.Get("reduceOnly") != "" || entryForm.Get("quantity") != "0.002" {
		t.Fatalf("bad add entry form: %#v", entryForm)
	}
}

func TestTraderSkipsLeverageWhenBinanceRemoteMatches(t *testing.T) {
	var leverageCalled bool
	var orderForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0","leverage":"10"}]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			leverageCalled = true
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		case "/fapi/v1/order":
			orderForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"0","origQty":"0.002","executedQty":"0","type":"MARKET","side":"BUY"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	result, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if leverageCalled {
		t.Fatal("matching Binance remote leverage should not be set again")
	}
	if result.Leverage != 10 || orderForm.Get("side") != "BUY" {
		t.Fatalf("bad result/order after leverage skip: result=%#v form=%#v", result, orderForm)
	}
}

func TestTraderSetsLeverageWhenBinanceRemoteDiffers(t *testing.T) {
	var leverageForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0","leverage":"5"}]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			leverageForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		case "/fapi/v1/order":
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"0","origQty":"0.002","executedQty":"0","type":"MARKET","side":"BUY"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	if _, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg); err != nil {
		t.Fatal(err)
	}
	if leverageForm.Get("symbol") != "BTCUSDT" || leverageForm.Get("leverage") != "10" {
		t.Fatalf("bad leverage setup form: %#v", leverageForm)
	}
}

func TestTraderCancelsReversePendingOrderBeforeNewDirection(t *testing.T) {
	var paths []string
	var canceledForm url.Values
	var entryForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":100,"clientOrderId":"old-long","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","origQty":"0.1","executedQty":"0"}]`))
		case "/fapi/v1/order":
			switch r.Method {
			case http.MethodDelete:
				canceledForm = cloneValues(r.Form)
				_, _ = w.Write([]byte(`{"orderId":100,"symbol":"BTCUSDT","status":"CANCELED","clientOrderId":"old-long"}`))
			case http.MethodPost:
				entryForm = cloneValues(r.Form)
				_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"entry","price":"0","origQty":"0.002","executedQty":"0","type":"MARKET","side":"SELL"}`))
			default:
				t.Fatalf("unexpected order method %s", r.Method)
			}
		case "/fapi/v1/openAlgoOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	_, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionShort,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if canceledForm.Get("orderId") != "100" {
		t.Fatalf("bad canceled order form: %#v", canceledForm)
	}
	if entryForm.Get("side") != "SELL" || entryForm.Get("reduceOnly") != "" || entryForm.Get("quantity") != "0.002" {
		t.Fatalf("bad reverse entry form: %#v", entryForm)
	}
	if pathIndex(paths, http.MethodDelete+" /fapi/v1/order") > pathIndex(paths, http.MethodPost+" /fapi/v1/order") {
		t.Fatalf("entry placed before reverse order cancellation: %#v", paths)
	}
}

func TestTraderClosesReversePositionBeforeNewDirection(t *testing.T) {
	var positionCalls int
	var canceledAlgo url.Values
	var orderForms []url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":1,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.1"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v3/positionRisk":
			positionCalls++
			if positionCalls <= 2 {
				_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0.1","entryPrice":"50000"}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openAlgoOrders":
			_, _ = w.Write([]byte(`[{"algoId":900,"clientAlgoId":"old-long-tp","algoType":"CONDITIONAL","orderType":"TAKE_PROFIT_MARKET","symbol":"BTCUSDT","side":"SELL","positionSide":"BOTH","quantity":"0.1","algoStatus":"NEW","reduceOnly":true}]`))
		case "/fapi/v1/algoOrder":
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected algo method %s", r.Method)
			}
			canceledAlgo = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"algoId":900,"clientAlgoId":"old-long-tp","code":"200","msg":"success"}`))
		case "/fapi/v1/order":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected order method %s", r.Method)
			}
			orderForms = append(orderForms, cloneValues(r.Form))
			_, _ = w.Write([]byte(`{"orderId":123,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"ok","price":"0","origQty":"0.1","executedQty":"0","type":"MARKET","side":"SELL"}`))
		case "/fapi/v1/marginType":
			_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
		case "/fapi/v1/leverage":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","leverage":10}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	_, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionShort,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if canceledAlgo.Get("algoId") != "900" {
		t.Fatalf("bad canceled algo form: %#v", canceledAlgo)
	}
	if len(orderForms) != 2 {
		t.Fatalf("expected close and entry orders, got %#v", orderForms)
	}
	closeForm, entryForm := orderForms[0], orderForms[1]
	if closeForm.Get("side") != "SELL" || closeForm.Get("type") != "MARKET" || closeForm.Get("quantity") != "0.1" || closeForm.Get("reduceOnly") != "true" {
		t.Fatalf("bad close form: %#v", closeForm)
	}
	if entryForm.Get("side") != "SELL" || entryForm.Get("quantity") != "0.002" || entryForm.Get("reduceOnly") != "" {
		t.Fatalf("bad entry form after close: %#v", entryForm)
	}
}

func TestTraderContinuesWhenMarginTypeSetupBlockedByOpenOrders(t *testing.T) {
	var orderForm url.Values
	var leverageCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"SXTUSDT","status":"TRADING","pricePrecision":6,"quantityPrecision":1,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.000001"},{"filterType":"LOT_SIZE","minQty":"0.1","stepSize":"0.1"}]}]}`))
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/marginType":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":-4067,"msg":"Position side cannot be changed if there exists open orders."}`))
		case "/fapi/v1/leverage":
			leverageCalled = true
			_, _ = w.Write([]byte(`{"symbol":"SXTUSDT","leverage":10}`))
		case "/fapi/v1/order":
			orderForm = cloneValues(r.Form)
			_, _ = w.Write([]byte(`{"orderId":321,"symbol":"SXTUSDT","status":"NEW","clientOrderId":"entry","price":"0.007312","origQty":"68231.4","executedQty":"0","type":"LIMIT","side":"BUY"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	cfg.Trading.OrderType = string(trading.OrderTypeLimit)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	result, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "SXTUSDT.P",
		Ticker:   "SXTUSDT.P",
		Price:    trading.NewFlexibleFloat(0.007335),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(500),
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !leverageCalled {
		t.Fatal("expected leverage setup after margin setup was skipped")
	}
	if result.OrdID != "321" || result.InstID != "SXTUSDT" {
		t.Fatalf("bad order result: %#v", result)
	}
	if orderForm.Get("symbol") != "SXTUSDT" || orderForm.Get("side") != "BUY" || orderForm.Get("type") != "LIMIT" || orderForm.Get("quantity") == "" {
		t.Fatalf("bad order form: %#v", orderForm)
	}
}

func TestTraderRejectsOutOfRangeBinanceTrailingBeforeSubmitting(t *testing.T) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskTrailing)
	cfg.Trading.TrailingPct = 10.1
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	_, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err == nil || !strings.Contains(err.Error(), "between 0.1 and 10") {
		t.Fatalf("expected trailing range error, got %v", err)
	}
	if called {
		t.Fatal("invalid trailing should fail before HTTP calls")
	}
}

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, list := range values {
		for _, value := range list {
			out.Add(key, value)
		}
	}
	return out
}

func ptrFlexible(value float64) *trading.FlexibleFloat {
	v := trading.NewFlexibleFloat(value)
	return &v
}

func pathIndex(paths []string, want string) int {
	for i, path := range paths {
		if path == want {
			return i
		}
	}
	return len(paths) + 1
}

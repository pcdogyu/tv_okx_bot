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

func TestTraderRejectsBinanceTrailingBeforeSubmitting(t *testing.T) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	cfg.Trading.RiskType = string(trading.RiskTrailing)
	trader := Trader{Credentials: Credentials{APIKey: "key", SecretKey: "secret"}, HTTPClient: ts.Client()}
	_, err := trader.ExecuteSignal(context.Background(), trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		Leverage: 10,
		Amount:   trading.NewFlexibleFloat(100),
	}, cfg)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("expected trailing unsupported error, got %v", err)
	}
	if called {
		t.Fatal("trailing unsupported should fail before HTTP calls")
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

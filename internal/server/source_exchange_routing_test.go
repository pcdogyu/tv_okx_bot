package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestTVOrderRoutesBinanceSourceToBinanceTarget(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.Exchange = "BINANCE"
	signal.TargetExchange = trading.ExchangeOKX
	signal.APIID = "okx-moni"
	signal.Token = srv.Token.Generate(signal.CanonicalWebhookTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Exchange != "BINANCE" || got.TargetExchange != trading.ExchangeBinance || got.APIID != "" {
			t.Fatalf("BINANCE source should route to default Binance API: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
	records := srv.Orders.List(10)
	if len(records) != 1 || records[0].SourceExchange != "BINANCE" || records[0].TargetExchange != trading.ExchangeBinance || records[0].APIID != "" {
		t.Fatalf("order record should save Binance source routing: %#v", records)
	}
}

func TestTVOrderRoutesOKXSourceToOKXTarget(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.Exchange = "OKX"
	signal.TargetExchange = trading.ExchangeBinance
	signal.APIID = "binance-main"
	signal.Token = srv.Token.Generate(signal.CanonicalWebhookTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Exchange != "OKX" || got.TargetExchange != trading.ExchangeOKX || got.APIID != "" {
			t.Fatalf("OKX source should route to default OKX API: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
}

func TestOrderRetryUsesSourceExchangeRouting(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.Exchange = "BINANCE"
	signal.TargetExchange = trading.ExchangeOKX
	signal.APIID = "okx-moni"
	signal.Normalize()
	source, duplicate, err := srv.Orders.RecordAccepted(signal, "source-binance-to-okx", srv.now())
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("seed order should not be duplicate")
	}
	if err := srv.Orders.MarkFailed(source.SignalID, fmt.Errorf("old OKX route failed"), srv.now()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/tvbot/orders/"+source.SignalID+"/retry", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Exchange != "BINANCE" || got.TargetExchange != trading.ExchangeBinance || got.APIID != "" {
			t.Fatalf("retry should route by source exchange: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("retry order was not executed")
	}
	records := srv.Orders.List(10)
	if len(records) != 2 {
		t.Fatalf("orders len=%d records=%#v", len(records), records)
	}
	var retry storage.OrderRecord
	for _, rec := range records {
		if rec.SignalID != source.SignalID {
			retry = rec
			break
		}
	}
	if retry.TargetExchange != trading.ExchangeBinance || retry.APIID != "" || retry.SourceExchange != "BINANCE" {
		t.Fatalf("retry record should save source-routed target: %#v", retry)
	}
}

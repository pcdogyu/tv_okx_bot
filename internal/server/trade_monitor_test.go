package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

type tradeMonitorExecutor struct {
	signals []trading.Signal
}

func (e *tradeMonitorExecutor) ExecuteSignal(ctx context.Context, signal trading.Signal, cfg trading.RuntimeConfig) (trading.OrderResult, error) {
	e.signals = append(e.signals, signal)
	return trading.OrderResult{
		APIID:          signal.APIID,
		TargetExchange: trading.ExchangeBinance,
		InstID:         "BTCUSDT",
		ClOrdID:        "reentry-client",
		OrdID:          "9901",
		OrdType:        "MARKET",
		Leverage:       signal.Leverage,
	}, nil
}

func (e *tradeMonitorExecutor) Check(ctx context.Context, cfg trading.RuntimeConfig) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

func TestTradeMonitorStateMachineClassifiesExitFills(t *testing.T) {
	cases := []struct {
		name        string
		realizedPnl string
		wantStatus  string
	}{
		{name: "sl", realizedPnl: "-5.25", wantStatus: storage.TradeLifecycleSLHit},
		{name: "tp", realizedPnl: "7.50", wantStatus: storage.TradeLifecycleTPHit},
		{name: "flat", realizedPnl: "0", wantStatus: storage.TradeLifecycleExited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := storage.NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			now := time.Date(2026, 7, 30, 2, 0, 0, 0, time.UTC)
			rec := submitMonitorTestOrder(t, store, now, "9001")
			if _, err := store.UpsertTradeLifecycleFromOrder(rec, "", 0, now); err != nil {
				t.Fatal(err)
			}
			srv := &Server{Orders: store, Now: func() time.Time { return now }}
			cfg := config.Default()
			entry := storage.BinanceFill{
				APIID:       "main",
				Symbol:      "BTCUSDT",
				TradeID:     "entry-" + tc.name,
				OrderID:     "9001",
				Side:        "BUY",
				Price:       "50000",
				Qty:         "0.01",
				RealizedPnl: "0",
				FillTime:    now.UnixMilli(),
			}
			if err := srv.processBinanceFill(context.Background(), cfg, binance.Client{}, entry, now); err != nil {
				t.Fatal(err)
			}
			exit := storage.BinanceFill{
				APIID:       "main",
				Symbol:      "BTCUSDT",
				TradeID:     "exit-" + tc.name,
				OrderID:     "9101",
				Side:        "SELL",
				Price:       "49500",
				Qty:         "0.01",
				RealizedPnl: tc.realizedPnl,
				FillTime:    now.Add(time.Minute).UnixMilli(),
			}
			if err := srv.processBinanceFill(context.Background(), cfg, binance.Client{}, exit, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			lifecycle, found, err := store.FindTradeLifecycle(rec.SignalID)
			if err != nil {
				t.Fatal(err)
			}
			if !found || lifecycle.Status != tc.wantStatus {
				t.Fatalf("bad lifecycle found=%v lifecycle=%#v", found, lifecycle)
			}
		})
	}
}

func TestTradeMonitorAutoReentrySubmitsOnce(t *testing.T) {
	store, err := storage.NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 7, 30, 3, 0, 0, 0, time.UTC)
	rec := submitMonitorTestOrder(t, store, now, "9001")
	if _, err := store.UpsertTradeLifecycleFromOrder(rec, "", 0, now); err != nil {
		t.Fatal(err)
	}
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[]`))
		case "/fapi/v1/ticker/bookTicker":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"49990","bidQty":"1","askPrice":"50010","askQty":"1","time":1784880000000}`))
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	executor := &tradeMonitorExecutor{}
	srv := &Server{
		Orders:   store,
		Executor: executor,
		Now:      func() time.Time { return now },
	}
	cfg := config.Default()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	cfg.Trading.AutoReentry.Enabled = true
	cfg.Trading.AutoReentry.ReentryAmountPct = 50
	client := binance.Client{
		BaseURL:     binanceServer.URL,
		Credentials: binance.Credentials{APIKey: "key", SecretKey: "secret"},
		HTTPClient:  binanceServer.Client(),
	}
	entry := storage.BinanceFill{
		APIID:       "main",
		Symbol:      "BTCUSDT",
		TradeID:     "entry",
		OrderID:     "9001",
		Side:        "BUY",
		Price:       "50000",
		Qty:         "0.01",
		RealizedPnl: "0",
		FillTime:    now.UnixMilli(),
	}
	if err := srv.processBinanceFill(context.Background(), cfg, client, entry, now); err != nil {
		t.Fatal(err)
	}
	exit := storage.BinanceFill{
		APIID:       "main",
		Symbol:      "BTCUSDT",
		TradeID:     "exit",
		OrderID:     "9101",
		Side:        "SELL",
		Price:       "49500",
		Qty:         "0.01",
		RealizedPnl: "-5",
		FillTime:    now.Add(time.Minute).UnixMilli(),
	}
	if err := srv.processBinanceFill(context.Background(), cfg, client, exit, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(executor.signals) != 1 {
		t.Fatalf("expected one auto reentry signal, got %#v", executor.signals)
	}
	if executor.signals[0].Amount.Value != 50 || executor.signals[0].Exchange != "server_fill_monitor" || executor.signals[0].TargetExchange != trading.ExchangeBinance {
		t.Fatalf("bad auto reentry signal: %#v", executor.signals[0])
	}
	lifecycle, found, err := store.FindTradeLifecycle(rec.SignalID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || lifecycle.Status != storage.TradeLifecycleReentrySubmitted || lifecycle.ReentrySignalID == "" {
		t.Fatalf("bad parent lifecycle after reentry found=%v lifecycle=%#v", found, lifecycle)
	}
	if err := srv.processBinanceFill(context.Background(), cfg, client, exit, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(executor.signals) != 1 {
		t.Fatalf("duplicate stop fill should not submit another reentry, got %#v", executor.signals)
	}
}

func submitMonitorTestOrder(t *testing.T, store *storage.OrderStore, now time.Time, orderID string) storage.OrderRecord {
	t.Helper()
	tp := trading.NewFlexibleFloat(2)
	sl := trading.NewFlexibleFloat(1)
	signal := trading.Signal{
		Action:         trading.ActionLong,
		APIID:          "main",
		TargetExchange: trading.ExchangeBinance,
		Coinpair:       "BTC",
		Ticker:         "BTCUSDT",
		Price:          trading.NewFlexibleFloat(50000),
		SentAt:         now.Format(time.RFC3339Nano),
		Leverage:       10,
		Amount:         trading.NewFlexibleFloat(100),
		Risk: trading.Risk{
			Type:  trading.RiskTPSL,
			TPPct: &tp,
			SLPct: &sl,
		},
	}
	signal.Normalize()
	rec, _, err := store.RecordAccepted(signal, storage.DedupeKey(signal), now)
	if err != nil {
		t.Fatal(err)
	}
	result := trading.OrderResult{
		APIID:          "main",
		TargetExchange: trading.ExchangeBinance,
		InstID:         "BTCUSDT",
		OrdID:          orderID,
		ClOrdID:        "entry-" + orderID,
		OrdType:        "MARKET",
		Leverage:       10,
		RiskOrders: []trading.RiskOrderResult{
			{Exchange: trading.ExchangeBinance, AlgoID: "7001", ClientAlgoID: "entry-" + orderID + "TP", OrderType: "TAKE_PROFIT_MARKET", TriggerPrice: "51000"},
			{Exchange: trading.ExchangeBinance, AlgoID: "7002", ClientAlgoID: "entry-" + orderID + "SL", OrderType: "STOP_MARKET", TriggerPrice: "49500"},
		},
	}
	if err := store.MarkSubmitted(rec.SignalID, result, now); err != nil {
		t.Fatal(err)
	}
	rec, ok := store.Get(rec.SignalID)
	if !ok {
		t.Fatal("submitted order not found")
	}
	return rec
}

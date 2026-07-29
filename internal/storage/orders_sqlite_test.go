package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

func TestSQLiteOrderStoreMigratesLegacyJSONOnce(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "orders.json")
	dbPath := filepath.Join(dir, "tvbot.db")
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	state := orderState{
		Dedupe: map[string]string{"dedupe": "sig-1"},
		Orders: []OrderRecord{{
			SignalID:   "sig-1",
			DedupeKey:  "dedupe",
			Status:     StatusSubmitted,
			Action:     trading.ActionLong,
			APIID:      "default",
			Coinpair:   "BTC",
			Ticker:     "BTCUSDT",
			Price:      "50000",
			Amount:     "100",
			AcceptedAt: now,
			UpdatedAt:  now,
			Result:     trading.OrderResult{InstID: "BTC-USDT-SWAP", OrdID: "123"},
		}},
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLiteOrderStore(dbPath, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.List(10); len(got) != 1 || got[0].SignalID != "sig-1" || got[0].Result.OrdID != "123" {
		t.Fatalf("bad migrated records: %#v", got)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSQLiteOrderStore(dbPath, legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.List(10); len(got) != 1 {
		t.Fatalf("migration should be idempotent, got %d records: %#v", len(got), got)
	}
}

func TestSQLiteOrderStoreRecordDuplicateAndMarkResults(t *testing.T) {
	store, err := NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	tp := trading.NewFlexibleFloat(3)
	sl := trading.NewFlexibleFloat(1.5)
	signal := trading.Signal{
		Action:   trading.ActionShort,
		APIID:    "backup",
		Coinpair: "ETH",
		Price:    trading.NewFlexibleFloat(2500),
		SentAt:   "2026-07-24T03:00:00Z",
		Ticker:   "ETHUSDT",
		Leverage: 3,
		Amount:   trading.NewFlexibleFloat(120),
		Risk: trading.Risk{
			Type:  trading.RiskTPSL,
			TPPct: &tp,
			SLPct: &sl,
		},
		Token:   "token",
		RawJSON: `{"action":"sell","token":"[redacted]"}`,
	}
	signal.Normalize()
	dedupe := DedupeKey(signal)
	rec, duplicate, err := store.RecordAccepted(signal, dedupe, now)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate || rec.Status != StatusAccepted {
		t.Fatalf("first record duplicate=%v rec=%#v", duplicate, rec)
	}
	if err := store.MarkSubmitted(rec.SignalID, trading.OrderResult{InstID: "ETH-USDT-SWAP", OrdID: "okx-1"}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	dup, duplicate, err := store.RecordAccepted(signal, dedupe, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate || dup.Status != StatusDuplicate {
		t.Fatalf("duplicate record duplicate=%v rec=%#v", duplicate, dup)
	}
	if err := store.MarkFailed("missing", errors.New("nope"), now); err == nil {
		t.Fatal("expected missing signal error")
	}
	records := store.List(10)
	if len(records) != 2 || records[0].Status != StatusDuplicate || records[1].Status != StatusSubmitted || records[1].Result.OrdID != "okx-1" {
		t.Fatalf("bad records: %#v", records)
	}
	if records[0].RawJSON != signal.RawJSON || records[1].RawJSON != signal.RawJSON {
		t.Fatalf("raw json should be preserved: %#v", records)
	}
	if records[1].Risk.Type != trading.RiskTPSL || records[1].Risk.TPPct == nil || records[1].Risk.TPPct.Value != 3 || records[1].Risk.SLPct == nil || records[1].Risk.SLPct.Value != 1.5 {
		t.Fatalf("risk settings should be preserved: %#v", records[1].Risk)
	}
}

func TestOrderStoreListByTargetExchangeFiltersMemoryAndSQLite(t *testing.T) {
	run := func(t *testing.T, store *OrderStore) {
		t.Helper()
		now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
		okxSignal := trading.Signal{Action: trading.ActionLong, TargetExchange: trading.ExchangeOKX, Coinpair: "BTC", Ticker: "OKX:BTCUSDT.P"}
		binanceSignal := trading.Signal{Action: trading.ActionShort, TargetExchange: trading.ExchangeBinance, Coinpair: "ETH", Ticker: "BINANCE:ETHUSDT.P"}
		if _, _, err := store.RecordAccepted(okxSignal, "okx", now); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.RecordAccepted(binanceSignal, "binance", now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		okx := store.ListByTargetExchange(trading.ExchangeOKX, 10)
		if len(okx) != 1 || okx[0].TargetExchange != trading.ExchangeOKX || okx[0].Coinpair != "BTC" {
			t.Fatalf("bad OKX filtered orders: %#v", okx)
		}
		binance := store.ListByTargetExchange(trading.ExchangeBinance, 10)
		if len(binance) != 1 || binance[0].TargetExchange != trading.ExchangeBinance || binance[0].Coinpair != "ETH" {
			t.Fatalf("bad Binance filtered orders: %#v", binance)
		}
	}
	t.Run("memory", func(t *testing.T) {
		store, err := NewOrderStore("")
		if err != nil {
			t.Fatal(err)
		}
		run(t, store)
	})
	t.Run("sqlite", func(t *testing.T) {
		store, err := NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		run(t, store)
	})
}

func TestOrderStoreListPageAndCountMemoryAndSQLite(t *testing.T) {
	run := func(t *testing.T, store *OrderStore) {
		t.Helper()
		now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			exchange := trading.ExchangeBinance
			if i%2 == 0 {
				exchange = trading.ExchangeOKX
			}
			signal := trading.Signal{
				Action:         trading.ActionLong,
				TargetExchange: exchange,
				Coinpair:       "COIN" + strconv.Itoa(i),
				Ticker:         "COIN" + strconv.Itoa(i) + "USDT",
			}
			if _, _, err := store.RecordAccepted(signal, "page-"+strconv.Itoa(i), now.Add(time.Duration(i)*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		page := store.ListPage(2, 1)
		if len(page) != 2 || page[0].Coinpair != "COIN3" || page[1].Coinpair != "COIN2" {
			t.Fatalf("bad unfiltered page: %#v", page)
		}
		okxPage := store.ListByTargetExchangePage(trading.ExchangeOKX, 1, 1)
		if len(okxPage) != 1 || okxPage[0].Coinpair != "COIN2" {
			t.Fatalf("bad OKX page: %#v", okxPage)
		}
		if got := store.Count(); got != 5 {
			t.Fatalf("count = %d", got)
		}
		if got := store.CountByTargetExchange(trading.ExchangeOKX); got != 3 {
			t.Fatalf("OKX count = %d", got)
		}
	}
	t.Run("memory", func(t *testing.T) {
		store, err := NewOrderStore("")
		if err != nil {
			t.Fatal(err)
		}
		run(t, store)
	})
	t.Run("sqlite", func(t *testing.T) {
		store, err := NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		run(t, store)
	})
}

func TestSQLiteOrderStoreReadsLegacyRowsWithNullExchangeColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tvbot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE orders (
		signal_id TEXT PRIMARY KEY,
		dedupe_key TEXT NOT NULL,
		status TEXT NOT NULL,
		action TEXT,
		api_id TEXT,
		coinpair TEXT,
		ticker TEXT,
		price TEXT,
		leverage INTEGER,
		amount TEXT,
		token_hash TEXT,
		accepted_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		result_json TEXT,
		error_code TEXT,
		error TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	_, err = db.Exec(`INSERT INTO orders
		(signal_id, dedupe_key, status, action, api_id, coinpair, ticker, price, leverage, amount, token_hash, accepted_at, updated_at, result_json, error_code, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL)`,
		"sig-old",
		"dedupe-old",
		string(StatusAccepted),
		string(trading.ActionLong),
		"default",
		"BTC",
		"BTCUSDT",
		"50000",
		5,
		"100",
		"tokenhash",
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewSQLiteOrderStore(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records := store.List(10)
	if len(records) != 1 || records[0].SignalID != "sig-old" || records[0].TargetExchange != trading.ExchangeOKX {
		t.Fatalf("legacy order should remain readable after exchange migration: %#v", records)
	}
	var sourceExchange, targetExchange, rawJSON, riskJSON string
	if err := store.db.QueryRow(`SELECT source_exchange, target_exchange, raw_json, risk_json FROM orders WHERE signal_id = 'sig-old'`).Scan(&sourceExchange, &targetExchange, &rawJSON, &riskJSON); err != nil {
		t.Fatal(err)
	}
	if sourceExchange != "" || targetExchange != trading.ExchangeOKX || rawJSON != "" || riskJSON != "" {
		t.Fatalf("legacy order columns not backfilled source=%q target=%q raw_json=%q risk_json=%q", sourceExchange, targetExchange, rawJSON, riskJSON)
	}
}

func TestSQLiteAnalysisTables(t *testing.T) {
	store, err := NewSQLiteOrderStore(filepath.Join(t.TempDir(), "tvbot.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	if err := store.UpsertMarketCandles([]MarketCandle{{
		InstID: "USDT-USD",
		Bar:    "1H",
		TS:     now.UnixMilli(),
		Open:   "0.9990",
		Close:  "0.9991",
	}}, now); err != nil {
		t.Fatal(err)
	}
	candles, err := store.ListMarketCandles("USDT-USD", "1H", now.Add(-time.Hour), 72)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 || candles[0].Close != "0.9991" {
		t.Fatalf("bad candles: %#v", candles)
	}
	fill := OKXFill{
		APIID:    "default",
		InstType: "SWAP",
		InstID:   "BTC-USDT-SWAP",
		TradeID:  "trade-1",
		FillPnl:  "2.5",
		Fee:      "-0.1",
		FeeCcy:   "USDT",
		FillTime: now.UnixMilli(),
	}
	if err := store.UpsertOKXFills([]OKXFill{fill, fill}, now); err != nil {
		t.Fatal(err)
	}
	fills, err := store.ListOKXFills("default", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 1 || fills[0].TradeID != "trade-1" {
		t.Fatalf("bad fills: %#v", fills)
	}
	if err := store.UpsertUSDTBalanceSnapshot(USDTBalanceSnapshot{
		APIID:            "default",
		Env:              "demo",
		BucketTS:         now.UnixMilli(),
		ObservedAt:       now,
		TotalEq:          "1001",
		Eq:               "1000",
		EqUsd:            "999.5",
		AvailEq:          "900",
		AvailBal:         "899",
		CashBal:          "950",
		FrozenBal:        "50",
		BalanceUpdatedAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUSDTBalanceSnapshot(USDTBalanceSnapshot{
		APIID:      "default",
		Env:        "demo",
		BucketTS:   now.UnixMilli(),
		ObservedAt: now.Add(10 * time.Minute),
		Eq:         "1002",
		EqUsd:      "1001.5",
	}); err != nil {
		t.Fatal(err)
	}
	snapshots, err := store.ListUSDTBalanceSnapshots("default", "demo", now.Add(-time.Hour), 72)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].EqUsd != "1001.5" || !snapshots[0].ObservedAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("bad USDT balance snapshots: %#v", snapshots)
	}
	minuteObservedAt := now.Add(14*time.Minute + 45*time.Second)
	if err := store.UpsertUSDTBalanceSnapshot(USDTBalanceSnapshot{
		APIID:      "default",
		Env:        "demo",
		ObservedAt: minuteObservedAt,
		Eq:         "1003",
		EqUsd:      "1002.5",
	}); err != nil {
		t.Fatal(err)
	}
	snapshots, err = store.ListUSDTBalanceSnapshots("default", "demo", now.Add(-time.Hour), 72)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 || snapshots[1].BucketTS != minuteObservedAt.UTC().Truncate(time.Minute).UnixMilli() || snapshots[1].EqUsd != "1002.5" {
		t.Fatalf("USDT balance snapshots should bucket by minute: %#v", snapshots)
	}
	if err := store.UpsertUSDTBalanceSnapshot(USDTBalanceSnapshot{
		Exchange:   "binance",
		APIID:      "default",
		Env:        "demo",
		BucketTS:   now.UnixMilli(),
		ObservedAt: now,
		Eq:         "2000",
		EqUsd:      "2000",
	}); err != nil {
		t.Fatal(err)
	}
	okxSnapshots, err := store.ListUSDTBalanceSnapshots("default", "demo", now.Add(-time.Hour), 72)
	if err != nil {
		t.Fatal(err)
	}
	binanceSnapshots, err := store.ListExchangeUSDTBalanceSnapshots("binance", "default", "demo", now.Add(-time.Hour), 72)
	if err != nil {
		t.Fatal(err)
	}
	if len(okxSnapshots) != 2 || okxSnapshots[0].Exchange != "okx" || len(binanceSnapshots) != 1 || binanceSnapshots[0].EqUsd != "2000" {
		t.Fatalf("exchange-specific USDT snapshots not isolated okx=%#v binance=%#v", okxSnapshots, binanceSnapshots)
	}
}

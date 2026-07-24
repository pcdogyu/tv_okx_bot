package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	signal := trading.Signal{
		Action:   trading.ActionShort,
		APIID:    "backup",
		Coinpair: "ETH",
		Price:    trading.NewFlexibleFloat(2500),
		SentAt:   "2026-07-24T03:00:00Z",
		Ticker:   "ETHUSDT",
		Leverage: 3,
		Amount:   trading.NewFlexibleFloat(120),
		Token:    "token",
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
}

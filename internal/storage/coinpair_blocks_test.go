package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCoinpairBlocksPersistDeduplicateExtendAndExpire(t *testing.T) {
	now := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "tvbot.db")
	store, err := NewSQLiteOrderStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	first := CoinpairBlockEvent{
		EventID:      "fill:1",
		Keyword:      "eth",
		Symbol:       "ETHUSDT",
		TriggerPrice: "2500",
		Source:       "exchange_fill",
		Exchange:     "binance",
		APIID:        "main",
		OccurredAt:   now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}
	block, created, err := store.AddCoinpairBlockEvent(first, now)
	if err != nil {
		t.Fatal(err)
	}
	if !created || block.Keyword != "ETH" || block.RemainingSecs != 24*60*60 {
		t.Fatalf("bad first block: created=%v block=%#v", created, block)
	}
	duplicate := first
	duplicate.OccurredAt = now.Add(2 * time.Hour)
	duplicate.ExpiresAt = duplicate.OccurredAt.Add(24 * time.Hour)
	duplicate.TriggerPrice = "2400"
	block, created, err = store.AddCoinpairBlockEvent(duplicate, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || block.TriggerPrice != "2500" || !block.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("duplicate event extended block: created=%v block=%#v", created, block)
	}
	second := duplicate
	second.EventID = "fill:2"
	block, created, err = store.AddCoinpairBlockEvent(second, second.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	if !created || block.TriggerPrice != "2400" || !block.ExpiresAt.Equal(second.ExpiresAt) {
		t.Fatalf("new stop did not extend block: created=%v block=%#v", created, block)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = NewSQLiteOrderStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	blocks, err := store.ListActiveCoinpairBlocks(now.Add(23 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Keyword != "ETH" || blocks[0].TriggerPrice != "2400" {
		t.Fatalf("block did not persist: %#v", blocks)
	}
	blocks, err = store.ListActiveCoinpairBlocks(second.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expired block still active: %#v", blocks)
	}
}

func TestMemoryCoinpairBlocksUseSameExpiryRules(t *testing.T) {
	store, err := NewOrderStore("")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 2, 0, 0, 0, time.UTC)
	event := CoinpairBlockEvent{
		EventID:    "monitor:1",
		Keyword:    "btc",
		Symbol:     "BTC-USDT-SWAP",
		Source:     "position_monitor",
		OccurredAt: now,
		ExpiresAt:  now.Add(time.Hour),
	}
	if _, created, err := store.AddCoinpairBlockEvent(event, now); err != nil || !created {
		t.Fatalf("add memory block: created=%v err=%v", created, err)
	}
	if removed, err := store.DeleteExpiredCoinpairBlocks(now.Add(30 * time.Minute)); err != nil || removed != 0 {
		t.Fatalf("active block removed: count=%d err=%v", removed, err)
	}
	if removed, err := store.DeleteExpiredCoinpairBlocks(event.ExpiresAt); err != nil || removed != 1 {
		t.Fatalf("expired block not removed: count=%d err=%v", removed, err)
	}
}

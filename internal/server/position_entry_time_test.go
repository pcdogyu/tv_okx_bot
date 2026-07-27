package server

import (
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/okx"
)

func TestPositionEntryFillTimeLongAddAndReduce(t *testing.T) {
	entry := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	reduce := entry.Add(time.Hour)
	got, ok, errText := positionEntryFillTime(okx.Position{
		InstID:  "BTC-USDT-SWAP",
		PosSide: "long",
		Pos:     "1",
	}, []positionEntryFill{
		{InstID: "BTC-USDT-SWAP", PosSide: "long", Side: "sell", Size: 0.5, FillTime: reduce},
		{InstID: "BTC-USDT-SWAP", PosSide: "long", Side: "buy", Size: 1.5, FillTime: entry},
	})
	if !ok || !got.Equal(entry) || errText != "" {
		t.Fatalf("entry=%s ok=%v err=%q", got, ok, errText)
	}
}

func TestPositionEntryFillTimeShortAddAndReduce(t *testing.T) {
	entry := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	reduce := entry.Add(time.Hour)
	got, ok, errText := positionEntryFillTime(okx.Position{
		InstID:  "BTC-USDT-SWAP",
		PosSide: "short",
		Pos:     "1",
	}, []positionEntryFill{
		{InstID: "BTC-USDT-SWAP", PosSide: "short", Side: "buy", Size: 0.5, FillTime: reduce},
		{InstID: "BTC-USDT-SWAP", PosSide: "short", Side: "sell", Size: 1.5, FillTime: entry},
	})
	if !ok || !got.Equal(entry) || errText != "" {
		t.Fatalf("entry=%s ok=%v err=%q", got, ok, errText)
	}
}

func TestPositionEntryFillTimeReversalUsesFlipFill(t *testing.T) {
	longEntry := time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)
	flip := longEntry.Add(time.Hour)
	got, ok, errText := positionEntryFillTime(okx.Position{
		InstID:  "BTC-USDT-SWAP",
		PosSide: "net",
		Pos:     "-1",
	}, []positionEntryFill{
		{InstID: "BTC-USDT-SWAP", Side: "sell", Size: 2, FillTime: flip},
		{InstID: "BTC-USDT-SWAP", Side: "buy", Size: 1, FillTime: longEntry},
	})
	if !ok || !got.Equal(flip) || errText != "" {
		t.Fatalf("entry=%s ok=%v err=%q", got, ok, errText)
	}
}

func TestPositionEntryFillTimeInsufficientHistory(t *testing.T) {
	_, ok, errText := positionEntryFillTime(okx.Position{
		InstID:  "BTC-USDT-SWAP",
		PosSide: "long",
		Pos:     "2",
	}, []positionEntryFill{
		{InstID: "BTC-USDT-SWAP", PosSide: "long", Side: "buy", Size: 1, FillTime: time.Date(2026, 7, 24, 1, 0, 0, 0, time.UTC)},
	})
	if ok || !strings.Contains(errText, "成交不足") {
		t.Fatalf("expected insufficient history, ok=%v err=%q", ok, errText)
	}
}

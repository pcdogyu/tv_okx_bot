package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	coinpairCooldownDuration        = 24 * time.Hour
	coinpairCooldownCleanupInterval = time.Minute
)

func (s *Server) StartCoinpairBlockCleaner(ctx context.Context) {
	if s.Orders == nil {
		return
	}
	go s.runCoinpairBlockCleaner(ctx)
}

func (s *Server) runCoinpairBlockCleaner(ctx context.Context) {
	s.cleanExpiredCoinpairBlocks()
	ticker := time.NewTicker(coinpairCooldownCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanExpiredCoinpairBlocks()
		}
	}
}

func (s *Server) cleanExpiredCoinpairBlocks() {
	if s.Orders == nil {
		return
	}
	removed, err := s.Orders.DeleteExpiredCoinpairBlocks(s.now())
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("failed to clean expired coinpair cooldowns", "error", err)
		}
		return
	}
	if removed > 0 && s.Logger != nil {
		s.Logger.Info("expired coinpair cooldowns removed", "count", removed)
	}
}

func (s *Server) handleCoinpairBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order store is not configured")
		return
	}
	now := s.now()
	blocks, err := s.Orders.ListActiveCoinpairBlocks(now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"blocks":     blocks,
		"updated_at": now,
	})
}

func (s *Server) activeCoinpairCooldown(signal trading.Signal, now time.Time) (storage.CoinpairBlock, bool, error) {
	if strings.EqualFold(strings.TrimSpace(signal.PositionEffect), trading.PositionEffectClose) || s.Orders == nil {
		return storage.CoinpairBlock{}, false, nil
	}
	blocks, err := s.Orders.ListActiveCoinpairBlocks(now)
	if err != nil {
		return storage.CoinpairBlock{}, false, err
	}
	for _, block := range blocks {
		filter := normalizeCoinpairFilter(block.Keyword)
		if filter == "" {
			continue
		}
		for _, candidate := range []string{signal.Coinpair, signal.Ticker} {
			if strings.Contains(normalizeCoinpairFilter(candidate), filter) {
				return block, true, nil
			}
		}
	}
	return storage.CoinpairBlock{}, false, nil
}

func (s *Server) recordCoinpairCooldown(eventID, source, exchange, apiID, triggerPrice string, occurredAt time.Time, symbolCandidates ...string) (storage.CoinpairBlock, bool, error) {
	if s.Orders == nil {
		return storage.CoinpairBlock{}, false, nil
	}
	keyword, symbol := coinpairCooldownIdentity(symbolCandidates...)
	if keyword == "" {
		return storage.CoinpairBlock{}, false, fmt.Errorf("cannot derive cooldown keyword from %q", symbolCandidates)
	}
	if occurredAt.IsZero() {
		occurredAt = s.now()
	}
	occurredAt = occurredAt.UTC()
	return s.Orders.AddCoinpairBlockEvent(storage.CoinpairBlockEvent{
		EventID:      eventID,
		Keyword:      keyword,
		Symbol:       symbol,
		TriggerPrice: strings.TrimSpace(triggerPrice),
		Source:       source,
		Exchange:     exchange,
		APIID:        apiID,
		OccurredAt:   occurredAt,
		ExpiresAt:    occurredAt.Add(coinpairCooldownDuration),
	}, s.now())
}

func coinpairCooldownIdentity(values ...string) (string, string) {
	for _, value := range values {
		raw := strings.ToUpper(strings.TrimSpace(value))
		if raw == "" {
			continue
		}
		if colon := strings.LastIndex(raw, ":"); colon >= 0 {
			raw = raw[colon+1:]
		}
		display := raw
		for _, suffix := range []string{".P", "-SWAP", "_SWAP", "SWAP", "-PERP", "_PERP", "PERP"} {
			raw = strings.TrimSuffix(raw, suffix)
		}
		normalized := normalizeCoinpairFilter(raw)
		for _, quote := range []string{"USDT", "USDC", "BUSD", "FDUSD", "USD"} {
			if strings.HasSuffix(normalized, quote) && len(normalized) > len(quote) {
				normalized = strings.TrimSuffix(normalized, quote)
				break
			}
		}
		if normalized != "" {
			return normalized, display
		}
	}
	return "", ""
}

func isStopLossCloseSignal(signal trading.Signal) bool {
	if !strings.EqualFold(strings.TrimSpace(signal.PositionEffect), trading.PositionEffectClose) {
		return false
	}
	primary := strings.ToLower(strings.Join([]string{signal.OrderIntent, signal.Intent}, " "))
	if containsStopLossMarker(primary) {
		return true
	}
	if containsTakeProfitMarker(primary) {
		return false
	}
	fallback := strings.ToLower(strings.Join([]string{signal.Condition, signal.Text}, " "))
	return containsStopLossMarker(fallback) && !containsTakeProfitMarker(fallback)
}

func containsStopLossMarker(text string) bool {
	if strings.Contains(text, "止损") || strings.Contains(text, "止損") || strings.Contains(text, "stop_loss") || strings.Contains(text, "stop-loss") || strings.Contains(text, "stop loss") {
		return true
	}
	for _, token := range tvOrderIntentTokens(text) {
		if token == "sl" || token == "stoploss" {
			return true
		}
	}
	return false
}

func containsTakeProfitMarker(text string) bool {
	if strings.Contains(text, "止盈") || strings.Contains(text, "take_profit") || strings.Contains(text, "take-profit") || strings.Contains(text, "take profit") {
		return true
	}
	for _, token := range tvOrderIntentTokens(text) {
		if token == "tp" || token == "takeprofit" {
			return true
		}
	}
	return false
}

func (s *Server) recordCooldownIgnoredSignal(signal trading.Signal, block storage.CoinpairBlock, now time.Time) (storage.OrderRecord, error) {
	message := fmt.Sprintf("coinpair matched stop-loss cooldown %q until %s", block.Keyword, block.ExpiresAt.UTC().Format(time.RFC3339Nano))
	return s.Orders.RecordIgnoredReason(signal, "coinpair_cooldown", message, now)
}

func coinpairCooldownResponse(record storage.OrderRecord, block storage.CoinpairBlock) map[string]any {
	return map[string]any{
		"status":        "ignored",
		"reason":        "coinpair_cooldown",
		"signal_id":     record.SignalID,
		"keyword":       block.Keyword,
		"blocked_until": block.ExpiresAt,
	}
}

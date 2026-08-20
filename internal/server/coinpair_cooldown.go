package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	coinpairCooldownDuration        = 24 * time.Hour
	coinpairCooldownCleanupInterval = time.Minute
)

var manualCoinpairCooldownSequence atomic.Uint64

type createCoinpairBlockRequest struct {
	Symbol       string `json:"symbol"`
	TriggerPrice string `json:"trigger_price"`
	Exchange     string `json:"exchange"`
	APIID        string `json:"api_id"`
}

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
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order store is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleCoinpairBlockList(w)
	case http.MethodPost:
		s.handleCoinpairBlockCreate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are allowed")
	}
}

func (s *Server) handleCoinpairBlockList(w http.ResponseWriter) {
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

func (s *Server) handleCoinpairBlockCreate(w http.ResponseWriter, r *http.Request) {
	var req createCoinpairBlockRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	req.Symbol = strings.TrimSpace(req.Symbol)
	req.TriggerPrice = strings.TrimSpace(req.TriggerPrice)
	req.Exchange = strings.TrimSpace(req.Exchange)
	req.APIID = strings.TrimSpace(req.APIID)
	if req.Symbol == "" {
		writeError(w, http.StatusBadRequest, "invalid_coinpair_block", "symbol is required")
		return
	}
	if req.Exchange == "" || !trading.ValidTargetExchange(req.Exchange) {
		writeError(w, http.StatusBadRequest, "invalid_coinpair_block", "exchange must be okx or binance")
		return
	}
	req.Exchange = trading.NormalizeExchange(req.Exchange)
	keyword, _ := coinpairCooldownIdentity(req.Symbol)
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "invalid_coinpair_block", "symbol does not contain a usable coinpair")
		return
	}
	now := s.now()
	blocks, err := s.Orders.ListActiveCoinpairBlocks(now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if block, found := coinpairBlockCoveringSymbol(blocks, req.Symbol); found {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "active",
			"block":  block,
		})
		return
	}
	eventID := fmt.Sprintf("analysis_manual:%s:%d:%d", keyword, now.UnixNano(), manualCoinpairCooldownSequence.Add(1))
	block, _, err := s.recordCoinpairCooldown(eventID, "analysis_manual", req.Exchange, req.APIID, req.TriggerPrice, now, req.Symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "created",
		"block":  block,
	})
}

func (s *Server) handleCoinpairBlockDelete(w http.ResponseWriter, r *http.Request, rawKeyword string) {
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order store is not configured")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only DELETE is allowed")
		return
	}
	keyword, _ := coinpairCooldownIdentity(rawKeyword)
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "invalid_coinpair_block", "coinpair block keyword is required")
		return
	}
	removed, err := s.Orders.DeleteCoinpairBlock(keyword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "not_found", "coinpair block was not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "removed",
		"keyword": keyword,
	})
}

func coinpairBlockCoveringSymbol(blocks []storage.CoinpairBlock, symbol string) (storage.CoinpairBlock, bool) {
	candidate := normalizeCoinpairFilter(symbol)
	if candidate == "" {
		return storage.CoinpairBlock{}, false
	}
	for _, block := range blocks {
		filter := normalizeCoinpairFilter(block.Keyword)
		if filter != "" && strings.Contains(candidate, filter) {
			return block, true
		}
	}
	return storage.CoinpairBlock{}, false
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

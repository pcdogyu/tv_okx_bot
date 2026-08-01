package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	tradeMonitorDefaultPollInterval = 20 * time.Second
	tradeMonitorPollTimeout         = 45 * time.Second
	tradeMonitorOrderTimeout        = 20 * time.Second
	tradeMonitorHistoryLimit        = 1000
)

func (s *Server) StartTradeFillMonitor(ctx context.Context) {
	if s.ConfigStore == nil || s.Orders == nil || s.BinanceCredentials == nil {
		return
	}
	go s.runTradeFillMonitor(ctx)
}

func (s *Server) runTradeFillMonitor(ctx context.Context) {
	s.runTradeFillMonitorOnce(ctx)
	ticker := time.NewTicker(tradeMonitorDefaultPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTradeFillMonitorOnce(ctx)
			cfg := s.ConfigStore.Get()
			interval := time.Duration(cfg.Trading.FillMonitor.PollIntervalSeconds) * time.Second
			if interval <= 0 {
				interval = tradeMonitorDefaultPollInterval
			}
			ticker.Reset(interval)
		}
	}
}

func (s *Server) runTradeFillMonitorOnce(ctx context.Context) {
	cfg := s.ConfigStore.Get()
	if !tradeFillMonitorBinanceEnabled(cfg) {
		return
	}
	now := s.now()
	lookback := time.Duration(cfg.Trading.FillMonitor.LookbackHours) * time.Hour
	if lookback <= 0 {
		lookback = 72 * time.Hour
	}
	if err := s.ensureRecentBinanceTradeLifecycles(cfg, now, lookback); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to sync Binance trade lifecycles", "error", err)
	}
	symbols := s.tradeMonitorBinanceSymbols(cfg, now, lookback)
	if len(symbols) == 0 {
		return
	}
	for _, requestedAPIID := range configuredBinanceAPIIDs(s.BinanceCredentials.Status()) {
		client, apiID, err := s.binanceClientForCredentials(cfg, requestedAPIID)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn("failed to create Binance trade monitor client", "api_id", requestedAPIID, "error", err)
			}
			continue
		}
		for _, symbol := range symbols {
			pollCtx, cancel := context.WithTimeout(ctx, tradeMonitorPollTimeout)
			err := s.pollBinanceSymbolFills(pollCtx, cfg, client, apiID, symbol, now, lookback)
			cancel()
			if err != nil && s.Logger != nil {
				s.Logger.Warn("failed to poll Binance fills", "api_id", apiID, "symbol", symbol, "error", err)
			}
		}
	}
}

func tradeFillMonitorBinanceEnabled(cfg config.Config) bool {
	if !cfg.Trading.FillMonitor.Enabled {
		return false
	}
	for _, exchange := range cfg.Trading.FillMonitor.Exchanges {
		if trading.NormalizeExchange(exchange) == trading.ExchangeBinance {
			return true
		}
	}
	return false
}

func (s *Server) ensureRecentBinanceTradeLifecycles(cfg config.Config, now time.Time, lookback time.Duration) error {
	if s.Orders == nil {
		return nil
	}
	cutoff := now.Add(-lookback)
	for _, rec := range s.Orders.List(10000) {
		if rec.UpdatedAt.Before(cutoff) && rec.AcceptedAt.Before(cutoff) {
			continue
		}
		if rec.Status != storage.StatusSubmitted || trading.NormalizeExchange(rec.TargetExchange) != trading.ExchangeBinance {
			continue
		}
		if _, err := s.Orders.UpsertTradeLifecycleFromOrder(rec, "", 0, now); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to upsert Binance trade lifecycle", "signal_id", rec.SignalID, "error", err)
		}
	}
	return nil
}

func (s *Server) tradeMonitorBinanceSymbols(cfg config.Config, now time.Time, lookback time.Duration) []string {
	seen := map[string]bool{}
	add := func(symbol string) {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || seen[symbol] {
			return
		}
		if !analysisBinanceSymbolSupported(symbol) {
			return
		}
		seen[symbol] = true
	}
	for key, sym := range cfg.Symbols {
		if symbol, ok := deriveBinanceAnalysisSymbol(sym.Coinpair, key, sym.InstID); ok {
			add(symbol)
		}
	}
	if s.Orders != nil {
		cutoff := now.Add(-lookback)
		for _, rec := range s.Orders.List(10000) {
			if rec.UpdatedAt.Before(cutoff) && rec.AcceptedAt.Before(cutoff) {
				continue
			}
			if trading.NormalizeExchange(rec.TargetExchange) != trading.ExchangeBinance {
				continue
			}
			if symbol, ok := deriveBinanceAnalysisSymbol(rec.Result.InstID, rec.Coinpair, rec.Ticker); ok {
				add(symbol)
			}
		}
	}
	symbols := make([]string, 0, len(seen))
	for symbol := range seen {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

func (s *Server) pollBinanceSymbolFills(ctx context.Context, cfg config.Config, client binance.Client, apiID, symbol string, now time.Time, lookback time.Duration) error {
	cp, found, err := s.Orders.TradeMonitorCheckpoint(trading.ExchangeBinance, apiID, symbol)
	if err != nil {
		return err
	}
	start := now.Add(-lookback)
	if found && cp.LastFillTime > 0 {
		start = time.UnixMilli(cp.LastFillTime).Add(-time.Second)
	}
	trades, err := client.UserTrades(ctx, symbol, start, now.UTC(), tradeMonitorHistoryLimit)
	if err != nil {
		_ = s.Orders.UpsertTradeMonitorCheckpoint(storage.TradeMonitorCheckpoint{
			Exchange:     trading.ExchangeBinance,
			APIID:        apiID,
			Symbol:       symbol,
			LastFillTime: cp.LastFillTime,
			LastTradeID:  cp.LastTradeID,
			LastPolledAt: now,
			LastError:    err.Error(),
			UpdatedAt:    now,
		})
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime: now,
			Exchange:  trading.ExchangeBinance,
			APIID:     apiID,
			Symbol:    symbol,
			EventType: "poll_error",
			Message:   err.Error(),
		})
		return err
	}
	fills := make([]storage.BinanceFill, 0, len(trades))
	for _, trade := range trades {
		fills = append(fills, storage.BinanceFill{
			APIID:           apiID,
			Symbol:          trade.Symbol,
			TradeID:         strconv.FormatInt(trade.ID, 10),
			OrderID:         strconv.FormatInt(trade.OrderID, 10),
			Side:            trade.Side,
			PositionSide:    trade.PositionSide,
			Price:           trade.Price,
			Qty:             trade.Qty,
			QuoteQty:        trade.QuoteQty,
			RealizedPnl:     trade.RealizedPnl,
			Commission:      trade.Commission,
			CommissionAsset: trade.CommissionAsset,
			Buyer:           trade.Buyer,
			Maker:           trade.Maker,
			FillTime:        trade.Time,
		})
	}
	inserted, err := s.Orders.UpsertBinanceFills(fills, now)
	if err != nil {
		return err
	}
	sort.Slice(inserted, func(i, j int) bool {
		if inserted[i].FillTime == inserted[j].FillTime {
			return inserted[i].TradeID < inserted[j].TradeID
		}
		return inserted[i].FillTime < inserted[j].FillTime
	})
	for _, fill := range inserted {
		if err := s.processBinanceFill(ctx, cfg, client, fill, now); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to process Binance fill", "api_id", apiID, "symbol", symbol, "trade_id", fill.TradeID, "error", err)
		}
	}
	lastFillTime := cp.LastFillTime
	lastTradeID := cp.LastTradeID
	for _, fill := range fills {
		if fill.FillTime > lastFillTime || (fill.FillTime == lastFillTime && fill.TradeID > lastTradeID) {
			lastFillTime = fill.FillTime
			lastTradeID = fill.TradeID
		}
	}
	if err := s.Orders.UpsertTradeMonitorCheckpoint(storage.TradeMonitorCheckpoint{
		Exchange:     trading.ExchangeBinance,
		APIID:        apiID,
		Symbol:       symbol,
		LastFillTime: lastFillTime,
		LastTradeID:  lastTradeID,
		LastPolledAt: now,
		LastError:    "",
		UpdatedAt:    now,
	}); err != nil {
		return err
	}
	if len(trades) >= tradeMonitorHistoryLimit {
		return fmt.Errorf("Binance %s userTrades reached %d limit; trade monitor history may be incomplete", symbol, tradeMonitorHistoryLimit)
	}
	return nil
}

func (s *Server) processBinanceFill(ctx context.Context, cfg config.Config, client binance.Client, fill storage.BinanceFill, now time.Time) error {
	filter := storage.TradeMonitorFilter{
		Exchange: trading.ExchangeBinance,
		APIID:    fill.APIID,
		Symbol:   fill.Symbol,
	}
	lifecycles, err := s.Orders.ListTradeLifecycles(filter, 1000)
	if err != nil {
		return err
	}
	if lifecycle, ok := matchingEntryLifecycle(lifecycles, fill); ok {
		updated, err := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:       storage.TradeLifecycleOpen,
			EntryPrice:   fill.Price,
			EntryQty:     fill.Qty,
			LastFillTime: fill.FillTime,
			UpdatedAt:    now,
		})
		if err != nil {
			return err
		}
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime:      now,
			Exchange:       trading.ExchangeBinance,
			APIID:          fill.APIID,
			Symbol:         fill.Symbol,
			LifecycleID:    updated.LifecycleID,
			SourceSignalID: updated.SourceSignalID,
			EventType:      "lifecycle_open",
			Status:         updated.Status,
			Message:        "Binance entry fill detected",
			RawJSON:        storage.TradeMonitorEventRawJSON(fill),
		})
		return nil
	}
	if lifecycle, status, ok := matchingExitLifecycle(lifecycles, fill); ok {
		updated, err := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:       status,
			ExitPrice:    fill.Price,
			ExitQty:      fill.Qty,
			RealizedPnl:  fill.RealizedPnl,
			LastFillTime: fill.FillTime,
			UpdatedAt:    now,
		})
		if err != nil {
			return err
		}
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime:      now,
			Exchange:       trading.ExchangeBinance,
			APIID:          fill.APIID,
			Symbol:         fill.Symbol,
			LifecycleID:    updated.LifecycleID,
			SourceSignalID: updated.SourceSignalID,
			EventType:      "lifecycle_exit",
			Status:         updated.Status,
			Message:        "Binance exit fill classified by realized PnL",
			RawJSON:        storage.TradeMonitorEventRawJSON(fill),
		})
		if status == storage.TradeLifecycleSLHit {
			return s.maybeSubmitAutoReentry(ctx, cfg, client, updated, now)
		}
		return nil
	}
	return nil
}

func matchingEntryLifecycle(lifecycles []storage.TradeLifecycle, fill storage.BinanceFill) (storage.TradeLifecycle, bool) {
	for _, lifecycle := range lifecycles {
		if lifecycle.Status != storage.TradeLifecycleEntryPending {
			continue
		}
		if storage.MonitorListContains(lifecycle.EntryOrderIDs, fill.OrderID) && strings.EqualFold(fill.Side, entrySide(lifecycle.Action)) {
			return lifecycle, true
		}
	}
	return storage.TradeLifecycle{}, false
}

func matchingExitLifecycle(lifecycles []storage.TradeLifecycle, fill storage.BinanceFill) (storage.TradeLifecycle, string, bool) {
	for _, lifecycle := range lifecycles {
		if lifecycle.Status != storage.TradeLifecycleOpen {
			continue
		}
		if !strings.EqualFold(fill.Side, exitSide(lifecycle.Action)) {
			continue
		}
		pnl, ok := storage.ParseMonitorFloat(fill.RealizedPnl)
		if !ok || math.Abs(pnl) < 1e-12 {
			return lifecycle, storage.TradeLifecycleExited, true
		}
		if pnl < 0 {
			return lifecycle, storage.TradeLifecycleSLHit, true
		}
		return lifecycle, storage.TradeLifecycleTPHit, true
	}
	return storage.TradeLifecycle{}, "", false
}

func entrySide(action string) string {
	switch trading.Side(strings.ToLower(strings.TrimSpace(action))) {
	case trading.ActionShort:
		return "SELL"
	default:
		return "BUY"
	}
}

func exitSide(action string) string {
	switch trading.Side(strings.ToLower(strings.TrimSpace(action))) {
	case trading.ActionShort:
		return "BUY"
	default:
		return "SELL"
	}
}

func (s *Server) maybeSubmitAutoReentry(ctx context.Context, cfg config.Config, client binance.Client, lifecycle storage.TradeLifecycle, now time.Time) error {
	reentry := cfg.Trading.AutoReentry
	if !reentry.Enabled {
		return nil
	}
	if s.Executor == nil {
		_, _ = s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:    storage.TradeLifecycleBlocked,
			UpdatedAt: now,
		})
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime:      now,
			Exchange:       trading.ExchangeBinance,
			APIID:          lifecycle.APIID,
			Symbol:         lifecycle.Symbol,
			LifecycleID:    lifecycle.LifecycleID,
			SourceSignalID: lifecycle.SourceSignalID,
			EventType:      "auto_reentry_failed",
			Status:         storage.TradeLifecycleBlocked,
			Message:        "executor is not configured",
		})
		return nil
	}
	if reentry.OnlyBotOrders && strings.TrimSpace(lifecycle.SourceSignalID) == "" {
		return nil
	}
	if lifecycle.ReentryCount >= reentry.MaxReentries {
		cooldownUntil := now.Add(time.Duration(reentry.CooldownAfterStopHours) * time.Hour).UTC().Format(time.RFC3339Nano)
		updated, err := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:        storage.TradeLifecycleCooldown,
			CooldownUntil: cooldownUntil,
			UpdatedAt:     now,
		})
		if err != nil {
			return err
		}
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime:      now,
			Exchange:       lifecycle.Exchange,
			APIID:          lifecycle.APIID,
			Symbol:         lifecycle.Symbol,
			LifecycleID:    lifecycle.LifecycleID,
			SourceSignalID: lifecycle.SourceSignalID,
			EventType:      "auto_reentry_cooldown",
			Status:         updated.Status,
			Message:        "max auto reentries reached",
		})
		return nil
	}
	positions, err := client.Positions(ctx, lifecycle.Symbol)
	if err != nil {
		return err
	}
	if binanceHasOpenPosition(positions, lifecycle.Symbol) {
		updated, err := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:    storage.TradeLifecycleBlocked,
			UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
			EventTime:      now,
			Exchange:       lifecycle.Exchange,
			APIID:          lifecycle.APIID,
			Symbol:         lifecycle.Symbol,
			LifecycleID:    lifecycle.LifecycleID,
			SourceSignalID: lifecycle.SourceSignalID,
			EventType:      "auto_reentry_blocked",
			Status:         updated.Status,
			Message:        "existing Binance position detected before auto reentry",
		})
		return nil
	}
	rec, ok := s.Orders.Get(lifecycle.SourceSignalID)
	if !ok {
		return fmt.Errorf("source signal %s not found for auto reentry", lifecycle.SourceSignalID)
	}
	signal, execCfg, err := s.autoReentrySignal(ctx, cfg, client, rec, lifecycle, now)
	if err != nil {
		return err
	}
	record, duplicate, err := s.Orders.RecordAccepted(signal, storage.RetryKey(lifecycle.SourceSignalID, signal, now), now)
	if err != nil {
		return err
	}
	if duplicate {
		return nil
	}
	orderCtx, cancel := context.WithTimeout(ctx, tradeMonitorOrderTimeout)
	result, err := s.Executor.ExecuteSignal(orderCtx, signal, execCfg)
	cancel()
	result.SignalID = record.SignalID
	if err != nil {
		_ = s.Orders.MarkFailed(record.SignalID, err, now)
		updated, updateErr := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
			Status:    storage.TradeLifecycleBlocked,
			UpdatedAt: now,
		})
		if updateErr == nil {
			s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
				EventTime:      now,
				Exchange:       lifecycle.Exchange,
				APIID:          lifecycle.APIID,
				Symbol:         lifecycle.Symbol,
				LifecycleID:    lifecycle.LifecycleID,
				SourceSignalID: lifecycle.SourceSignalID,
				EventType:      "auto_reentry_failed",
				Status:         updated.Status,
				Message:        err.Error(),
			})
		}
		return err
	}
	if err := s.Orders.MarkSubmitted(record.SignalID, result, now); err != nil {
		return err
	}
	submitted, ok := s.Orders.Get(record.SignalID)
	if ok {
		if _, err := s.Orders.UpsertTradeLifecycleFromOrder(submitted, lifecycle.LifecycleID, lifecycle.ReentryCount+1, now); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to create auto reentry lifecycle", "signal_id", submitted.SignalID, "parent_lifecycle_id", lifecycle.LifecycleID, "error", err)
		}
	}
	updated, err := s.Orders.UpdateTradeLifecycle(lifecycle.LifecycleID, storage.TradeLifecycleUpdate{
		Status:          storage.TradeLifecycleReentrySubmitted,
		ReentrySignalID: record.SignalID,
		UpdatedAt:       now,
	})
	if err != nil {
		return err
	}
	s.recordTradeMonitorEvent(storage.TradeMonitorEvent{
		EventTime:      now,
		Exchange:       lifecycle.Exchange,
		APIID:          lifecycle.APIID,
		Symbol:         lifecycle.Symbol,
		LifecycleID:    lifecycle.LifecycleID,
		SourceSignalID: lifecycle.SourceSignalID,
		EventType:      "auto_reentry_submitted",
		Status:         updated.Status,
		Message:        "auto reentry order submitted: " + record.SignalID,
		RawJSON:        storage.TradeMonitorEventRawJSON(result),
	})
	return nil
}

func binanceHasOpenPosition(positions []binance.Position, symbol string) bool {
	for _, position := range positions {
		if !strings.EqualFold(position.Symbol, symbol) {
			continue
		}
		amt, err := strconv.ParseFloat(strings.TrimSpace(position.PositionAmt), 64)
		if err == nil && math.Abs(amt) > 1e-12 {
			return true
		}
	}
	return false
}

func (s *Server) autoReentrySignal(ctx context.Context, cfg config.Config, client binance.Client, rec storage.OrderRecord, lifecycle storage.TradeLifecycle, now time.Time) (trading.Signal, config.Config, error) {
	amount := cfg.Trading.OrderAmountUSDT
	if strings.TrimSpace(rec.Amount) != "" {
		parsed, err := parseOrderRecordFloat("amount", rec.Amount)
		if err != nil {
			return trading.Signal{}, cfg, err
		}
		amount = parsed
	}
	amount *= cfg.Trading.AutoReentry.ReentryAmountPct / 100
	if amount <= 0 {
		return trading.Signal{}, cfg, fmt.Errorf("auto reentry amount is not positive")
	}
	ticker, err := client.BookTicker(ctx, lifecycle.Symbol)
	if err != nil {
		return trading.Signal{}, cfg, err
	}
	price, err := binanceBookMidPrice(ticker)
	if err != nil {
		return trading.Signal{}, cfg, err
	}
	rawJSON, _ := json.Marshal(map[string]any{
		"source":              "server_fill_monitor",
		"source_signal_id":    lifecycle.SourceSignalID,
		"source_lifecycle_id": lifecycle.LifecycleID,
		"created_at":          now.UTC().Format(time.RFC3339Nano),
	})
	signal := trading.Signal{
		Action:         rec.Action,
		APIID:          lifecycle.APIID,
		TargetExchange: trading.ExchangeBinance,
		TradeEnv:       orderRecordTradeEnv(rec),
		Coinpair:       firstNonEmptyString(rec.Coinpair, lifecycle.Symbol),
		Ticker:         firstNonEmptyString(rec.Ticker, lifecycle.Symbol),
		Exchange:       "server_fill_monitor",
		Price:          trading.NewFlexibleFloat(price),
		SentAt:         now.UTC().Format(time.RFC3339Nano),
		Leverage:       rec.Leverage,
		Amount:         trading.NewFlexibleFloat(amount),
		Risk:           rec.Risk,
		RawJSON:        string(rawJSON),
	}
	signal.Normalize()
	execCfg := retryExecutionConfig(cfg, rec)
	if err := signal.Validate(now, 0, execCfg); err != nil {
		return trading.Signal{}, cfg, err
	}
	return signal, execCfg, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) recordTradeMonitorEvent(event storage.TradeMonitorEvent) {
	if s.Orders == nil {
		return
	}
	if err := s.Orders.InsertTradeMonitorEvent(event); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to record trade monitor event", "event_type", event.EventType, "error", err)
	}
}

func (s *Server) handleTradeMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	if s.ConfigStore == nil || s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "trade monitor dependencies are not configured")
		return
	}
	cfg := s.ConfigStore.Get()
	filter := storage.TradeMonitorFilter{
		Exchange: r.URL.Query().Get("exchange"),
		APIID:    r.URL.Query().Get("api_id"),
		Symbol:   r.URL.Query().Get("symbol"),
		Status:   r.URL.Query().Get("status"),
	}
	checkpoints, cpErr := s.Orders.ListTradeMonitorCheckpoints(filter, 200)
	lifecycles, lcErr := s.Orders.ListTradeLifecycles(filter, 200)
	events, evErr := s.Orders.ListTradeMonitorEvents(filter, 100)
	errors := []string{}
	for _, err := range []error{cpErr, lcErr, evErr} {
		if err != nil {
			errors = append(errors, err.Error())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           len(errors) == 0,
		"running":      tradeFillMonitorBinanceEnabled(cfg),
		"exchange":     trading.ExchangeBinance,
		"fill_monitor": cfg.Trading.FillMonitor,
		"auto_reentry": cfg.Trading.AutoReentry,
		"checkpoints":  checkpoints,
		"lifecycles":   lifecycles,
		"events":       events,
		"errors":       errors,
		"updated_at":   s.now(),
	})
}

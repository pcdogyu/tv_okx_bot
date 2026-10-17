package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	batchOrderStatusSubmitted = "submitted"
	batchOrderStatusFailed    = "failed"
)

func (s *Server) executeBatch(signalID string, signal trading.Signal, cfg config.Config) {
	cfg.Normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	legs := cfg.Trading.BatchOrders.LegsForSide(signal.Action)
	batch := &trading.BatchOrderResult{
		Total: len(legs),
		Legs:  make([]trading.BatchLegResult, 0, len(legs)),
	}
	aggregate := trading.OrderResult{
		SignalID:       signalID,
		PositionEffect: signal.PositionEffect,
		PositionSide:   signal.PositionSide,
		Batch:          batch,
	}

	for i, leg := range legs {
		legIndex := i + 1
		legSignal := signal
		legSignal.Amount = trading.NewFlexibleFloat(signal.Amount.Value * leg.AmountPct / 100)
		legSignal.Execution = &trading.SignalExecution{BatchIndex: legIndex}
		legResult := trading.BatchLegResult{
			Index:       legIndex,
			Type:        leg.Type,
			DistancePct: leg.DistancePct,
			AmountPct:   leg.AmountPct,
			AmountUSDT:  trading.NormalizeFloat(legSignal.Amount.Value),
		}

		switch leg.Type {
		case config.BatchOrderTypeMarket:
			legSignal.Execution.OrderType = trading.OrderTypeMarket
		case config.BatchOrderTypeSignalLimit:
			legSignal.Execution.OrderType = trading.OrderTypeLimit
			legSignal.Execution.LimitPrice = signal.Price
		case config.BatchOrderTypeCurrentOffsetLimit:
			legSignal.Execution.OrderType = trading.OrderTypeLimit
			legSignal.Execution.CurrentPriceOffsetPct = leg.DistancePct
		default:
			batch.FailedIndex = legIndex
			legResult.Status = batchOrderStatusFailed
			legResult.Error = fmt.Sprintf("unsupported batch order type %q", leg.Type)
			batch.Legs = append(batch.Legs, legResult)
			failed := fmt.Errorf("batch order leg %d: %s", legIndex, legResult.Error)
			_ = s.Orders.MarkFailedWithResult(signalID, batchFailureCode(batch), aggregate, failed, s.now())
			return
		}

		result, err := s.Executor.ExecuteSignal(ctx, legSignal, cfg)
		mergeBatchOrderResult(&aggregate, result)
		legResult.ClOrdID = result.ClOrdID
		legResult.OrdID = result.OrdID
		legResult.OrdType = result.OrdType
		legResult.Px = result.Px
		legResult.RiskOrders = result.RiskOrders
		if err != nil {
			legResult.Status = batchOrderStatusFailed
			legResult.Error = err.Error()
			batch.FailedIndex = legIndex
			batch.Legs = append(batch.Legs, legResult)
			failed := fmt.Errorf("batch order leg %d failed: %w", legIndex, err)
			if storeErr := s.Orders.MarkFailedWithResult(signalID, batchFailureCode(batch), aggregate, failed, s.now()); storeErr != nil && s.Logger != nil {
				s.Logger.Error("failed to persist batch order failure", "signal_id", signalID, "error", storeErr)
			}
			if s.Logger != nil {
				s.Logger.Error("batch order failed", "signal_id", signalID, "failed_index", legIndex, "submitted", batch.Submitted, "error", err)
			}
			return
		}

		legResult.Status = batchOrderStatusSubmitted
		batch.Submitted++
		batch.Legs = append(batch.Legs, legResult)
	}

	if err := s.Orders.MarkSubmitted(signalID, aggregate, s.now()); err != nil && s.Logger != nil {
		s.Logger.Error("failed to persist batch order result", "signal_id", signalID, "error", err)
	}
}

func batchFailureCode(batch *trading.BatchOrderResult) string {
	if batch != nil && batch.Submitted > 0 {
		return "batch_partial"
	}
	return "batch_failed"
}

func mergeBatchOrderResult(aggregate *trading.OrderResult, result trading.OrderResult) {
	if aggregate.APIID == "" {
		aggregate.APIID = result.APIID
	}
	if aggregate.TargetExchange == "" {
		aggregate.TargetExchange = result.TargetExchange
	}
	if aggregate.InstID == "" {
		aggregate.InstID = result.InstID
	}
	if aggregate.OrdType == "" {
		aggregate.OrdType = result.OrdType
	}
	if aggregate.Px == "" {
		aggregate.Px = result.Px
	}
	if aggregate.Leverage == 0 {
		aggregate.Leverage = result.Leverage
	}
	aggregate.ClOrdID = appendBatchIDs(aggregate.ClOrdID, result.ClOrdID)
	aggregate.OrdID = appendBatchIDs(aggregate.OrdID, result.OrdID)
	aggregate.RiskOrders = append(aggregate.RiskOrders, result.RiskOrders...)
	if aggregate.OKXCode == "" {
		aggregate.OKXCode = result.OKXCode
	}
	if aggregate.OKXMsg == "" {
		aggregate.OKXMsg = result.OKXMsg
	}
	if aggregate.BinanceCode == 0 {
		aggregate.BinanceCode = result.BinanceCode
	}
	if aggregate.BinanceMsg == "" {
		aggregate.BinanceMsg = result.BinanceMsg
	}
}

func appendBatchIDs(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	switch {
	case existing == "":
		return next
	case next == "":
		return existing
	default:
		return existing + " / " + next
	}
}

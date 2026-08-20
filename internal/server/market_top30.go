package server

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const marketTopSymbolLimit = 30

type marketTop30Decision struct {
	Exchange  string
	TradeEnv  string
	Symbol    string
	Available bool
	Allowed   bool
}

func applyTop30Rankings(resp *symbolsResponse) {
	if resp == nil {
		return
	}
	resp.OKX.Live.TopInstruments = topOKXInstruments(resp.OKX.Live.Instruments, marketTopSymbolLimit)
	resp.OKX.Demo.TopInstruments = topOKXInstruments(resp.OKX.Demo.Instruments, marketTopSymbolLimit)
	resp.Binance.Live.TopInstruments = topBinanceInstruments(resp.Binance.Live.Instruments, marketTopSymbolLimit)
	resp.Binance.Demo.TopInstruments = topBinanceInstruments(resp.Binance.Demo.Instruments, marketTopSymbolLimit)
}

func markUnavailableFetchedRankings(resp *symbolsResponse) {
	if resp == nil {
		return
	}
	markOKXRankingUnavailable(&resp.OKX.Live)
	markOKXRankingUnavailable(&resp.OKX.Demo)
	markBinanceRankingUnavailable(&resp.Binance.Live)
	markBinanceRankingUnavailable(&resp.Binance.Demo)
}

func markOKXRankingUnavailable(set *okxInstrumentSet) {
	if set == nil || set.Error != "" || set.TickerError != "" || rankingSizeComplete(set.Count, len(set.TopInstruments)) {
		return
	}
	set.TickerError = fmt.Sprintf("top 30 ranking unavailable: got %d ranked symbols", len(set.TopInstruments))
}

func markBinanceRankingUnavailable(set *binanceInstrumentSet) {
	if set == nil || set.Error != "" || set.TickerError != "" || rankingSizeComplete(set.Count, len(set.TopInstruments)) {
		return
	}
	set.TickerError = fmt.Sprintf("top 30 ranking unavailable: got %d ranked symbols", len(set.TopInstruments))
}

func rankingSizeComplete(catalogCount, rankedCount int) bool {
	required := catalogCount
	if required > marketTopSymbolLimit {
		required = marketTopSymbolLimit
	}
	return required > 0 && rankedCount >= required
}

func topOKXInstruments(in []symbolInstrument, limit int) []symbolInstrument {
	eligible := make([]symbolInstrument, 0, len(in))
	for _, instrument := range in {
		turnover, ok := positiveTurnover(instrument.TurnoverUSDT24h)
		if !ok || !strings.EqualFold(strings.TrimSpace(instrument.State), "live") {
			continue
		}
		instrument.TurnoverUSDT24h = trading.NormalizeFloat(turnover)
		eligible = append(eligible, instrument)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, _ := positiveTurnover(eligible[i].TurnoverUSDT24h)
		right, _ := positiveTurnover(eligible[j].TurnoverUSDT24h)
		if left == right {
			return strings.Compare(strings.ToUpper(eligible[i].InstID), strings.ToUpper(eligible[j].InstID)) < 0
		}
		return left > right
	})
	return limitOKXInstruments(eligible, limit)
}

func topBinanceInstruments(in []binanceSymbolInstrument, limit int) []binanceSymbolInstrument {
	eligible := make([]binanceSymbolInstrument, 0, len(in))
	for _, instrument := range in {
		turnover, ok := positiveTurnover(instrument.TurnoverUSDT24h)
		if !ok || !strings.EqualFold(strings.TrimSpace(instrument.Status), "TRADING") {
			continue
		}
		instrument.TurnoverUSDT24h = trading.NormalizeFloat(turnover)
		eligible = append(eligible, instrument)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, _ := positiveTurnover(eligible[i].TurnoverUSDT24h)
		right, _ := positiveTurnover(eligible[j].TurnoverUSDT24h)
		if left == right {
			return strings.Compare(strings.ToUpper(eligible[i].Symbol), strings.ToUpper(eligible[j].Symbol)) < 0
		}
		return left > right
	})
	return limitBinanceInstruments(eligible, limit)
}

func positiveTurnover(raw string) (float64, bool) {
	value, ok := parseAnyFloat(raw)
	return value, ok && value > 0
}

func limitOKXInstruments(in []symbolInstrument, limit int) []symbolInstrument {
	if limit <= 0 || len(in) == 0 {
		return []symbolInstrument{}
	}
	if len(in) > limit {
		in = in[:limit]
	}
	return append([]symbolInstrument(nil), in...)
}

func limitBinanceInstruments(in []binanceSymbolInstrument, limit int) []binanceSymbolInstrument {
	if limit <= 0 || len(in) == 0 {
		return []binanceSymbolInstrument{}
	}
	if len(in) > limit {
		in = in[:limit]
	}
	return append([]binanceSymbolInstrument(nil), in...)
}

func (s *Server) marketTop30Decision(signal trading.Signal) (marketTop30Decision, error) {
	exchange := trading.NormalizeExchange(signal.TargetExchange)
	tradeEnv := trading.NormalizeTradeEnv(signal.TradeEnv)
	if tradeEnv == "" {
		tradeEnv = trading.TradeEnvDemo
	}
	decision := marketTop30Decision{Exchange: exchange, TradeEnv: tradeEnv}
	if s.ConfigStore == nil || s.Orders == nil {
		return decision, fmt.Errorf("top 30 dependencies are not configured")
	}
	resp, err := s.cachedSymbolsResponse(s.ConfigStore.Get())
	if err != nil {
		return decision, err
	}
	applyTop30Rankings(&resp)
	candidates := marketSymbolCandidates(signal.Coinpair, signal.Ticker)
	switch exchange {
	case trading.ExchangeBinance:
		set := resp.Binance.Demo
		if tradeEnv == trading.TradeEnvLive {
			set = resp.Binance.Live
		}
		decision.Available = set.SyncedAt != "" && len(set.TopInstruments) > 0
		for _, instrument := range set.TopInstruments {
			base := marketSymbolBase(instrument.BaseAsset, instrument.Symbol)
			if candidates[base] {
				decision.Allowed = true
				decision.Symbol = instrument.Symbol
				break
			}
		}
	case trading.ExchangeOKX:
		set := resp.OKX.Demo
		if tradeEnv == trading.TradeEnvLive {
			set = resp.OKX.Live
		}
		decision.Available = set.SyncedAt != "" && len(set.TopInstruments) > 0
		for _, instrument := range set.TopInstruments {
			base := marketSymbolBase(instrument.BaseCcy, instrument.InstID)
			if candidates[base] {
				decision.Allowed = true
				decision.Symbol = instrument.InstID
				break
			}
		}
	default:
		return decision, fmt.Errorf("unsupported target exchange %q", exchange)
	}
	return decision, nil
}

func marketSymbolCandidates(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		keyword, _ := coinpairCooldownIdentity(value)
		if keyword != "" {
			out[keyword] = true
		}
	}
	return out
}

func marketSymbolBase(base, instrument string) string {
	keyword, _ := coinpairCooldownIdentity(base, instrument)
	return keyword
}

func (s *Server) recordTop30IgnoredSignal(signal trading.Signal, decision marketTop30Decision, now time.Time) (storage.OrderRecord, error) {
	message := fmt.Sprintf("coinpair is outside %s %s turnover top %d", decision.Exchange, decision.TradeEnv, marketTopSymbolLimit)
	return s.Orders.RecordIgnoredReason(signal, "outside_market_top30", message, now)
}

func top30IgnoredResponse(record storage.OrderRecord, decision marketTop30Decision) map[string]any {
	return map[string]any{
		"status":          "ignored",
		"reason":          "outside_market_top30",
		"signal_id":       record.SignalID,
		"target_exchange": decision.Exchange,
		"trade_env":       decision.TradeEnv,
	}
}

func top30UnavailableMessage(decision marketTop30Decision) string {
	return fmt.Sprintf("%s %s top %d ranking is unavailable", decision.Exchange, decision.TradeEnv, marketTopSymbolLimit)
}

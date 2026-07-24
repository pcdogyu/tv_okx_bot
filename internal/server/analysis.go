package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
)

const (
	analysisPriceInstID = "USDT-USD"
	analysisPriceBar    = "1H"
	defaultPriceDays    = 3
	defaultPNLDays      = 30
	analysisCacheTTL    = 60 * time.Second
)

type analysisResponse struct {
	OK          bool                  `json:"ok"`
	APIID       string                `json:"api_id"`
	PriceDays   int                   `json:"price_days"`
	PNLDays     int                   `json:"pnl_days"`
	PriceInstID string                `json:"price_inst_id"`
	PriceBar    string                `json:"price_bar"`
	RefreshedAt time.Time             `json:"refreshed_at"`
	Cache       analysisCacheStatus   `json:"cache"`
	Source      analysisSourceStatus  `json:"source"`
	PricePoints []analysisPricePoint  `json:"price_points"`
	Summary     analysisSymbolStats   `json:"summary"`
	Symbols     []analysisSymbolStats `json:"symbols"`
}

type analysisCacheStatus struct {
	Hit      bool      `json:"hit"`
	Stale    bool      `json:"stale"`
	CachedAt time.Time `json:"cached_at,omitempty"`
	CacheKey string    `json:"cache_key"`
}

type analysisSourceStatus struct {
	Price string `json:"price"`
	Fills string `json:"fills"`
}

type analysisPricePoint struct {
	Time    time.Time `json:"time"`
	TS      int64     `json:"ts"`
	Open    string    `json:"open"`
	High    string    `json:"high"`
	Low     string    `json:"low"`
	Close   string    `json:"close"`
	Confirm string    `json:"confirm,omitempty"`
}

type analysisSymbolStats struct {
	InstID           string  `json:"inst_id"`
	TradeCount       int     `json:"trade_count"`
	Wins             int     `json:"wins"`
	Losses           int     `json:"losses"`
	Flats            int     `json:"flats"`
	GrossProfit      float64 `json:"gross_profit"`
	GrossLoss        float64 `json:"gross_loss"`
	Fees             float64 `json:"fees"`
	NetPnL           float64 `json:"net_pnl"`
	WinRate          float64 `json:"win_rate"`
	ProfitFactor     float64 `json:"profit_factor"`
	ProfitFactorText string  `json:"profit_factor_text,omitempty"`
	PayoffRatio      float64 `json:"payoff_ratio"`
}

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order database is not configured")
		return
	}
	if s.OKXCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
		return
	}
	priceDays := positiveIntQuery(r, "price_days", defaultPriceDays)
	pnlDays := positiveIntQuery(r, "pnl_days", defaultPNLDays)
	refresh := strings.EqualFold(r.URL.Query().Get("refresh"), "true")
	apiID := strings.TrimSpace(r.URL.Query().Get("api_id"))
	cfg := s.ConfigStore.Get()
	resp, err := s.buildAnalysis(r.Context(), cfg, apiID, priceDays, pnlDays, refresh)
	if err != nil {
		writeError(w, http.StatusBadGateway, "analysis_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) buildAnalysis(ctx context.Context, cfg config.Config, requestedAPIID string, priceDays, pnlDays int, refresh bool) (analysisResponse, error) {
	creds, apiID, err := s.OKXCredentials.OKXCredentials(requestedAPIID)
	if err != nil {
		return analysisResponse{}, err
	}
	now := s.now()
	cacheKey := "analysis|" + apiID + "|" + strconv.Itoa(priceDays) + "|" + strconv.Itoa(pnlDays)
	if !refresh {
		if cached, ok, err := s.Orders.CachedPayload(cacheKey); err != nil {
			return analysisResponse{}, err
		} else if ok && now.Sub(cached.RefreshedAt) < analysisCacheTTL {
			var resp analysisResponse
			if err := json.Unmarshal([]byte(cached.PayloadJSON), &resp); err != nil {
				return analysisResponse{}, err
			}
			resp.Cache.Hit = true
			resp.Cache.Stale = false
			resp.Cache.CachedAt = cached.RefreshedAt
			resp.Cache.CacheKey = cacheKey
			return resp, nil
		}
	}
	client := okx.Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  s.okxHTTPClient(),
	}
	source := analysisSourceStatus{Price: "okx", Fills: "okx"}
	if err := s.refreshAnalysisData(ctx, client, apiID, priceDays, pnlDays, now); err != nil {
		cached, ok, cacheErr := s.Orders.CachedPayload(cacheKey)
		if cacheErr == nil && ok {
			var resp analysisResponse
			if jsonErr := json.Unmarshal([]byte(cached.PayloadJSON), &resp); jsonErr == nil {
				resp.Cache.Hit = true
				resp.Cache.Stale = true
				resp.Cache.CachedAt = cached.RefreshedAt
				resp.Cache.CacheKey = cacheKey
				resp.Source = analysisSourceStatus{Price: "cache", Fills: "cache"}
				return resp, nil
			}
		}
		return analysisResponse{}, err
	}
	resp, err := s.analysisFromStore(apiID, priceDays, pnlDays, now, source)
	if err != nil {
		return analysisResponse{}, err
	}
	resp.Cache.CacheKey = cacheKey
	if err := s.Orders.CachePayload(cacheKey, resp, now); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write analysis cache", "error", err)
	}
	return resp, nil
}

func (s *Server) refreshAnalysisData(ctx context.Context, client okx.Client, apiID string, priceDays, pnlDays int, now time.Time) error {
	priceLimit := priceDays * 24
	if priceLimit <= 0 {
		priceLimit = defaultPriceDays * 24
	}
	candles, _, err := client.MarketCandles(ctx, analysisPriceInstID, analysisPriceBar, priceLimit)
	if err != nil {
		return err
	}
	storageCandles := make([]storage.MarketCandle, 0, len(candles))
	for _, candle := range candles {
		ts, err := strconv.ParseInt(strings.TrimSpace(candle.TS), 10, 64)
		if err != nil {
			continue
		}
		storageCandles = append(storageCandles, storage.MarketCandle{
			InstID:  analysisPriceInstID,
			Bar:     analysisPriceBar,
			TS:      ts,
			Open:    candle.Open,
			High:    candle.High,
			Low:     candle.Low,
			Close:   candle.Close,
			Volume:  candle.Volume,
			Confirm: candle.Confirm,
		})
	}
	if err := s.Orders.UpsertMarketCandles(storageCandles, now); err != nil {
		return err
	}
	return s.refreshFills(ctx, client, apiID, pnlDays, now)
}

func (s *Server) refreshFills(ctx context.Context, client okx.Client, apiID string, pnlDays int, now time.Time) error {
	cutoff := now.AddDate(0, 0, -pnlDays)
	after := ""
	for page := 0; page < 20; page++ {
		fills, _, err := client.FillsHistory(ctx, "SWAP", after, 100)
		if err != nil {
			return err
		}
		if len(fills) == 0 {
			return nil
		}
		stored := make([]storage.OKXFill, 0, len(fills))
		var oldest time.Time
		for i, fill := range fills {
			fillTimeMS, err := strconv.ParseInt(strings.TrimSpace(fill.FillTime), 10, 64)
			if err != nil {
				continue
			}
			fillTime := time.UnixMilli(fillTimeMS).UTC()
			if oldest.IsZero() || fillTime.Before(oldest) {
				oldest = fillTime
			}
			stored = append(stored, storage.OKXFill{
				APIID:    apiID,
				InstType: fill.InstType,
				InstID:   fill.InstID,
				TradeID:  fill.TradeID,
				OrdID:    fill.OrdID,
				ClOrdID:  fill.ClOrdID,
				Side:     fill.Side,
				FillPx:   fill.FillPx,
				FillSz:   fill.FillSz,
				FillPnl:  fill.FillPnl,
				Fee:      fill.Fee,
				FeeCcy:   fill.FeeCcy,
				FillTime: fillTimeMS,
				RawJSON:  fill.RawJSON,
			})
			if i == len(fills)-1 {
				after = fill.TradeID
				if after == "" {
					after = fill.OrdID
				}
			}
		}
		if err := s.Orders.UpsertOKXFills(stored, now); err != nil {
			return err
		}
		if oldest.IsZero() || oldest.Before(cutoff) || len(fills) < 100 || after == "" {
			return nil
		}
	}
	return nil
}

func (s *Server) analysisFromStore(apiID string, priceDays, pnlDays int, now time.Time, source analysisSourceStatus) (analysisResponse, error) {
	priceLimit := priceDays * 24
	priceSince := now.AddDate(0, 0, -priceDays)
	candles, err := s.Orders.ListMarketCandles(analysisPriceInstID, analysisPriceBar, priceSince, priceLimit)
	if err != nil {
		return analysisResponse{}, err
	}
	points := make([]analysisPricePoint, 0, len(candles))
	for _, candle := range candles {
		points = append(points, analysisPricePoint{
			Time:    time.UnixMilli(candle.TS).UTC(),
			TS:      candle.TS,
			Open:    candle.Open,
			High:    candle.High,
			Low:     candle.Low,
			Close:   candle.Close,
			Confirm: candle.Confirm,
		})
	}
	fills, err := s.Orders.ListOKXFills(apiID, now.AddDate(0, 0, -pnlDays))
	if err != nil {
		return analysisResponse{}, err
	}
	summary, symbols := computeStats(fills)
	return analysisResponse{
		OK:          true,
		APIID:       apiID,
		PriceDays:   priceDays,
		PNLDays:     pnlDays,
		PriceInstID: analysisPriceInstID,
		PriceBar:    analysisPriceBar,
		RefreshedAt: now.UTC(),
		Cache: analysisCacheStatus{
			Hit:      false,
			Stale:    false,
			CacheKey: "analysis|" + apiID + "|" + strconv.Itoa(priceDays) + "|" + strconv.Itoa(pnlDays),
		},
		Source:      source,
		PricePoints: points,
		Summary:     summary,
		Symbols:     symbols,
	}, nil
}

func computeStats(fills []storage.OKXFill) (analysisSymbolStats, []analysisSymbolStats) {
	bySymbol := map[string]*analysisSymbolStats{}
	summary := analysisSymbolStats{InstID: "ALL"}
	for _, fill := range fills {
		stats := bySymbol[fill.InstID]
		if stats == nil {
			stats = &analysisSymbolStats{InstID: fill.InstID}
			bySymbol[fill.InstID] = stats
		}
		net := parseFloat(fill.FillPnl)
		fee := 0.0
		if strings.EqualFold(fill.FeeCcy, "USDT") || fill.FeeCcy == "" {
			fee = parseFloat(fill.Fee)
			net += fee
		}
		applyFillStats(stats, net, fee)
		applyFillStats(&summary, net, fee)
	}
	symbols := make([]analysisSymbolStats, 0, len(bySymbol))
	for _, stats := range bySymbol {
		finalizeStats(stats)
		symbols = append(symbols, *stats)
	}
	finalizeStats(&summary)
	sortSymbolStats(symbols)
	return summary, symbols
}

func applyFillStats(stats *analysisSymbolStats, net, fee float64) {
	stats.TradeCount++
	stats.NetPnL += net
	stats.Fees += fee
	switch {
	case net > 0:
		stats.Wins++
		stats.GrossProfit += net
	case net < 0:
		stats.Losses++
		stats.GrossLoss += net
	default:
		stats.Flats++
	}
}

func finalizeStats(stats *analysisSymbolStats) {
	decided := stats.Wins + stats.Losses
	if decided > 0 {
		stats.WinRate = float64(stats.Wins) / float64(decided)
	}
	if stats.GrossLoss < 0 {
		stats.ProfitFactor = stats.GrossProfit / math.Abs(stats.GrossLoss)
		if stats.Wins > 0 && stats.Losses > 0 {
			stats.PayoffRatio = (stats.GrossProfit / float64(stats.Wins)) / math.Abs(stats.GrossLoss/float64(stats.Losses))
		}
	} else if stats.GrossProfit > 0 {
		stats.ProfitFactorText = "∞"
	}
}

func sortSymbolStats(symbols []analysisSymbolStats) {
	for i := 0; i < len(symbols); i++ {
		for j := i + 1; j < len(symbols); j++ {
			if symbols[j].NetPnL > symbols[i].NetPnL {
				symbols[i], symbols[j] = symbols[j], symbols[i]
			}
		}
	}
}

func parseFloat(v string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func positiveIntQuery(r *http.Request, name string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (s *Server) okxHTTPClient() *http.Client {
	if s.OKXHTTPClient != nil {
		return s.OKXHTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

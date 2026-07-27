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
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	analysisPriceInstID = "USDT-USD"
	analysisPriceBar    = "1H"
	defaultPriceDays    = 3
	defaultPNLDays      = 30
	maxAnalysisPNLDays  = 30
	analysisCacheTTL    = 60 * time.Second
	usdtSampleInterval  = time.Minute
	maxBalanceMinutes   = 90 * 24 * 60

	binanceAnalysisTradeWindow = 7 * 24 * time.Hour
	binanceAnalysisTradeLimit  = 1000
)

type analysisResponse struct {
	OK            bool                   `json:"ok"`
	APIID         string                 `json:"api_id"`
	BinanceAPIID  string                 `json:"binance_api_id,omitempty"`
	Env           string                 `json:"env"`
	PriceDays     int                    `json:"price_days"`
	PNLDays       int                    `json:"pnl_days"`
	PriceInstID   string                 `json:"price_inst_id"`
	PriceBar      string                 `json:"price_bar"`
	RefreshedAt   time.Time              `json:"refreshed_at"`
	Cache         analysisCacheStatus    `json:"cache"`
	Source        analysisSourceStatus   `json:"source"`
	Balance       analysisBalance        `json:"balance"`
	PricePoints   []analysisPricePoint   `json:"price_points"`
	BalancePoints []analysisBalancePoint `json:"balance_points"`
	Summary       analysisSymbolStats    `json:"summary"`
	Symbols       []analysisSymbolStats  `json:"symbols"`
	Trades        []analysisTrade        `json:"trades"`
}

type analysisCacheStatus struct {
	Hit      bool      `json:"hit"`
	Stale    bool      `json:"stale"`
	CachedAt time.Time `json:"cached_at,omitempty"`
	CacheKey string    `json:"cache_key"`
}

type analysisSourceStatus struct {
	Balance string `json:"balance"`
	Price   string `json:"price"`
	Fills   string `json:"fills"`
}

type analysisBalance struct {
	TotalEq   string                  `json:"total_eq"`
	AdjEq     string                  `json:"adj_eq,omitempty"`
	AvailEq   string                  `json:"avail_eq,omitempty"`
	Currency  string                  `json:"currency"`
	UpdatedAt string                  `json:"updated_at,omitempty"`
	Details   []analysisBalanceDetail `json:"details"`
}

type analysisBalanceDetail struct {
	Ccy       string `json:"ccy"`
	Eq        string `json:"eq"`
	EqUsd     string `json:"eq_usd"`
	AvailBal  string `json:"avail_bal,omitempty"`
	AvailEq   string `json:"avail_eq,omitempty"`
	CashBal   string `json:"cash_bal,omitempty"`
	FrozenBal string `json:"frozen_bal,omitempty"`
	DisEq     string `json:"dis_eq,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
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

type analysisBalancePoint struct {
	Time             time.Time `json:"time"`
	TS               int64     `json:"ts"`
	Value            float64   `json:"value"`
	Eq               string    `json:"eq,omitempty"`
	EqUsd            string    `json:"eq_usd,omitempty"`
	AvailEq          string    `json:"avail_eq,omitempty"`
	AvailBal         string    `json:"avail_bal,omitempty"`
	CashBal          string    `json:"cash_bal,omitempty"`
	FrozenBal        string    `json:"frozen_bal,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
	BalanceUpdatedAt string    `json:"balance_updated_at,omitempty"`
}

type balanceOverviewResponse struct {
	OK          bool                      `json:"ok"`
	Env         string                    `json:"env"`
	Days        int                       `json:"days"`
	Minutes     int                       `json:"minutes"`
	RefreshedAt time.Time                 `json:"refreshed_at"`
	Exchanges   []exchangeBalanceOverview `json:"exchanges"`
}

type exchangeBalanceOverview struct {
	Exchange      string                 `json:"exchange"`
	Label         string                 `json:"label"`
	Configured    bool                   `json:"configured"`
	Status        string                 `json:"status"`
	APIID         string                 `json:"api_id,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Balance       analysisBalance        `json:"balance"`
	BalancePoints []analysisBalancePoint `json:"balance_points"`
	RefreshedAt   time.Time              `json:"refreshed_at,omitempty"`
}

type analysisSymbolStats struct {
	Exchange         string  `json:"exchange,omitempty"`
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

type analysisTrade struct {
	Exchange string    `json:"exchange"`
	APIID    string    `json:"api_id,omitempty"`
	InstID   string    `json:"inst_id"`
	TradeID  string    `json:"trade_id"`
	OrdID    string    `json:"ord_id,omitempty"`
	Side     string    `json:"side,omitempty"`
	FillPx   string    `json:"fill_px,omitempty"`
	FillSz   string    `json:"fill_sz,omitempty"`
	FillPnl  string    `json:"fill_pnl,omitempty"`
	Fee      string    `json:"fee,omitempty"`
	FeeCcy   string    `json:"fee_ccy,omitempty"`
	FillTime time.Time `json:"fill_time"`
	FillTS   int64     `json:"fill_ts"`
}

func (s *Server) StartUSDTBalanceSampler(ctx context.Context) {
	if s.ConfigStore == nil || s.Orders == nil || (s.OKXCredentials == nil && s.BinanceCredentials == nil) {
		return
	}
	go s.runUSDTBalanceSampler(ctx, usdtSampleInterval)
}

func (s *Server) runUSDTBalanceSampler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = usdtSampleInterval
	}
	s.sampleConfiguredUSDTBalances(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sampleConfiguredUSDTBalances(ctx)
		}
	}
}

func (s *Server) sampleConfiguredUSDTBalances(ctx context.Context) {
	cfg := s.ConfigStore.Get()
	envName := analysisEnvName(cfg)
	now := s.now()
	if s.OKXCredentials != nil {
		for _, requestedAPIID := range configuredAPIIDs(s.OKXCredentials.Status()) {
			sampleCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := s.sampleUSDTBalance(sampleCtx, cfg, requestedAPIID, envName, now)
			cancel()
			if err != nil && s.Logger != nil {
				s.Logger.Warn("failed to sample USDT balance", "exchange", trading.ExchangeOKX, "api_id", requestedAPIID, "env", envName, "error", err)
			}
		}
	}
	if s.BinanceCredentials != nil {
		for _, requestedAPIID := range configuredBinanceAPIIDs(s.BinanceCredentials.Status()) {
			sampleCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := s.sampleBinanceUSDTBalance(sampleCtx, cfg, requestedAPIID, envName, now)
			cancel()
			if err != nil && s.Logger != nil {
				s.Logger.Warn("failed to sample USDT balance", "exchange", trading.ExchangeBinance, "api_id", requestedAPIID, "env", envName, "error", err)
			}
		}
	}
}

func configuredAPIIDs(status okx.CredentialStatus) []string {
	ids := make([]string, 0, len(status.Credentials))
	seen := map[string]bool{}
	for _, account := range status.Credentials {
		id := strings.TrimSpace(account.ID)
		if id == "" || !account.Configured || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	activeID := strings.TrimSpace(status.ActiveID)
	if len(ids) == 0 && status.Configured && activeID != "" {
		ids = append(ids, activeID)
	}
	return ids
}

func configuredBinanceAPIIDs(status binance.CredentialStatus) []string {
	ids := make([]string, 0, len(status.Credentials))
	seen := map[string]bool{}
	for _, account := range status.Credentials {
		id := strings.TrimSpace(account.ID)
		if id == "" || !account.Configured || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	activeID := strings.TrimSpace(status.ActiveID)
	if len(ids) == 0 && status.Configured && activeID != "" {
		ids = append(ids, activeID)
	}
	return ids
}

func (s *Server) sampleUSDTBalance(ctx context.Context, cfg config.Config, requestedAPIID, envName string, now time.Time) error {
	creds, apiID, err := s.OKXCredentials.OKXCredentials(requestedAPIID)
	if err != nil {
		return err
	}
	client := s.analysisOKXClient(cfg, creds)
	_, err = s.fetchAnalysisBalance(ctx, client, apiID, envName, now)
	return err
}

func (s *Server) sampleBinanceUSDTBalance(ctx context.Context, cfg config.Config, requestedAPIID, envName string, now time.Time) error {
	creds, apiID, err := s.BinanceCredentials.BinanceCredentials(requestedAPIID)
	if err != nil {
		return err
	}
	client := s.analysisBinanceClient(cfg, creds)
	_, err = s.fetchBinanceAnalysisBalance(ctx, client, apiID, envName, now)
	return err
}

func (s *Server) handleBalanceOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order database is not configured")
		return
	}
	cfg := s.ConfigStore.Get()
	minutes, days := balanceWindowQuery(r)
	now := s.now()
	envName := analysisEnvName(cfg)
	writeJSON(w, http.StatusOK, balanceOverviewResponse{
		OK:          true,
		Env:         envName,
		Days:        days,
		Minutes:     minutes,
		RefreshedAt: now.UTC(),
		Exchanges: []exchangeBalanceOverview{
			s.balanceOverviewForOKX(r.Context(), cfg, envName, minutes, now),
			s.balanceOverviewForBinance(r.Context(), cfg, envName, minutes, now),
		},
	})
}

func balanceWindowQuery(r *http.Request) (int, int) {
	if raw := strings.TrimSpace(r.URL.Query().Get("minutes")); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes < 0 {
			minutes = defaultPriceDays * 24 * 60
		}
		if minutes > maxBalanceMinutes {
			minutes = maxBalanceMinutes
		}
		days := 0
		if minutes > 0 {
			days = int(math.Ceil(float64(minutes) / (24 * 60)))
		}
		return minutes, days
	}
	days := positiveIntQuery(r, "days", defaultPriceDays)
	minutes := days * 24 * 60
	if minutes > maxBalanceMinutes {
		minutes = maxBalanceMinutes
		days = maxBalanceMinutes / (24 * 60)
	}
	return minutes, days
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
	if pnlDays > maxAnalysisPNLDays {
		pnlDays = maxAnalysisPNLDays
	}
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
	if pnlDays > maxAnalysisPNLDays {
		pnlDays = maxAnalysisPNLDays
	}
	creds, apiID, err := s.OKXCredentials.OKXCredentials(requestedAPIID)
	if err != nil {
		return analysisResponse{}, err
	}
	now := s.now()
	envName := analysisEnvName(cfg)
	binanceCreds, binanceAPIID, binanceConfigured := s.analysisBinanceCredentials()
	cacheKey := analysisCacheKey(apiID, binanceAPIID, envName, priceDays, pnlDays)
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
	client := s.analysisOKXClient(cfg, creds)
	balance, err := s.fetchAnalysisBalance(ctx, client, apiID, envName, now)
	if err != nil {
		return analysisResponse{}, err
	}
	source := analysisSourceStatus{Balance: "okx", Price: "okx", Fills: "okx"}
	binanceTrades := []analysisTrade{}
	if binanceConfigured {
		source.Fills = "okx+binance"
		binanceTrades, err = s.fetchBinanceAnalysisTrades(ctx, s.analysisBinanceClient(cfg, binanceCreds), binanceAPIID, cfg, pnlDays, now)
		if err != nil {
			source.Fills = "okx+binance_error"
			if s.Logger != nil {
				s.Logger.Warn("failed to fetch Binance analysis trades", "api_id", binanceAPIID, "env", envName, "error", err)
			}
		}
	}
	if err := s.refreshAnalysisData(ctx, client, apiID, priceDays, pnlDays, now); err != nil {
		cached, ok, cacheErr := s.Orders.CachedPayload(cacheKey)
		if cacheErr == nil && ok {
			var resp analysisResponse
			if jsonErr := json.Unmarshal([]byte(cached.PayloadJSON), &resp); jsonErr == nil {
				resp.Cache.Hit = true
				resp.Cache.Stale = true
				resp.Cache.CachedAt = cached.RefreshedAt
				resp.Cache.CacheKey = cacheKey
				resp.Source = analysisSourceStatus{Balance: "cache", Price: "cache", Fills: "cache"}
				return resp, nil
			}
		}
		return analysisResponse{}, err
	}
	resp, err := s.analysisFromStore(apiID, binanceAPIID, envName, priceDays, pnlDays, now, source, binanceTrades)
	if err != nil {
		return analysisResponse{}, err
	}
	resp.Balance = balance
	resp.Cache.CacheKey = cacheKey
	if err := s.Orders.CachePayload(cacheKey, resp, now); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write analysis cache", "error", err)
	}
	return resp, nil
}

func (s *Server) analysisBinanceCredentials() (binance.Credentials, string, bool) {
	if s.BinanceCredentials == nil {
		return binance.Credentials{}, "", false
	}
	status := s.BinanceCredentials.Status()
	if !status.Configured {
		return binance.Credentials{}, strings.TrimSpace(status.ActiveID), false
	}
	creds, apiID, err := s.BinanceCredentials.BinanceCredentials("")
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("failed to resolve Binance analysis credentials", "error", err)
		}
		return binance.Credentials{}, strings.TrimSpace(status.ActiveID), false
	}
	return creds, apiID, true
}

func (s *Server) analysisOKXClient(cfg config.Config, creds okx.Credentials) okx.Client {
	return okx.Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  s.okxHTTPClient(),
	}
}

func (s *Server) analysisBinanceClient(cfg config.Config, creds binance.Credentials) binance.Client {
	return binance.Client{
		BaseURL:     cfg.BinanceBaseURL(),
		Credentials: creds,
		HTTPClient:  s.binanceHTTPClient(),
	}
}

func analysisEnvName(cfg config.Config) string {
	envName := strings.ToLower(strings.TrimSpace(cfg.Trading.Env))
	if envName == "" {
		return config.EnvDemo
	}
	return envName
}

func analysisCacheKey(apiID, binanceAPIID, envName string, priceDays, pnlDays int) string {
	return "analysis|" + apiID + "|binance:" + binanceAPIID + "|" + envName + "|" + strconv.Itoa(priceDays) + "|" + strconv.Itoa(pnlDays)
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

func (s *Server) fetchAnalysisBalance(ctx context.Context, client okx.Client, apiID, envName string, now time.Time) (analysisBalance, error) {
	balance, _, err := client.AccountBalanceSnapshot(ctx)
	if err != nil {
		return analysisBalance{}, err
	}
	out := analysisBalanceFromOKX(balance)
	if err := s.recordUSDTBalanceSnapshot(apiID, envName, balance, now); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write USDT balance snapshot", "api_id", apiID, "env", envName, "error", err)
	}
	return out, nil
}

func (s *Server) fetchBinanceAnalysisBalance(ctx context.Context, client binance.Client, apiID, envName string, now time.Time) (analysisBalance, error) {
	balances, err := client.AccountBalance(ctx)
	if err != nil {
		return analysisBalance{}, err
	}
	out := analysisBalanceFromBinance(balances)
	if err := s.recordBinanceUSDTBalanceSnapshot(apiID, envName, balances, now); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write USDT balance snapshot", "exchange", trading.ExchangeBinance, "api_id", apiID, "env", envName, "error", err)
	}
	return out, nil
}

func analysisBalanceFromOKX(balance okx.AccountBalanceData) analysisBalance {
	out := analysisBalance{
		TotalEq:   strings.TrimSpace(balance.TotalEq),
		AdjEq:     strings.TrimSpace(balance.AdjEq),
		AvailEq:   strings.TrimSpace(balance.AvailEq),
		Currency:  "USD",
		UpdatedAt: okxMillisToRFC3339(balance.UTime),
		Details:   make([]analysisBalanceDetail, 0, len(balance.Details)),
	}
	for _, detail := range balance.Details {
		row := analysisBalanceDetail{
			Ccy:       strings.TrimSpace(detail.Ccy),
			Eq:        strings.TrimSpace(detail.Eq),
			EqUsd:     strings.TrimSpace(detail.EqUsd),
			AvailBal:  strings.TrimSpace(detail.AvailBal),
			AvailEq:   strings.TrimSpace(detail.AvailEq),
			CashBal:   strings.TrimSpace(detail.CashBal),
			FrozenBal: strings.TrimSpace(detail.FrozenBal),
			DisEq:     strings.TrimSpace(detail.DisEq),
			UpdatedAt: okxMillisToRFC3339(detail.UTime),
		}
		if row.Ccy == "" {
			continue
		}
		if row.UpdatedAt == "" {
			row.UpdatedAt = out.UpdatedAt
		}
		out.Details = append(out.Details, row)
	}
	sortBalanceDetails(out.Details)
	return out
}

func analysisBalanceFromBinance(balances []binance.Balance) analysisBalance {
	out := analysisBalance{Currency: "USDT"}
	balance, ok := binance.USDTBalanceFromAccount(balances)
	if !ok {
		return out
	}
	updatedAt := binanceMillisToRFC3339(balance.UpdateTime)
	out.TotalEq = strings.TrimSpace(balance.Balance)
	out.AvailEq = strings.TrimSpace(balance.AvailableBalance)
	out.UpdatedAt = updatedAt
	out.Details = []analysisBalanceDetail{{
		Ccy:       "USDT",
		Eq:        strings.TrimSpace(balance.Balance),
		EqUsd:     strings.TrimSpace(balance.Balance),
		AvailBal:  strings.TrimSpace(balance.AvailableBalance),
		AvailEq:   strings.TrimSpace(balance.AvailableBalance),
		CashBal:   strings.TrimSpace(balance.CrossWalletBalance),
		UpdatedAt: updatedAt,
	}}
	return out
}

func sortBalanceDetails(details []analysisBalanceDetail) {
	for i := 0; i < len(details); i++ {
		for j := i + 1; j < len(details); j++ {
			if parseFloat(details[j].EqUsd) > parseFloat(details[i].EqUsd) {
				details[i], details[j] = details[j], details[i]
			}
		}
	}
}

func (s *Server) recordUSDTBalanceSnapshot(apiID, envName string, balance okx.AccountBalanceData, observedAt time.Time) error {
	if s.Orders == nil {
		return nil
	}
	for _, detail := range balance.Details {
		if !strings.EqualFold(strings.TrimSpace(detail.Ccy), "USDT") {
			continue
		}
		updatedAt := okxMillisToRFC3339(detail.UTime)
		if updatedAt == "" {
			updatedAt = okxMillisToRFC3339(balance.UTime)
		}
		return s.Orders.UpsertUSDTBalanceSnapshot(storage.USDTBalanceSnapshot{
			Exchange:         trading.ExchangeOKX,
			APIID:            apiID,
			Env:              envName,
			ObservedAt:       observedAt.UTC(),
			TotalEq:          strings.TrimSpace(balance.TotalEq),
			Eq:               strings.TrimSpace(detail.Eq),
			EqUsd:            strings.TrimSpace(detail.EqUsd),
			AvailEq:          strings.TrimSpace(detail.AvailEq),
			AvailBal:         strings.TrimSpace(detail.AvailBal),
			CashBal:          strings.TrimSpace(detail.CashBal),
			FrozenBal:        strings.TrimSpace(detail.FrozenBal),
			DisEq:            strings.TrimSpace(detail.DisEq),
			BalanceUpdatedAt: updatedAt,
		})
	}
	return nil
}

func (s *Server) recordBinanceUSDTBalanceSnapshot(apiID, envName string, balances []binance.Balance, observedAt time.Time) error {
	if s.Orders == nil {
		return nil
	}
	balance, ok := binance.USDTBalanceFromAccount(balances)
	if !ok {
		return nil
	}
	return s.Orders.UpsertUSDTBalanceSnapshot(storage.USDTBalanceSnapshot{
		Exchange:         trading.ExchangeBinance,
		APIID:            apiID,
		Env:              envName,
		ObservedAt:       observedAt.UTC(),
		TotalEq:          strings.TrimSpace(balance.Balance),
		Eq:               strings.TrimSpace(balance.Balance),
		EqUsd:            strings.TrimSpace(balance.Balance),
		AvailEq:          strings.TrimSpace(balance.AvailableBalance),
		AvailBal:         strings.TrimSpace(balance.AvailableBalance),
		CashBal:          strings.TrimSpace(balance.CrossWalletBalance),
		BalanceUpdatedAt: binanceMillisToRFC3339(balance.UpdateTime),
	})
}

func okxMillisToRFC3339(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func binanceMillisToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
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

func (s *Server) fetchBinanceAnalysisTrades(ctx context.Context, client binance.Client, apiID string, cfg config.Config, pnlDays int, now time.Time) ([]analysisTrade, error) {
	if pnlDays <= 0 {
		pnlDays = defaultPNLDays
	}
	if pnlDays > maxAnalysisPNLDays {
		pnlDays = maxAnalysisPNLDays
	}
	since := now.AddDate(0, 0, -pnlDays).UTC()
	symbols := s.analysisBinanceSymbols(cfg, since)
	if len(symbols) == 0 {
		return nil, nil
	}
	out := []analysisTrade{}
	for _, symbol := range symbols {
		trades, err := fetchBinanceAnalysisSymbolTrades(ctx, client, apiID, symbol, since, now.UTC())
		if err != nil {
			return out, err
		}
		out = append(out, trades...)
	}
	return out, nil
}

func fetchBinanceAnalysisSymbolTrades(ctx context.Context, client binance.Client, apiID, symbol string, since, until time.Time) ([]analysisTrade, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, nil
	}
	out := []analysisTrade{}
	windowEnd := until.UTC()
	if windowEnd.IsZero() {
		windowEnd = time.Now().UTC()
	}
	for windowEnd.After(since) {
		windowStart := windowEnd.Add(-binanceAnalysisTradeWindow)
		if windowStart.Before(since) {
			windowStart = since
		}
		trades, err := client.UserTrades(ctx, symbol, windowStart, windowEnd, binanceAnalysisTradeLimit)
		if err != nil {
			return out, err
		}
		for _, trade := range trades {
			row, ok := analysisTradeFromBinanceTrade(apiID, trade)
			if !ok || row.FillTime.Before(since) || row.FillTime.After(until) {
				continue
			}
			out = append(out, row)
		}
		if len(trades) >= binanceAnalysisTradeLimit {
			return out, fmt.Errorf("Binance %s 7d trades reached %d limit; analysis trade history may be incomplete", symbol, binanceAnalysisTradeLimit)
		}
		windowEnd = windowStart.Add(-time.Millisecond)
	}
	return out, nil
}

func (s *Server) analysisBinanceSymbols(cfg config.Config, since time.Time) []string {
	seen := map[string]bool{}
	add := func(symbol string) {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || seen[symbol] {
			return
		}
		seen[symbol] = true
	}
	for key, sym := range cfg.Symbols {
		if symbol, ok := deriveBinanceAnalysisSymbol(sym.Coinpair, key, ""); ok {
			add(symbol)
		}
	}
	if s.Orders != nil {
		for _, rec := range s.Orders.List(10000) {
			if rec.AcceptedAt.Before(since) && rec.UpdatedAt.Before(since) {
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

func deriveBinanceAnalysisSymbol(candidates ...string) (string, bool) {
	for _, candidate := range candidates {
		raw := strings.ToUpper(strings.TrimSpace(candidate))
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "-") {
			parts := strings.Split(raw, "-")
			if len(parts) >= 2 && parts[1] == "USDT" {
				raw = parts[0] + "USDT"
			}
		}
		symbol, err := binance.DeriveUSDMSymbol(raw, raw)
		if err != nil {
			continue
		}
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if strings.HasSuffix(symbol, "USDT") {
			return symbol, true
		}
	}
	return "", false
}

func analysisTradeFromOKXFill(fill storage.OKXFill) (analysisTrade, bool) {
	instID := strings.ToUpper(strings.TrimSpace(fill.InstID))
	if instID == "" || fill.FillTime <= 0 {
		return analysisTrade{}, false
	}
	return analysisTrade{
		Exchange: trading.ExchangeOKX,
		APIID:    strings.TrimSpace(fill.APIID),
		InstID:   instID,
		TradeID:  strings.TrimSpace(fill.TradeID),
		OrdID:    strings.TrimSpace(fill.OrdID),
		Side:     strings.TrimSpace(fill.Side),
		FillPx:   strings.TrimSpace(fill.FillPx),
		FillSz:   strings.TrimSpace(fill.FillSz),
		FillPnl:  strings.TrimSpace(fill.FillPnl),
		Fee:      strings.TrimSpace(fill.Fee),
		FeeCcy:   strings.TrimSpace(fill.FeeCcy),
		FillTime: time.UnixMilli(fill.FillTime).UTC(),
		FillTS:   fill.FillTime,
	}, true
}

func analysisTradeFromBinanceTrade(apiID string, trade binance.UserTrade) (analysisTrade, bool) {
	instID := strings.ToUpper(strings.TrimSpace(trade.Symbol))
	if instID == "" || trade.Time <= 0 {
		return analysisTrade{}, false
	}
	fee := normalizedBinanceFee(trade.Commission)
	return analysisTrade{
		Exchange: trading.ExchangeBinance,
		APIID:    strings.TrimSpace(apiID),
		InstID:   instID,
		TradeID:  strconv.FormatInt(trade.ID, 10),
		OrdID:    strconv.FormatInt(trade.OrderID, 10),
		Side:     strings.TrimSpace(trade.Side),
		FillPx:   strings.TrimSpace(trade.Price),
		FillSz:   strings.TrimSpace(trade.Qty),
		FillPnl:  strings.TrimSpace(trade.RealizedPnl),
		Fee:      fee,
		FeeCcy:   strings.TrimSpace(trade.CommissionAsset),
		FillTime: time.UnixMilli(trade.Time).UTC(),
		FillTS:   trade.Time,
	}, true
}

func normalizedBinanceFee(commission string) string {
	raw := strings.TrimSpace(commission)
	if raw == "" || raw == "0" {
		return raw
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		if strings.HasPrefix(raw, "-") {
			return raw
		}
		return "-" + raw
	}
	if value > 0 {
		value = -value
	}
	return trading.NormalizeFloat(value)
}

func (s *Server) analysisFromStore(apiID, binanceAPIID, envName string, priceDays, pnlDays int, now time.Time, source analysisSourceStatus, binanceTrades []analysisTrade) (analysisResponse, error) {
	priceLimit := priceDays * 24
	balanceLimit := priceDays*24*60 + 1
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
	snapshots, err := s.Orders.ListExchangeUSDTBalanceSnapshots(trading.ExchangeOKX, apiID, envName, priceSince, balanceLimit)
	if err != nil {
		return analysisResponse{}, err
	}
	balancePoints := balancePointsFromSnapshots(snapshots)
	fills, err := s.Orders.ListOKXFills(apiID, now.AddDate(0, 0, -pnlDays))
	if err != nil {
		return analysisResponse{}, err
	}
	trades := make([]analysisTrade, 0, len(fills)+len(binanceTrades))
	for _, fill := range fills {
		trade, ok := analysisTradeFromOKXFill(fill)
		if ok {
			trades = append(trades, trade)
		}
	}
	trades = append(trades, binanceTrades...)
	sortAnalysisTrades(trades)
	summary, symbols := computeStats(trades)
	return analysisResponse{
		OK:           true,
		APIID:        apiID,
		BinanceAPIID: binanceAPIID,
		Env:          envName,
		PriceDays:    priceDays,
		PNLDays:      pnlDays,
		PriceInstID:  analysisPriceInstID,
		PriceBar:     analysisPriceBar,
		RefreshedAt:  now.UTC(),
		Cache: analysisCacheStatus{
			Hit:      false,
			Stale:    false,
			CacheKey: analysisCacheKey(apiID, binanceAPIID, envName, priceDays, pnlDays),
		},
		Source:        source,
		PricePoints:   points,
		BalancePoints: balancePoints,
		Summary:       summary,
		Symbols:       symbols,
		Trades:        trades,
	}, nil
}

func (s *Server) balanceOverviewForOKX(ctx context.Context, cfg config.Config, envName string, minutes int, now time.Time) exchangeBalanceOverview {
	out := exchangeBalanceOverview{Exchange: trading.ExchangeOKX, Label: "OKX", Status: "not_configured", RefreshedAt: now.UTC()}
	if s.OKXCredentials == nil {
		out.Error = "OKX credential store is not configured"
		return out
	}
	status := s.OKXCredentials.Status()
	out.Configured = status.Configured
	out.APIID = strings.TrimSpace(status.ActiveID)
	if !status.Configured {
		return out
	}
	creds, apiID, err := s.OKXCredentials.OKXCredentials(out.APIID)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	out.APIID = apiID
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	balance, err := s.fetchAnalysisBalance(fetchCtx, s.analysisOKXClient(cfg, creds), apiID, envName, now)
	cancel()
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
	} else {
		out.Status = "ok"
		out.Balance = balance
	}
	out.BalancePoints = s.balanceOverviewPoints(trading.ExchangeOKX, apiID, envName, minutes, now)
	return out
}

func (s *Server) balanceOverviewForBinance(ctx context.Context, cfg config.Config, envName string, minutes int, now time.Time) exchangeBalanceOverview {
	out := exchangeBalanceOverview{Exchange: trading.ExchangeBinance, Label: "Binance", Status: "not_configured", RefreshedAt: now.UTC()}
	if s.BinanceCredentials == nil {
		out.Error = "Binance credential store is not configured"
		return out
	}
	status := s.BinanceCredentials.Status()
	out.Configured = status.Configured
	out.APIID = strings.TrimSpace(status.ActiveID)
	if !status.Configured {
		return out
	}
	creds, apiID, err := s.BinanceCredentials.BinanceCredentials(out.APIID)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	out.APIID = apiID
	fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	balance, err := s.fetchBinanceAnalysisBalance(fetchCtx, s.analysisBinanceClient(cfg, creds), apiID, envName, now)
	cancel()
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
	} else {
		out.Status = "ok"
		out.Balance = balance
	}
	out.BalancePoints = s.balanceOverviewPoints(trading.ExchangeBinance, apiID, envName, minutes, now)
	return out
}

func (s *Server) balanceOverviewPoints(exchange, apiID, envName string, minutes int, now time.Time) []analysisBalancePoint {
	if s.Orders == nil || strings.TrimSpace(apiID) == "" {
		return nil
	}
	if minutes < 0 {
		minutes = 0
	}
	if minutes > maxBalanceMinutes {
		minutes = maxBalanceMinutes
	}
	limit := minutes + 1
	since := now.Add(-time.Duration(minutes) * time.Minute)
	if minutes == 0 {
		limit = 6
		since = now.Add(-5 * time.Minute)
	}
	snapshots, err := s.Orders.ListExchangeUSDTBalanceSnapshots(exchange, apiID, envName, since, limit)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("failed to list USDT balance snapshots", "exchange", exchange, "api_id", apiID, "env", envName, "error", err)
		}
		return nil
	}
	return compactBalancePoints(balancePointsFromSnapshots(snapshots), minutes)
}

func balancePointsFromSnapshots(snapshots []storage.USDTBalanceSnapshot) []analysisBalancePoint {
	balancePoints := make([]analysisBalancePoint, 0, len(snapshots))
	for _, snapshot := range snapshots {
		balancePoints = append(balancePoints, analysisBalancePoint{
			Time:             time.UnixMilli(snapshot.BucketTS).UTC(),
			TS:               snapshot.BucketTS,
			Value:            snapshotValue(snapshot),
			Eq:               snapshot.Eq,
			EqUsd:            snapshot.EqUsd,
			AvailEq:          snapshot.AvailEq,
			AvailBal:         snapshot.AvailBal,
			CashBal:          snapshot.CashBal,
			FrozenBal:        snapshot.FrozenBal,
			ObservedAt:       snapshot.ObservedAt,
			BalanceUpdatedAt: snapshot.BalanceUpdatedAt,
		})
	}
	return balancePoints
}

func compactBalancePoints(points []analysisBalancePoint, minutes int) []analysisBalancePoint {
	if len(points) == 0 {
		return nil
	}
	if minutes <= 0 {
		return []analysisBalancePoint{points[len(points)-1]}
	}
	bucket := balancePointBucket(minutes)
	if bucket <= time.Minute {
		return points
	}
	out := make([]analysisBalancePoint, 0, len(points))
	lastBucket := int64(-1)
	for _, point := range points {
		ts := point.TS
		if ts <= 0 && !point.Time.IsZero() {
			ts = point.Time.UnixMilli()
		}
		if ts <= 0 {
			continue
		}
		bucketTS := time.UnixMilli(ts).UTC().Truncate(bucket).UnixMilli()
		point.TS = bucketTS
		point.Time = time.UnixMilli(bucketTS).UTC()
		if len(out) > 0 && bucketTS == lastBucket {
			out[len(out)-1] = point
			continue
		}
		out = append(out, point)
		lastBucket = bucketTS
	}
	return out
}

func balancePointBucket(minutes int) time.Duration {
	if minutes >= 30*24*60 {
		return time.Hour
	}
	return time.Minute
}

func snapshotValue(snapshot storage.USDTBalanceSnapshot) float64 {
	for _, raw := range []string{snapshot.EqUsd, snapshot.Eq, snapshot.AvailEq, snapshot.AvailBal} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func computeStats(fills []analysisTrade) (analysisSymbolStats, []analysisSymbolStats) {
	bySymbol := map[string]*analysisSymbolStats{}
	summary := analysisSymbolStats{InstID: "ALL"}
	for _, fill := range fills {
		exchange := trading.NormalizeExchange(fill.Exchange)
		instID := strings.ToUpper(strings.TrimSpace(fill.InstID))
		if instID == "" {
			continue
		}
		key := exchange + "|" + instID
		stats := bySymbol[key]
		if stats == nil {
			stats = &analysisSymbolStats{Exchange: exchange, InstID: instID}
			bySymbol[key] = stats
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
			if symbols[j].NetPnL > symbols[i].NetPnL ||
				(symbols[j].NetPnL == symbols[i].NetPnL && symbols[j].Exchange < symbols[i].Exchange) ||
				(symbols[j].NetPnL == symbols[i].NetPnL && symbols[j].Exchange == symbols[i].Exchange && symbols[j].InstID < symbols[i].InstID) {
				symbols[i], symbols[j] = symbols[j], symbols[i]
			}
		}
	}
}

func sortAnalysisTrades(trades []analysisTrade) {
	sort.SliceStable(trades, func(i, j int) bool {
		if trades[i].FillTS == trades[j].FillTS {
			if trades[i].Exchange == trades[j].Exchange {
				if trades[i].InstID == trades[j].InstID {
					return trades[i].TradeID > trades[j].TradeID
				}
				return trades[i].InstID < trades[j].InstID
			}
			return trades[i].Exchange < trades[j].Exchange
		}
		return trades[i].FillTS > trades[j].FillTS
	})
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

func (s *Server) binanceHTTPClient() *http.Client {
	if s.BinanceHTTPClient != nil {
		return s.BinanceHTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

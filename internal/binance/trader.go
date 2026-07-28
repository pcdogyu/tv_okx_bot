package binance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const maxBinanceLeverageFallback = 50

type Trader struct {
	Credentials        Credentials
	CredentialProvider CredentialProvider
	HTTPClient         *http.Client
	Logger             *slog.Logger
}

type USDTBalance struct {
	Ccy              string `json:"ccy"`
	TotalEquity      string `json:"total_eq,omitempty"`
	Equity           string `json:"eq,omitempty"`
	AvailableEquity  string `json:"avail_eq,omitempty"`
	AvailableBalance string `json:"avail_bal,omitempty"`
	CashBalance      string `json:"cash_bal,omitempty"`
	FrozenBalance    string `json:"frozen_bal,omitempty"`
	UpdateTime       string `json:"u_time,omitempty"`
}

func (t Trader) ExecuteSignal(ctx context.Context, signal trading.Signal, cfg trading.RuntimeConfig) (trading.OrderResult, error) {
	if !cfg.BinanceLiveTradingAllowedByEnvironment() {
		return trading.OrderResult{}, fmt.Errorf("live trading requires config env=live, BINANCE_ENV=live and ALLOW_LIVE_TRADING=true")
	}
	orderSettings := cfg.OrderSettings().Normalize()
	orderSettings.ApplyToSignal(&signal)
	if err := validateBinanceRisk(signal.Risk); err != nil {
		return trading.OrderResult{}, err
	}
	client, apiID, err := t.client(cfg, signal.APIID)
	if err != nil {
		return trading.OrderResult{}, err
	}
	symbol, err := DeriveUSDMSymbol(signal.Coinpair, signal.Ticker)
	if err != nil {
		return trading.OrderResult{}, err
	}
	info, err := client.SymbolInfo(ctx, symbol)
	if err != nil {
		return trading.OrderResult{}, err
	}
	filters, err := info.TradingFilters()
	if err != nil {
		return trading.OrderResult{}, err
	}
	switchState, err := t.prepareDirectionSwitch(ctx, client, signal.Action, symbol, filters)
	if err != nil {
		return trading.OrderResult{}, err
	}
	sizingPx := signal.Price.Value
	orderType := strings.ToUpper(string(orderSettings.OrderType))
	orderPx := ""
	if orderSettings.OrderType == trading.OrderTypeLimit {
		sizingPx = orderSettings.LimitPrice(signal.Action, signal.Price.Value)
		orderPx = formatStep(sizingPx, filters.TickSize, signal.Action == trading.ActionShort)
	}
	quantity := formatStep(signal.Amount.Value/sizingPx, filters.StepSize, false)
	if compareDecimal(quantity, filters.MinQty) < 0 {
		return trading.OrderResult{}, fmt.Errorf("order size %s is below Binance minQty %s", quantity, filters.MinQty)
	}
	posSide := binancePositionSide(signal.Action, cfg.PositionMode())
	marginType := binanceMarginType(cfg.MarginMode())
	if err := client.ChangeMarginType(ctx, symbol, marginType); err != nil {
		if shouldContinueAfterBinanceMarginSetupError(err) {
			if t.Logger != nil {
				t.Logger.Warn("binance margin type setup skipped before order", "symbol", symbol, "margin_type", marginType, "error", err)
			}
		} else {
			return trading.OrderResult{}, fmt.Errorf("binance change margin type %s to %s: %w", symbol, marginType, err)
		}
	}
	usedLeverage, err := t.ensureLeverage(ctx, client, symbol, posSide, signal.Action, signal.Leverage, switchState)
	if err != nil {
		return trading.OrderResult{}, fmt.Errorf("binance set leverage %s to %dx: %w", symbol, signal.Leverage, err)
	}
	clOrdID := clientOrderID(signal)
	req := PlaceOrderRequest{
		Symbol:           symbol,
		Side:             binanceSide(signal.Action),
		PositionSide:     posSide,
		Type:             orderType,
		Quantity:         quantity,
		Price:            orderPx,
		NewClientOrderID: clOrdID,
	}
	if orderSettings.OrderType == trading.OrderTypeLimit {
		req.TimeInForce = "GTC"
	}
	ack, err := client.PlaceOrder(ctx, req)
	result := trading.OrderResult{
		APIID:          apiID,
		TargetExchange: trading.ExchangeBinance,
		InstID:         symbol,
		ClOrdID:        clOrdID,
		OrdType:        req.Type,
		Px:             req.Price,
		OrdID:          strconv.FormatInt(ack.OrderID, 10),
		Leverage:       usedLeverage,
	}
	if err != nil {
		return result, err
	}
	if err := t.placeRiskOrders(ctx, client, signal, req, sizingPx, filters.TickSize); err != nil {
		return result, err
	}
	if t.Logger != nil {
		t.Logger.Info("binance order submitted", "api_id", apiID, "symbol", symbol, "action", signal.Action, "client_order_id", clOrdID)
	}
	return result, nil
}

func (t Trader) ensureLeverage(ctx context.Context, client Client, symbol, posSide string, action trading.Side, desired int, state directionSwitchState) (int, error) {
	if desired <= 0 {
		return 0, fmt.Errorf("invalid leverage %d: must be positive", desired)
	}
	if binanceRemoteLeverageMatches(state.positions, symbol, posSide, action, desired) {
		if t.Logger != nil {
			t.Logger.Info("binance leverage already matches configured value", "symbol", symbol, "position_side", posSide, "leverage", desired)
		}
		return desired, nil
	}
	return t.setLeverageWithFallback(ctx, client, symbol, desired)
}

func (t Trader) setLeverageWithFallback(ctx context.Context, client Client, symbol string, desired int) (int, error) {
	attempts := binanceLeverageAttempts(desired)
	var lastErr error
	for idx, leverage := range attempts {
		err := client.SetLeverage(ctx, symbol, leverage)
		if err == nil {
			return leverage, nil
		}
		lastErr = err
		if !isBinanceLeverageFallbackError(err) {
			return 0, err
		}
		if t.Logger != nil && idx+1 < len(attempts) {
			t.Logger.Warn("binance leverage rejected, trying fallback leverage", "symbol", symbol, "leverage", leverage, "next_leverage", attempts[idx+1], "error", err)
		}
	}
	return 0, fmt.Errorf("set leverage failed after trying %s: %w", binanceLeverageAttemptsText(attempts), lastErr)
}

func binanceLeverageAttempts(desired int) []int {
	if desired <= 0 {
		return nil
	}
	attempts := make([]int, 0, maxBinanceLeverageFallback)
	seen := map[int]bool{}
	add := func(leverage int) {
		if leverage <= 0 || seen[leverage] {
			return
		}
		attempts = append(attempts, leverage)
		seen[leverage] = true
	}
	add(desired)
	for leverage := minInt(desired-1, maxBinanceLeverageFallback); leverage >= 1; leverage-- {
		add(leverage)
	}
	for leverage := desired + 1; leverage <= maxBinanceLeverageFallback; leverage++ {
		add(leverage)
	}
	return attempts
}

func binanceLeverageAttemptsText(attempts []int) string {
	parts := make([]string, 0, len(attempts))
	for _, leverage := range attempts {
		parts = append(parts, strconv.Itoa(leverage)+"x")
	}
	return strings.Join(parts, ", ")
}

func isBinanceLeverageFallbackError(err error) bool {
	if err == nil {
		return false
	}
	if IsAPIErrorCode(err, -4028) {
		return true
	}
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "leverage") {
		return false
	}
	return strings.Contains(text, "not valid") ||
		strings.Contains(text, "invalid") ||
		strings.Contains(text, "maximum") ||
		strings.Contains(text, "exceed") ||
		strings.Contains(text, "less than") ||
		strings.Contains(text, "greater than")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func binanceRemoteLeverageMatches(positions []Position, symbol, posSide string, action trading.Side, desired int) bool {
	found := false
	mismatch := false
	for _, position := range positions {
		if !strings.EqualFold(position.Symbol, symbol) || !binancePositionMatchesLeverageScope(position, posSide, action) {
			continue
		}
		if strings.TrimSpace(position.Leverage) == "" {
			continue
		}
		found = true
		if !binanceLeverageValueMatches(position.Leverage, desired) {
			mismatch = true
		}
	}
	return found && !mismatch
}

func binancePositionMatchesLeverageScope(position Position, posSide string, action trading.Side) bool {
	wantSide := normalizeBinancePositionSide(posSide)
	if wantSide != "" {
		return normalizeBinancePositionSide(position.PositionSide) == wantSide
	}
	if direction, ok := binancePositionDirection(position); ok {
		return direction == action
	}
	return normalizeBinancePositionSide(position.PositionSide) == ""
}

func binanceLeverageValueMatches(raw string, desired int) bool {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return err == nil && math.Abs(value-float64(desired)) < 1e-9
}

func (t Trader) placeRiskOrders(ctx context.Context, client Client, signal trading.Signal, order PlaceOrderRequest, entryPx, tickSize float64) error {
	risk := signal.Risk
	risk.Normalize()
	switch risk.Type {
	case trading.RiskNone:
		return nil
	case trading.RiskTrailing:
		return t.placeTrailingStop(ctx, client, signal, order)
	case trading.RiskTPSL:
	default:
		return nil
	}
	if risk.TPPct == nil || risk.SLPct == nil {
		return nil
	}
	closeSide := "SELL"
	if signal.Action == trading.ActionShort {
		closeSide = "BUY"
	}
	tpPx, slPx := riskTriggerPrices(signal.Action, entryPx, risk.TPPct.Value, risk.SLPct.Value)
	tpID := trimClientID(order.NewClientOrderID, 32-2) + "TP"
	slID := trimClientID(order.NewClientOrderID, 32-2) + "SL"
	if _, err := client.NewAlgoOrder(ctx, AlgoOrderRequest{
		Symbol:           order.Symbol,
		Side:             closeSide,
		PositionSide:     order.PositionSide,
		Type:             "TAKE_PROFIT_MARKET",
		Quantity:         order.Quantity,
		TriggerPrice:     formatStep(tpPx, tickSize, signal.Action == trading.ActionLong),
		WorkingType:      "MARK_PRICE",
		NewClientOrderID: tpID,
		ReduceOnly:       order.PositionSide == "",
	}); err != nil {
		return err
	}
	if _, err := client.NewAlgoOrder(ctx, AlgoOrderRequest{
		Symbol:           order.Symbol,
		Side:             closeSide,
		PositionSide:     order.PositionSide,
		Type:             "STOP_MARKET",
		Quantity:         order.Quantity,
		TriggerPrice:     formatStep(slPx, tickSize, signal.Action == trading.ActionShort),
		WorkingType:      "MARK_PRICE",
		NewClientOrderID: slID,
		ReduceOnly:       order.PositionSide == "",
	}); err != nil {
		return err
	}
	return nil
}

func validateBinanceRisk(risk trading.Risk) error {
	risk.Normalize()
	if risk.Type != trading.RiskTrailing {
		return nil
	}
	if risk.TrailingPct == nil || !risk.TrailingPct.Set || risk.TrailingPct.Value <= 0 {
		return fmt.Errorf("Binance trailing_pct must be positive")
	}
	if risk.TrailingPct.Value < 0.1 || risk.TrailingPct.Value > 10 {
		return fmt.Errorf("Binance trailing_pct must be between 0.1 and 10, got %s", trading.NormalizeFloat(risk.TrailingPct.Value))
	}
	return nil
}

func (t Trader) placeTrailingStop(ctx context.Context, client Client, signal trading.Signal, order PlaceOrderRequest) error {
	risk := signal.Risk
	risk.Normalize()
	if risk.TrailingPct == nil || !risk.TrailingPct.Set {
		return nil
	}
	callbackRate := risk.TrailingPct.Value
	if callbackRate < 0.1 || callbackRate > 10 {
		return fmt.Errorf("Binance trailing_pct must be between 0.1 and 10, got %s", trading.NormalizeFloat(callbackRate))
	}
	closeSide := "SELL"
	if signal.Action == trading.ActionShort {
		closeSide = "BUY"
	}
	trailingID := trimClientID(order.NewClientOrderID, 32-2) + "TS"
	_, err := client.NewAlgoOrder(ctx, AlgoOrderRequest{
		Symbol:           order.Symbol,
		Side:             closeSide,
		PositionSide:     order.PositionSide,
		Type:             "TRAILING_STOP_MARKET",
		Quantity:         order.Quantity,
		CallbackRate:     trading.NormalizeFloat(callbackRate),
		WorkingType:      "MARK_PRICE",
		NewClientOrderID: trailingID,
		ReduceOnly:       order.PositionSide == "",
	})
	return err
}

func (t Trader) Check(ctx context.Context, cfg trading.RuntimeConfig) (map[string]any, error) {
	return t.CheckAccount(ctx, cfg, "")
}

func (t Trader) CheckAccount(ctx context.Context, cfg trading.RuntimeConfig, apiID string) (map[string]any, error) {
	client, resolvedID, err := t.client(cfg, apiID)
	if err != nil {
		return nil, err
	}
	return t.checkClient(ctx, cfg, client, resolvedID)
}

func (t Trader) CheckCredentials(ctx context.Context, cfg trading.RuntimeConfig, apiID string, creds Credentials) (map[string]any, error) {
	creds = trimCredentials(creds)
	if err := creds.Validate(); err != nil {
		return nil, err
	}
	resolvedID := strings.TrimSpace(apiID)
	if resolvedID == "" {
		resolvedID = "input"
	}
	client := Client{BaseURL: cfg.BinanceBaseURL(), Credentials: creds, HTTPClient: t.HTTPClient}
	return t.checkClient(ctx, cfg, client, resolvedID)
}

func (t Trader) checkClient(ctx context.Context, cfg trading.RuntimeConfig, client Client, apiID string) (map[string]any, error) {
	balances, err := client.AccountBalance(ctx)
	if err != nil {
		return nil, err
	}
	info, err := client.ExchangeInfo(ctx)
	if err != nil {
		return nil, err
	}
	usdtBalance, found := usdtBalanceFromAccount(balances)
	return map[string]any{
		"ok":                 true,
		"exchange":           trading.ExchangeBinance,
		"api_id":             apiID,
		"base_url":           cfg.BinanceBaseURL(),
		"usdt_balance":       usdtBalance,
		"usdt_balance_found": found,
		"instruments_count":  len(info.Symbols),
	}, nil
}

func usdtBalanceFromAccount(balances []Balance) (USDTBalance, bool) {
	balance, ok := USDTBalanceFromAccount(balances)
	if !ok {
		return USDTBalance{}, false
	}
	return USDTBalance{
		Ccy:              "USDT",
		TotalEquity:      balance.Balance,
		Equity:           balance.Balance,
		AvailableEquity:  balance.AvailableBalance,
		AvailableBalance: balance.AvailableBalance,
		CashBalance:      balance.CrossWalletBalance,
		UpdateTime:       strconv.FormatInt(balance.UpdateTime, 10),
	}, true
}

func (t Trader) client(cfg trading.RuntimeConfig, apiID string) (Client, string, error) {
	creds, resolvedID, err := t.credentials(apiID)
	if err != nil {
		return Client{}, resolvedID, err
	}
	return Client{BaseURL: cfg.BinanceBaseURL(), Credentials: creds, HTTPClient: t.HTTPClient}, resolvedID, nil
}

func (t Trader) credentials(apiID string) (Credentials, string, error) {
	if t.CredentialProvider != nil {
		return t.CredentialProvider.BinanceCredentials(apiID)
	}
	creds := trimCredentials(t.Credentials)
	if err := creds.Validate(); err != nil {
		return Credentials{}, strings.TrimSpace(apiID), err
	}
	return creds, strings.TrimSpace(apiID), nil
}

type TradingFilters struct {
	TickSize float64
	StepSize float64
	MinQty   string
}

func (s SymbolInfo) TradingFilters() (TradingFilters, error) {
	var out TradingFilters
	for _, filter := range s.Filters {
		switch filter.FilterType {
		case "PRICE_FILTER":
			tick, err := strconv.ParseFloat(strings.TrimSpace(filter.TickSize), 64)
			if err != nil || tick <= 0 {
				return TradingFilters{}, fmt.Errorf("invalid Binance tickSize %q", filter.TickSize)
			}
			out.TickSize = tick
		case "LOT_SIZE":
			step, err := strconv.ParseFloat(strings.TrimSpace(filter.StepSize), 64)
			if err != nil || step <= 0 {
				return TradingFilters{}, fmt.Errorf("invalid Binance stepSize %q", filter.StepSize)
			}
			out.StepSize = step
			out.MinQty = strings.TrimSpace(filter.MinQty)
		}
	}
	if out.TickSize <= 0 {
		out.TickSize = math.Pow10(-maxInt(s.PricePrecision, 0))
	}
	if out.StepSize <= 0 {
		out.StepSize = math.Pow10(-maxInt(s.QuantityPrecision, 0))
	}
	if out.MinQty == "" {
		out.MinQty = formatStep(out.StepSize, out.StepSize, false)
	}
	return out, nil
}

func DeriveUSDMSymbol(coinpair, ticker string) (string, error) {
	raw := strings.TrimSpace(coinpair)
	if raw == "" {
		raw = strings.TrimSpace(ticker)
	}
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.ToUpper(strings.TrimSpace(raw))
	raw = strings.TrimSuffix(raw, ".P")
	raw = strings.TrimSuffix(raw, "PERP")
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.ReplaceAll(raw, "_", "")
	raw = strings.ReplaceAll(raw, "/", "")
	if raw == "" {
		return "", fmt.Errorf("coinpair or ticker is required")
	}
	if strings.HasSuffix(raw, "USDT") {
		return raw, nil
	}
	return raw + "USDT", nil
}

func binanceSide(action trading.Side) string {
	if action == trading.ActionShort {
		return "SELL"
	}
	return "BUY"
}

func binancePositionSide(action trading.Side, mode string) string {
	if mode != config.PositionLongShort {
		return ""
	}
	if action == trading.ActionShort {
		return "SHORT"
	}
	return "LONG"
}

func binanceMarginType(mode string) string {
	if mode == config.MarginCross {
		return "CROSSED"
	}
	return "ISOLATED"
}

func shouldContinueAfterBinanceMarginSetupError(err error) bool {
	return IsAPIErrorCode(err, -4047, -4048, -4067, -4068)
}

func riskTriggerPrices(action trading.Side, entryPx, tpPct, slPct float64) (float64, float64) {
	if action == trading.ActionShort {
		return entryPx * (1 - tpPct/100), entryPx * (1 + slPct/100)
	}
	return entryPx * (1 + tpPct/100), entryPx * (1 - slPct/100)
}

func formatStep(value, step float64, ceil bool) string {
	if step <= 0 {
		return trading.NormalizeFloat(value)
	}
	scaled := value / step
	if ceil {
		value = math.Ceil(scaled-1e-12) * step
	} else {
		value = math.Floor(scaled+1e-12) * step
	}
	return trading.NormalizeFloat(value)
}

func compareDecimal(a, b string) int {
	af, _ := strconv.ParseFloat(strings.TrimSpace(a), 64)
	bf, _ := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if af < bf {
		return -1
	}
	if af > bf {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clientOrderID(signal trading.Signal) string {
	sum := sha256.Sum256([]byte(signal.CanonicalTokenPayload() + "|" + signal.SentAt + "|" + signal.Ticker + "|binance"))
	short := hex.EncodeToString(sum[:])[:10]
	return trimClientID(strings.ToUpper(fmt.Sprintf("TV%d%s", time.Now().UTC().UnixMilli(), short)), 32)
}

func trimClientID(v string, maxLen int) string {
	v = strings.TrimSpace(v)
	if maxLen <= 0 {
		return ""
	}
	if len(v) > maxLen {
		return v[:maxLen]
	}
	return v
}

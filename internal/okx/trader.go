package okx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

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
	if !cfg.LiveTradingAllowedByEnvironment() {
		return trading.OrderResult{}, fmt.Errorf("live trading requires config env=live, OKX_ENV=live and ALLOW_LIVE_TRADING=true")
	}
	client, apiID, err := t.client(cfg, signal.APIID)
	if err != nil {
		return trading.OrderResult{}, err
	}
	sym, err := t.resolveSymbol(ctx, client, signal, cfg)
	if err != nil {
		return trading.OrderResult{}, err
	}
	orderSettings := cfg.OrderSettings().Normalize()
	orderType := string(orderSettings.OrderType)
	sizingPx := signal.Price.Value
	orderPx := ""
	if orderSettings.OrderType == trading.OrderTypeLimit {
		sizingPx = orderSettings.LimitPrice(signal.Action, signal.Price.Value)
		orderPx = trading.NormalizeFloat(sizingPx)
	}
	sz, err := trading.SizeFromUSDTNotional(signal.Amount.Value, sizingPx, sym.CtVal, sym.LotSz, sym.MinSz)
	if err != nil {
		return trading.OrderResult{}, err
	}
	posSide := ""
	if cfg.PositionMode() == config.PositionLongShort {
		posSide = string(signal.Action)
	}
	usedLeverage, err := t.setLeverageWithFallback(ctx, client, SetLeverageRequest{
		InstID:  sym.InstID,
		Lever:   strconv.Itoa(signal.Leverage),
		MgnMode: cfg.MarginMode(),
		PosSide: posSide,
	})
	if err != nil {
		return trading.OrderResult{}, err
	}
	signal.Leverage = usedLeverage
	clOrdID := clientOrderID(signal)
	req := PlaceOrderRequest{
		InstID:         sym.InstID,
		TDMode:         cfg.MarginMode(),
		ClOrdID:        clOrdID,
		Side:           okxSide(signal.Action),
		PosSide:        posSide,
		OrdType:        orderType,
		Px:             orderPx,
		Sz:             sz,
		AttachAlgoOrds: attachAlgoOrders(signal, clOrdID),
	}
	ack, env, err := client.PlaceOrder(ctx, req)
	result := trading.OrderResult{
		SignalID:       "",
		APIID:          apiID,
		TargetExchange: trading.ExchangeOKX,
		InstID:         sym.InstID,
		ClOrdID:        clOrdID,
		OrdType:        req.OrdType,
		Px:             req.Px,
		OrdID:          ack.OrdID,
		OKXCode:        env.Code,
		OKXMsg:         env.Msg,
		Leverage:       usedLeverage,
	}
	if err != nil {
		return result, err
	}
	if t.Logger != nil {
		t.Logger.Info("okx order submitted", "api_id", apiID, "inst_id", sym.InstID, "action", signal.Action, "cl_ord_id", clOrdID, "okx_code", env.Code)
	}
	return result, nil
}

func (t Trader) setLeverageWithFallback(ctx context.Context, client Client, req SetLeverageRequest) (int, error) {
	desired, err := strconv.Atoi(strings.TrimSpace(req.Lever))
	if err != nil || desired <= 0 {
		if err == nil {
			err = fmt.Errorf("must be positive")
		}
		return 0, fmt.Errorf("invalid leverage %q: %w", req.Lever, err)
	}
	var lastErr error
	for lever := desired; lever >= 1; lever-- {
		req.Lever = strconv.Itoa(lever)
		err := client.SetLeverage(ctx, req)
		if err == nil {
			return lever, nil
		}
		lastErr = err
		if !isMaxLeverageError(err) {
			return 0, err
		}
		if t.Logger != nil && lever > 1 {
			t.Logger.Warn("okx leverage exceeds maximum, trying lower leverage", "inst_id", req.InstID, "leverage", lever, "next_leverage", lever-1)
		}
	}
	return 0, fmt.Errorf("set leverage failed from %dx down to 1x: %w", desired, lastErr)
}

func isMaxLeverageError(err error) bool {
	var apiErr APIError
	if errors.As(err, &apiErr) && apiErr.HasCode("59102") {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "59102") ||
		(strings.Contains(text, "leverage") && strings.Contains(text, "maximum"))
}

func (t Trader) resolveSymbol(ctx context.Context, client Client, signal trading.Signal, cfg trading.RuntimeConfig) (trading.SymbolInfo, error) {
	if sym, ok := cfg.SymbolMeta(signal.Coinpair); ok {
		return sym, nil
	}
	instID, coinpair, err := DeriveSwapInstrumentID(signal.Coinpair, signal.Ticker)
	if err != nil {
		return trading.SymbolInfo{}, err
	}
	inst, err := client.SwapInstrument(ctx, instID)
	if err != nil {
		return trading.SymbolInfo{}, err
	}
	meta, err := inst.SymbolInfo()
	if err != nil {
		return trading.SymbolInfo{}, err
	}
	return trading.SymbolInfo{
		Coinpair: coinpair,
		InstID:   meta.InstID,
		CtVal:    meta.CtVal,
		LotSz:    meta.LotSz,
		MinSz:    meta.MinSz,
	}, nil
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
	client := Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  t.HTTPClient,
	}
	return t.checkClient(ctx, cfg, client, resolvedID)
}

func (t Trader) checkClient(ctx context.Context, cfg trading.RuntimeConfig, client Client, apiID string) (map[string]any, error) {
	balanceData, balance, err := client.AccountBalanceSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	instruments, err := client.Instruments(ctx)
	if err != nil {
		return nil, err
	}
	usdtBalance, usdtBalanceFound := usdtBalanceFromAccount(balanceData)
	return map[string]any{
		"ok":                 true,
		"api_id":             apiID,
		"demo":               cfg.DemoTradingHeaderEnabled(),
		"base_url":           cfg.OKXBaseURL(),
		"balance_code":       balance.Code,
		"balance_msg":        balance.Msg,
		"usdt_balance":       usdtBalance,
		"usdt_balance_found": usdtBalanceFound,
		"instruments":        string(instruments.Data),
	}, nil
}

func usdtBalanceFromAccount(account AccountBalanceData) (USDTBalance, bool) {
	for _, detail := range account.Details {
		if strings.EqualFold(detail.Ccy, "USDT") {
			return USDTBalance{
				Ccy:              "USDT",
				TotalEquity:      account.TotalEq,
				Equity:           detail.Eq,
				AvailableEquity:  detail.AvailEq,
				AvailableBalance: detail.AvailBal,
				CashBalance:      detail.CashBal,
				FrozenBalance:    detail.FrozenBal,
				UpdateTime:       detail.UTime,
			}, true
		}
	}
	return USDTBalance{}, false
}

func (t Trader) client(cfg trading.RuntimeConfig, apiID string) (Client, string, error) {
	creds, resolvedID, err := t.credentials(apiID)
	if err != nil {
		return Client{}, resolvedID, err
	}
	return Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  t.HTTPClient,
	}, resolvedID, nil
}

func (t Trader) credentials(apiID string) (Credentials, string, error) {
	if t.CredentialProvider != nil {
		return t.CredentialProvider.OKXCredentials(apiID)
	}
	creds := trimCredentials(t.Credentials)
	if err := creds.Validate(); err != nil {
		return Credentials{}, strings.TrimSpace(apiID), err
	}
	return creds, strings.TrimSpace(apiID), nil
}

func okxSide(action trading.Side) string {
	if action == trading.ActionShort {
		return "sell"
	}
	return "buy"
}

func attachAlgoOrders(signal trading.Signal, clOrdID string) []map[string]string {
	return AttachAlgoOrders(signal.Action, signal.Risk, clOrdID)
}

func AttachAlgoOrders(action trading.Side, risk trading.Risk, clOrdID string) []map[string]string {
	risk.Normalize()
	switch risk.Type {
	case trading.RiskTPSL:
		if risk.TPPct == nil || risk.SLPct == nil {
			return nil
		}
		return []map[string]string{{
			"attachAlgoClOrdId": attachAlgoClOrdID(clOrdID, "A"),
			"tpTriggerRatio":    signedTPRatio(action, risk.TPPct.Value),
			"tpOrdPx":           "-1",
			"tpTriggerPxType":   "mark",
			"slTriggerRatio":    pctRatio(risk.SLPct.Value),
			"slOrdPx":           "-1",
			"slTriggerPxType":   "mark",
		}}
	case trading.RiskTrailing:
		if risk.TrailingPct == nil {
			return nil
		}
		return []map[string]string{{
			"attachAlgoClOrdId": attachAlgoClOrdID(clOrdID, "T"),
			"ordType":           "move_order_stop",
			"callbackRatio":     pctRatio(risk.TrailingPct.Value),
		}}
	default:
		return nil
	}
}

func attachAlgoClOrdID(clOrdID, suffix string) string {
	clOrdID = strings.TrimSpace(clOrdID)
	limit := 32 - len(suffix)
	if limit < 0 {
		limit = 0
	}
	if len(clOrdID) > limit {
		clOrdID = clOrdID[:limit]
	}
	return clOrdID + suffix
}

func signedTPRatio(action trading.Side, pct float64) string {
	ratio := pct / 100
	if action == trading.ActionShort {
		ratio = -ratio
	}
	return trading.NormalizeFloat(ratio)
}

func pctRatio(pct float64) string {
	return trading.NormalizeFloat(pct / 100)
}

func clientOrderID(signal trading.Signal) string {
	sum := sha256.Sum256([]byte(signal.CanonicalTokenPayload() + "|" + signal.SentAt + "|" + signal.Ticker))
	short := hex.EncodeToString(sum[:])[:12]
	return strings.ToUpper(fmt.Sprintf("TV%d%s", time.Now().UTC().UnixMilli(), short))
}

func DeriveSwapInstrumentID(coinpair, ticker string) (string, string, error) {
	raw := strings.TrimSpace(coinpair)
	if raw == "" {
		raw = strings.TrimSpace(ticker)
	}
	if raw == "" {
		return "", "", fmt.Errorf("coinpair or ticker is required")
	}
	raw = strings.ToUpper(raw)
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".P")
	raw = strings.TrimSuffix(raw, ".PERP")
	raw = strings.TrimSuffix(raw, "PERP")
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.ReplaceAll(raw, "/", "-")
	raw = strings.ReplaceAll(raw, " ", "")
	if strings.HasSuffix(raw, "-SWAP") {
		return raw, baseCoin(raw), nil
	}
	if strings.Contains(raw, "-") {
		parts := strings.Split(raw, "-")
		if len(parts) >= 2 {
			instID := parts[0] + "-" + parts[1] + "-SWAP"
			return instID, parts[0], nil
		}
	}
	if strings.HasSuffix(raw, "USDT") && len(raw) > len("USDT") {
		base := strings.TrimSuffix(raw, "USDT")
		return base + "-USDT-SWAP", base, nil
	}
	return raw + "-USDT-SWAP", raw, nil
}

func baseCoin(instID string) string {
	parts := strings.Split(instID, "-")
	if len(parts) == 0 {
		return instID
	}
	return parts[0]
}

package okx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Credentials Credentials
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

func (t Trader) ExecuteSignal(ctx context.Context, signal trading.Signal, cfg trading.RuntimeConfig) (trading.OrderResult, error) {
	if !cfg.LiveTradingAllowedByEnvironment() {
		return trading.OrderResult{}, fmt.Errorf("live trading requires config env=live, OKX_ENV=live and ALLOW_LIVE_TRADING=true")
	}
	sym, ok := cfg.SymbolMeta(signal.Coinpair)
	if !ok {
		return trading.OrderResult{}, fmt.Errorf("coinpair %s is not configured", signal.Coinpair)
	}
	sz, err := trading.SizeFromUSDTNotional(signal.Amount.Value, signal.Price.Value, sym.CtVal, sym.LotSz, sym.MinSz)
	if err != nil {
		return trading.OrderResult{}, err
	}
	client := t.client(cfg)
	posSide := ""
	if cfg.PositionMode() == config.PositionLongShort {
		posSide = string(signal.Action)
	}
	if err := client.SetLeverage(ctx, SetLeverageRequest{
		InstID:  sym.InstID,
		Lever:   strconv.Itoa(signal.Leverage),
		MgnMode: cfg.MarginMode(),
		PosSide: posSide,
	}); err != nil {
		return trading.OrderResult{}, err
	}
	clOrdID := clientOrderID(signal)
	req := PlaceOrderRequest{
		InstID:         sym.InstID,
		TDMode:         cfg.MarginMode(),
		ClOrdID:        clOrdID,
		Side:           okxSide(signal.Action),
		PosSide:        posSide,
		OrdType:        "market",
		Sz:             sz,
		AttachAlgoOrds: attachAlgoOrders(signal, clOrdID),
	}
	ack, env, err := client.PlaceOrder(ctx, req)
	result := trading.OrderResult{
		SignalID: "",
		InstID:   sym.InstID,
		ClOrdID:  clOrdID,
		OrdID:    ack.OrdID,
		OKXCode:  env.Code,
		OKXMsg:   env.Msg,
	}
	if err != nil {
		return result, err
	}
	if t.Logger != nil {
		t.Logger.Info("okx order submitted", "inst_id", sym.InstID, "action", signal.Action, "cl_ord_id", clOrdID, "okx_code", env.Code)
	}
	return result, nil
}

func (t Trader) Check(ctx context.Context, cfg trading.RuntimeConfig) (map[string]any, error) {
	if err := t.Credentials.Validate(); err != nil {
		return nil, err
	}
	client := t.client(cfg)
	balance, err := client.AccountBalance(ctx)
	if err != nil {
		return nil, err
	}
	instruments, err := client.Instruments(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":           true,
		"demo":         cfg.DemoTradingHeaderEnabled(),
		"base_url":     cfg.OKXBaseURL(),
		"balance_code": balance.Code,
		"balance_msg":  balance.Msg,
		"instruments":  string(instruments.Data),
	}, nil
}

func (t Trader) client(cfg trading.RuntimeConfig) Client {
	return Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: t.Credentials,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  t.HTTPClient,
	}
}

func okxSide(action trading.Side) string {
	if action == trading.ActionShort {
		return "sell"
	}
	return "buy"
}

func attachAlgoOrders(signal trading.Signal, clOrdID string) []map[string]string {
	signal.Risk.Normalize()
	switch signal.Risk.Type {
	case trading.RiskTPSL:
		return []map[string]string{{
			"attachAlgoClOrdId": clOrdID + "A",
			"tpTriggerRatio":    signedTPRatio(signal.Action, signal.Risk.TPPct.Value),
			"tpOrdPx":           "-1",
			"tpTriggerPxType":   "mark",
			"slTriggerRatio":    pctRatio(signal.Risk.SLPct.Value),
			"slOrdPx":           "-1",
			"slTriggerPxType":   "mark",
		}}
	case trading.RiskTrailing:
		return []map[string]string{{
			"attachAlgoClOrdId": clOrdID + "T",
			"ordType":           "move_order_stop",
			"callbackRatio":     pctRatio(signal.Risk.TrailingPct.Value),
		}}
	default:
		return nil
	}
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

package trading

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Side string

const (
	ActionLong  Side = "long"
	ActionShort Side = "short"
)

type RiskType string

const (
	RiskNone     RiskType = "none"
	RiskTPSL     RiskType = "tp_sl"
	RiskTrailing RiskType = "trailing"
)

type FlexibleFloat struct {
	Value float64
	Set   bool
}

func NewFlexibleFloat(v float64) FlexibleFloat {
	return FlexibleFloat{Value: v, Set: true}
}

func (f *FlexibleFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*f = FlexibleFloat{}
		return nil
	}
	var raw string
	if b[0] == '"' {
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
	} else {
		raw = string(b)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*f = FlexibleFloat{}
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("invalid number %q", raw)
	}
	*f = FlexibleFloat{Value: v, Set: true}
	return nil
}

func (f FlexibleFloat) MarshalJSON() ([]byte, error) {
	if !f.Set {
		return []byte("null"), nil
	}
	return []byte(strconv.Quote(NormalizeFloat(f.Value))), nil
}

type Risk struct {
	Type        RiskType       `json:"type"`
	TPPct       *FlexibleFloat `json:"tp_pct,omitempty"`
	SLPct       *FlexibleFloat `json:"sl_pct,omitempty"`
	TrailingPct *FlexibleFloat `json:"trailing_pct,omitempty"`
}

type Signal struct {
	Action   Side          `json:"action"`
	Coinpair string        `json:"coinpair"`
	Price    FlexibleFloat `json:"price"`
	SentAt   string        `json:"sent_at"`
	Ticker   string        `json:"ticker"`
	Leverage int           `json:"leverage"`
	Amount   FlexibleFloat `json:"amount"`
	Risk     Risk          `json:"risk,omitempty"`
	Token    string        `json:"token"`
}

type TemplateRequest struct {
	PriceSource string        `json:"price_source"`
	Leverage    int           `json:"leverage"`
	Amount      FlexibleFloat `json:"amount"`
}

type TemplateResponse struct {
	JSON  string `json:"json"`
	Token string `json:"token"`
}

type OrderResult struct {
	SignalID string `json:"signal_id"`
	InstID   string `json:"inst_id"`
	ClOrdID  string `json:"cl_ord_id"`
	OrdID    string `json:"ord_id,omitempty"`
	OKXCode  string `json:"okx_code,omitempty"`
	OKXMsg   string `json:"okx_msg,omitempty"`
}

type Executor interface {
	ExecuteSignal(ctx context.Context, signal Signal, cfg RuntimeConfig) (OrderResult, error)
	Check(ctx context.Context, cfg RuntimeConfig) (map[string]any, error)
}

type RuntimeConfig interface {
	SymbolMeta(coinpair string) (SymbolInfo, bool)
	DemoTradingHeaderEnabled() bool
	LiveTradingAllowedByEnvironment() bool
	OKXBaseURL() string
	MarginMode() string
	PositionMode() string
}

type SymbolInfo struct {
	Coinpair string
	InstID   string
	CtVal    float64
	LotSz    float64
	MinSz    float64
}

func (r *Risk) Normalize() {
	r.Type = RiskType(strings.ToLower(strings.TrimSpace(string(r.Type))))
	if r.Type == "" {
		r.Type = RiskNone
	}
}

func (s *Signal) Normalize() {
	switch strings.ToLower(strings.TrimSpace(string(s.Action))) {
	case "buy", "long":
		s.Action = ActionLong
	case "sell", "short":
		s.Action = ActionShort
	default:
		s.Action = Side(strings.ToLower(strings.TrimSpace(string(s.Action))))
	}
	s.Coinpair = strings.ToUpper(strings.TrimSpace(s.Coinpair))
	s.Ticker = strings.TrimSpace(s.Ticker)
	if s.Coinpair == "" {
		s.Coinpair = strings.ToUpper(s.Ticker)
	}
	s.Token = strings.TrimSpace(s.Token)
	s.Risk.Normalize()
}

func (r Risk) Validate() error {
	switch r.Type {
	case RiskNone:
		return nil
	case RiskTPSL:
		if r.TPPct == nil || !r.TPPct.Set || r.TPPct.Value <= 0 {
			return errors.New("risk.tp_pct must be positive for tp_sl")
		}
		if r.SLPct == nil || !r.SLPct.Set || r.SLPct.Value <= 0 {
			return errors.New("risk.sl_pct must be positive for tp_sl")
		}
		return nil
	case RiskTrailing:
		if r.TrailingPct == nil || !r.TrailingPct.Set || r.TrailingPct.Value <= 0 {
			return errors.New("risk.trailing_pct must be positive for trailing")
		}
		return nil
	default:
		return fmt.Errorf("unsupported risk.type %q", r.Type)
	}
}

func (s Signal) Validate(now time.Time, ttl time.Duration, cfg RuntimeConfig) error {
	s.Normalize()
	switch s.Action {
	case ActionLong, ActionShort:
	default:
		return fmt.Errorf("action must be %q or %q", ActionLong, ActionShort)
	}
	if s.Coinpair == "" && strings.TrimSpace(s.Ticker) == "" {
		return errors.New("coinpair or ticker is required")
	}
	if !s.Price.Set || s.Price.Value <= 0 {
		return errors.New("price must be positive")
	}
	if s.Leverage <= 0 {
		return errors.New("leverage must be positive")
	}
	if !s.Amount.Set || s.Amount.Value <= 0 {
		return errors.New("amount must be positive")
	}
	if err := s.Risk.Validate(); err != nil {
		return err
	}
	sentAt, err := ParseTradingViewTime(s.SentAt)
	if err != nil {
		return err
	}
	age := now.UTC().Sub(sentAt)
	if age < -30*time.Second {
		return errors.New("sent_at is too far in the future")
	}
	if ttl > 0 && age > ttl {
		return errors.New("sent_at is expired")
	}
	return nil
}

func ParseTradingViewTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, errors.New("sent_at is required")
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC(), nil
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		if ms > 1_000_000_000_000 {
			return time.UnixMilli(ms).UTC(), nil
		}
		return time.Unix(ms, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("sent_at %q must be RFC3339 or unix timestamp", v)
}

func NormalizeFloat(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 8, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func CanonicalRisk(r Risk) string {
	r.Normalize()
	switch r.Type {
	case RiskTPSL:
		return strings.Join([]string{
			string(r.Type),
			NormalizeOptionalFloat(r.TPPct),
			NormalizeOptionalFloat(r.SLPct),
		}, "|")
	case RiskTrailing:
		return strings.Join([]string{
			string(r.Type),
			NormalizeOptionalFloat(r.TrailingPct),
		}, "|")
	default:
		return string(RiskNone)
	}
}

func NormalizeOptionalFloat(v *FlexibleFloat) string {
	if v == nil || !v.Set {
		return ""
	}
	return NormalizeFloat(v.Value)
}

func CanonicalTokenPayload(leverage int, amount FlexibleFloat) string {
	return strings.Join([]string{
		"v2",
		strconv.Itoa(leverage),
		NormalizeFloat(amount.Value),
	}, "\n")
}

func (s Signal) CanonicalTokenPayload() string {
	return CanonicalTokenPayload(s.Leverage, s.Amount)
}

func (t TemplateRequest) CanonicalTokenPayload() string {
	return CanonicalTokenPayload(t.Leverage, t.Amount)
}

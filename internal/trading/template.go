package trading

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TokenGenerator interface {
	Generate(payload string) string
}

func BuildTemplate(req TemplateRequest, generator TokenGenerator) (TemplateResponse, error) {
	req.Action = Side(strings.ToLower(strings.TrimSpace(string(req.Action))))
	req.Coinpair = strings.ToUpper(strings.TrimSpace(req.Coinpair))
	req.PriceSource = strings.ToLower(strings.TrimSpace(req.PriceSource))
	req.Risk.Normalize()
	if req.PriceSource == "" {
		req.PriceSource = "close"
	}
	if req.PriceSource != "close" && req.PriceSource != "high" && req.PriceSource != "low" {
		return TemplateResponse{}, fmt.Errorf("price_source must be close, high or low")
	}
	signal := Signal{
		Action:   req.Action,
		Coinpair: req.Coinpair,
		Price:    NewFlexibleFloat(0),
		SentAt:   "{{timenow}}",
		Ticker:   "{{ticker}}",
		Leverage: req.Leverage,
		Amount:   req.Amount,
		Risk:     req.Risk,
	}
	switch signal.Action {
	case ActionLong, ActionShort:
	default:
		return TemplateResponse{}, fmt.Errorf("action must be long or short")
	}
	if signal.Leverage <= 0 {
		return TemplateResponse{}, fmt.Errorf("leverage must be positive")
	}
	if !signal.Amount.Set || signal.Amount.Value <= 0 {
		return TemplateResponse{}, fmt.Errorf("amount must be positive")
	}
	if err := signal.Risk.Validate(); err != nil {
		return TemplateResponse{}, err
	}
	token := generator.Generate(req.CanonicalTokenPayload())
	payload := map[string]any{
		"action":   signal.Action,
		"coinpair": signal.Coinpair,
		"price":    "{{" + req.PriceSource + "}}",
		"sent_at":  signal.SentAt,
		"ticker":   signal.Ticker,
		"leverage": signal.Leverage,
		"amount":   signal.Amount,
		"risk":     signal.Risk,
		"token":    token,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return TemplateResponse{}, err
	}
	return TemplateResponse{JSON: string(b), Token: token}, nil
}

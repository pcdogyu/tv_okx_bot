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
	req.PriceSource = strings.ToLower(strings.TrimSpace(req.PriceSource))
	req.APIID = strings.TrimSpace(req.APIID)
	if req.PriceSource == "" {
		req.PriceSource = "close"
	}
	if req.PriceSource != "close" && req.PriceSource != "high" && req.PriceSource != "low" {
		return TemplateResponse{}, fmt.Errorf("price_source must be close, high or low")
	}
	signal := Signal{
		Action:   "{{strategy.order.action}}",
		APIID:    req.APIID,
		Coinpair: "{{ticker}}",
		Price:    NewFlexibleFloat(0),
		SentAt:   "{{timenow}}",
		Ticker:   "{{ticker}}",
		Leverage: req.Leverage,
		Amount:   req.Amount,
	}
	if signal.Leverage <= 0 {
		return TemplateResponse{}, fmt.Errorf("leverage must be positive")
	}
	if !signal.Amount.Set || signal.Amount.Value <= 0 {
		return TemplateResponse{}, fmt.Errorf("amount must be positive")
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
		"token":    token,
	}
	if req.APIID != "" {
		payload["api_id"] = req.APIID
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return TemplateResponse{}, err
	}
	return TemplateResponse{JSON: string(b), Token: token}, nil
}

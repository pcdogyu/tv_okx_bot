package trading

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TokenGenerator interface {
	Generate(payload string) string
}

type alertTemplatePayload struct {
	SentAt    string `json:"sent_at"`
	APIID     string `json:"api_id,omitempty"`
	Action    string `json:"action"`
	Ticker    string `json:"ticker"`
	Coinpair  string `json:"coinpair"`
	Price     string `json:"price"`
	Exchange  string `json:"exchange"`
	Interval  string `json:"interval"`
	Condition string `json:"condition"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Token     string `json:"token"`
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
	token := generator.Generate(req.CanonicalTokenPayload())
	payload := alertTemplatePayload{
		SentAt:    signal.SentAt,
		APIID:     req.APIID,
		Action:    string(signal.Action),
		Ticker:    signal.Ticker,
		Coinpair:  signal.Coinpair,
		Price:     "{{" + req.PriceSource + "}}",
		Exchange:  "{{exchange}}",
		Interval:  "{{interval}}",
		Condition: "{{strategy.order.comment}}",
		Text:      "{{strategy.order.alert_message}}",
		Source:    "tradingview",
		Token:     token,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return TemplateResponse{}, err
	}
	return TemplateResponse{JSON: string(b), Token: token}, nil
}

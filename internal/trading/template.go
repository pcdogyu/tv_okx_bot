package trading

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type TokenGenerator interface {
	Generate(payload string) string
}

type alertTemplatePayload struct {
	SentAt         string `json:"sent_at"`
	APIID          string `json:"api_id,omitempty"`
	TargetExchange string `json:"target_exchange,omitempty"`
	Action         string `json:"action"`
	Ticker         string `json:"ticker"`
	Coinpair       string `json:"coinpair"`
	Price          string `json:"price"`
	Exchange       string `json:"exchange"`
	Interval       string `json:"interval"`
	Condition      string `json:"condition"`
	Text           string `json:"text"`
	Source         string `json:"source"`
	TokenNonce     string `json:"token_nonce"`
	Token          string `json:"token"`
}

func BuildTemplate(req TemplateRequest, generator TokenGenerator) (TemplateResponse, error) {
	req.PriceSource = strings.ToLower(strings.TrimSpace(req.PriceSource))
	req.APIID = strings.TrimSpace(req.APIID)
	req.TargetExchange = NormalizeExchange(req.TargetExchange)
	if req.PriceSource == "" {
		req.PriceSource = "close"
	}
	if !ValidTargetExchange(req.TargetExchange) {
		return TemplateResponse{}, fmt.Errorf("target_exchange must be okx or binance")
	}
	if req.PriceSource != "close" && req.PriceSource != "high" && req.PriceSource != "low" {
		return TemplateResponse{}, fmt.Errorf("price_source must be close, high or low")
	}
	signal := Signal{
		Action:         "{{strategy.order.action}}",
		APIID:          req.APIID,
		TargetExchange: req.TargetExchange,
		Coinpair:       "{{ticker}}",
		Price:          NewFlexibleFloat(0),
		SentAt:         "{{timenow}}",
		Ticker:         "{{ticker}}",
		Leverage:       req.Leverage,
		Amount:         req.Amount,
	}
	tokenNonce, err := newTemplateTokenNonce()
	if err != nil {
		return TemplateResponse{}, err
	}
	signal.TokenNonce = tokenNonce
	token := generator.Generate(signal.CanonicalNonceWebhookTokenPayload())
	payload := alertTemplatePayload{
		SentAt:         signal.SentAt,
		APIID:          req.APIID,
		TargetExchange: req.TargetExchange,
		Action:         string(signal.Action),
		Ticker:         signal.Ticker,
		Coinpair:       signal.Coinpair,
		Price:          "{{" + req.PriceSource + "}}",
		Exchange:       "{{exchange}}",
		Interval:       "{{interval}}",
		Condition:      "{{strategy.order.comment}}",
		Text:           "{{strategy.order.alert_message}}",
		Source:         "tradingview",
		TokenNonce:     tokenNonce,
		Token:          token,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return TemplateResponse{}, err
	}
	return TemplateResponse{JSON: string(b), Token: token, TokenNonce: tokenNonce}, nil
}

func newTemplateTokenNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

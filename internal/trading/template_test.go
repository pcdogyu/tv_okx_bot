package trading

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pcdogyu/tv_okx_bot/internal/security"
)

func TestBuildTemplateProducesValidJSONAndToken(t *testing.T) {
	req := TemplateRequest{
		PriceSource: "close",
		Leverage:    5,
		Amount:      NewFlexibleFloat(100),
	}
	tokenSvc := security.NewTokenService("unit-test-secret")
	resp, err := BuildTemplate(req, tokenSvc)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["price"] != "{{close}}" || payload["sent_at"] != "{{timenow}}" || payload["ticker"] != "{{ticker}}" {
		t.Fatalf("unexpected placeholders: %#v", payload)
	}
	if payload["action"] != "{{strategy.order.action}}" || payload["coinpair"] != "{{ticker}}" {
		t.Fatalf("unexpected dynamic placeholders: %#v", payload)
	}
	if payload["exchange"] != "{{exchange}}" || payload["interval"] != "{{interval}}" {
		t.Fatalf("unexpected TradingView market placeholders: %#v", payload)
	}
	if payload["condition"] != "{{strategy.order.comment}}" || payload["text"] != "{{strategy.order.alert_message}}" || payload["source"] != "tradingview" {
		t.Fatalf("unexpected TradingView order fields: %#v", payload)
	}
	if _, ok := payload["risk"]; ok {
		t.Fatalf("risk field should not be present: %#v", payload)
	}
	if _, ok := payload["amount"]; ok {
		t.Fatalf("amount field should not be present: %#v", payload)
	}
	if _, ok := payload["leverage"]; ok {
		t.Fatalf("leverage field should not be present: %#v", payload)
	}
	nonce, ok := payload["token_nonce"].(string)
	if !ok || nonce == "" || nonce != resp.TokenNonce {
		t.Fatalf("token_nonce not included: %#v", payload)
	}
	if !tokenSvc.Validate(CanonicalTargetWebhookTokenPayloadWithNonce(req.TargetExchange, req.APIID, resp.TokenNonce), resp.Token) {
		t.Fatal("generated token did not validate against canonical template payload")
	}
	if strings.LastIndex(resp.JSON, `"token"`) < strings.LastIndex(resp.JSON, `"source"`) {
		t.Fatalf("token should be the final JSON field: %s", resp.JSON)
	}
}

func TestBuildTemplateCanBindSelectedAPI(t *testing.T) {
	req := TemplateRequest{
		PriceSource: "low",
		APIID:       "backup",
		Leverage:    3,
		Amount:      NewFlexibleFloat(50),
	}
	tokenSvc := security.NewTokenService("unit-test-secret")
	resp, err := BuildTemplate(req, tokenSvc)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["api_id"] != "backup" {
		t.Fatalf("api_id not included: %#v", payload)
	}
	if !tokenSvc.Validate(CanonicalTargetWebhookTokenPayloadWithNonce(req.TargetExchange, req.APIID, resp.TokenNonce), resp.Token) {
		t.Fatal("generated token did not validate against selected api payload")
	}
	withoutAPI := req
	withoutAPI.APIID = ""
	if tokenSvc.Validate(CanonicalTargetWebhookTokenPayloadWithNonce(withoutAPI.TargetExchange, withoutAPI.APIID, resp.TokenNonce), resp.Token) {
		t.Fatal("token should be bound to api_id when api_id is selected")
	}
}

func TestBuildTemplateCanBindTargetExchange(t *testing.T) {
	req := TemplateRequest{
		TargetExchange: ExchangeBinance,
		PriceSource:    "close",
		APIID:          "binance-main",
	}
	tokenSvc := security.NewTokenService("unit-test-secret")
	resp, err := BuildTemplate(req, tokenSvc)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.JSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["target_exchange"] != ExchangeBinance || payload["api_id"] != "binance-main" {
		t.Fatalf("target exchange not included: %#v", payload)
	}
	if !tokenSvc.Validate(CanonicalTargetWebhookTokenPayloadWithNonce(req.TargetExchange, req.APIID, resp.TokenNonce), resp.Token) {
		t.Fatal("generated token did not validate against Binance target payload")
	}
	okxReq := req
	okxReq.TargetExchange = ExchangeOKX
	if tokenSvc.Validate(CanonicalTargetWebhookTokenPayloadWithNonce(okxReq.TargetExchange, okxReq.APIID, resp.TokenNonce), resp.Token) {
		t.Fatal("Binance target token should not validate as OKX token")
	}
}

func TestBuildTemplateGeneratesDifferentTokenEachTime(t *testing.T) {
	req := TemplateRequest{
		TargetExchange: ExchangeBinance,
		PriceSource:    "close",
		APIID:          "binance-main",
	}
	tokenSvc := security.NewTokenService("unit-test-secret")
	first, err := BuildTemplate(req, tokenSvc)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTemplate(req, tokenSvc)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatalf("template tokens should differ for repeated generation: %q", first.Token)
	}
	if first.TokenNonce == "" || second.TokenNonce == "" || first.TokenNonce == second.TokenNonce {
		t.Fatalf("template token nonces should differ: first=%q second=%q", first.TokenNonce, second.TokenNonce)
	}
}

func TestCanonicalTokenPayloadDoesNotBindActionOrCoinpair(t *testing.T) {
	longBTC := Signal{Action: ActionLong, Coinpair: "BTC", Leverage: 5, Amount: NewFlexibleFloat(100)}
	shortETH := Signal{Action: ActionShort, Coinpair: "ETH", Leverage: 5, Amount: NewFlexibleFloat(100)}
	if longBTC.CanonicalTokenPayload() != shortETH.CanonicalTokenPayload() {
		t.Fatalf("token payload should not depend on action or coinpair")
	}
}

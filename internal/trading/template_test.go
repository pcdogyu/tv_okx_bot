package trading

import (
	"encoding/json"
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
	if _, ok := payload["risk"]; ok {
		t.Fatalf("risk field should not be present: %#v", payload)
	}
	if !tokenSvc.Validate(req.CanonicalTokenPayload(), resp.Token) {
		t.Fatal("generated token did not validate against canonical template payload")
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
	if !tokenSvc.Validate(req.CanonicalTokenPayload(), resp.Token) {
		t.Fatal("generated token did not validate against selected api payload")
	}
	withoutAPI := req
	withoutAPI.APIID = ""
	if tokenSvc.Validate(withoutAPI.CanonicalTokenPayload(), resp.Token) {
		t.Fatal("token should be bound to api_id when api_id is selected")
	}
}

func TestCanonicalTokenPayloadDoesNotBindActionOrCoinpair(t *testing.T) {
	longBTC := Signal{Action: ActionLong, Coinpair: "BTC", Leverage: 5, Amount: NewFlexibleFloat(100)}
	shortETH := Signal{Action: ActionShort, Coinpair: "ETH", Leverage: 5, Amount: NewFlexibleFloat(100)}
	if longBTC.CanonicalTokenPayload() != shortETH.CanonicalTokenPayload() {
		t.Fatalf("token payload should not depend on action or coinpair")
	}
}

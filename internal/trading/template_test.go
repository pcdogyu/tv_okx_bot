package trading

import (
	"encoding/json"
	"testing"

	"github.com/pcdogyu/tv_okx_bot/internal/security"
)

func TestBuildTemplateProducesValidJSONAndToken(t *testing.T) {
	tp := NewFlexibleFloat(2)
	sl := NewFlexibleFloat(1)
	req := TemplateRequest{
		Action:      ActionLong,
		Coinpair:    "btc",
		PriceSource: "close",
		Leverage:    5,
		Amount:      NewFlexibleFloat(100),
		Risk:        Risk{Type: RiskTPSL, TPPct: &tp, SLPct: &sl},
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
	req.Coinpair = "BTC"
	if !tokenSvc.Validate(req.CanonicalTokenPayload(), resp.Token) {
		t.Fatal("generated token did not validate against canonical template payload")
	}
}

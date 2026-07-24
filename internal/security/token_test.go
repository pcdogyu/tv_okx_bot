package security

import "testing"

func TestTokenServiceGenerateAndValidate(t *testing.T) {
	svc := NewTokenService("unit-test-secret")
	payload := "long\nBTC\n5\n100\ntp_sl|2|1"
	token := svc.Generate(payload)
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64", len(token))
	}
	if !svc.Validate(payload, token) {
		t.Fatal("expected token to validate")
	}
	if svc.Validate(payload+"x", token) {
		t.Fatal("expected modified payload to fail validation")
	}
	if svc.Validate(payload, token[:63]) {
		t.Fatal("expected short token to fail validation")
	}
}

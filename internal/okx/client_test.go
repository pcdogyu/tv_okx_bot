package okx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSetLeverageSignsPrivateDemoRequest(t *testing.T) {
	fixedNow := time.Date(2026, 7, 24, 3, 0, 0, 123000000, time.UTC)
	secret := "secret"
	var saw bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = true
		if r.URL.Path != "/api/v5/account/set-leverage" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatal("missing demo trading header")
		}
		if r.Header.Get("OK-ACCESS-KEY") != "key" || r.Header.Get("OK-ACCESS-PASSPHRASE") != "pass" {
			t.Fatal("missing OKX auth headers")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := json.Marshal(SetLeverageRequest{InstID: "BTC-USDT-SWAP", Lever: "5", MgnMode: "isolated"})
		timestamp := fixedNow.UTC().Format("2006-01-02T15:04:05.000Z")
		wantSign := sign(timestamp, http.MethodPost, "/api/v5/account/set-leverage", string(bodyBytes), secret)
		if r.Header.Get("OK-ACCESS-TIMESTAMP") != timestamp || r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatal("invalid OKX signature headers")
		}
		if body["instId"] != "BTC-USDT-SWAP" || body["lever"] != "5" || body["mgnMode"] != "isolated" {
			t.Fatalf("unexpected body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{}]}`))
	}))
	defer ts.Close()

	client := Client{
		BaseURL: ts.URL,
		Credentials: Credentials{
			APIKey:     "key",
			SecretKey:  secret,
			Passphrase: "pass",
		},
		Demo:       true,
		HTTPClient: ts.Client(),
		Now:        func() time.Time { return fixedNow },
	}
	err := client.SetLeverage(context.Background(), SetLeverageRequest{
		InstID:  "BTC-USDT-SWAP",
		Lever:   "5",
		MgnMode: "isolated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saw {
		t.Fatal("server did not receive request")
	}
}

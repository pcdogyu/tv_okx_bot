package binance

import (
	"path/filepath"
	"testing"
)

func TestCredentialStoreUsesActiveAccountWhenAPIIDIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binance-credentials.json")
	store, err := NewCredentialStore(path, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAccount(CredentialAccountUpdate{
		ID:     "tvbot",
		Name:   "binance-moni",
		Active: true,
		Credentials: Credentials{
			APIKey:    "active-key",
			SecretKey: "active-secret",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAccount(CredentialAccountUpdate{
		ID:     "backup",
		Name:   "backup",
		Active: false,
		Credentials: Credentials{
			APIKey:    "backup-key",
			SecretKey: "backup-secret",
		},
	}); err != nil {
		t.Fatal(err)
	}
	creds, id, err := store.BinanceCredentials("")
	if err != nil {
		t.Fatal(err)
	}
	if id != "tvbot" || creds.APIKey != "active-key" {
		t.Fatalf("empty api id should use active account id=%q creds=%#v", id, creds)
	}
	reloaded, err := NewCredentialStore(path, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	creds, id, err = reloaded.BinanceCredentials("")
	if err != nil {
		t.Fatal(err)
	}
	if id != "tvbot" || creds.APIKey != "active-key" {
		t.Fatalf("reloaded empty api id should use active account id=%q creds=%#v", id, creds)
	}
}

package okx

import (
	"path/filepath"
	"testing"
)

func TestCredentialStoreUpdateMasksAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okx-credentials.json")
	store, err := NewCredentialStore(path, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Update(Credentials{
		APIKey:     "abcd12345678wxyz",
		SecretKey:  "secret",
		Passphrase: "pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.APIKeyMasked != "abcd...wxyz" || !status.SecretKeySet || !status.PassphraseSet {
		t.Fatalf("bad status: %#v", status)
	}
	reloaded, err := NewCredentialStore(path, Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	creds := reloaded.OKXCredentials()
	if creds.APIKey != "abcd12345678wxyz" || creds.SecretKey != "secret" || creds.Passphrase != "pass" {
		t.Fatalf("bad reloaded credentials: %#v", creds)
	}
}

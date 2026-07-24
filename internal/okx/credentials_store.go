package okx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CredentialProvider interface {
	OKXCredentials() Credentials
}

type CredentialStore struct {
	mu        sync.RWMutex
	path      string
	creds     Credentials
	source    string
	updatedAt time.Time
}

type CredentialStatus struct {
	Configured    bool      `json:"configured"`
	APIKeyMasked  string    `json:"api_key_masked,omitempty"`
	SecretKeySet  bool      `json:"secret_key_set"`
	PassphraseSet bool      `json:"passphrase_set"`
	Source        string    `json:"source,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type credentialFile struct {
	APIKey     string    `json:"api_key"`
	SecretKey  string    `json:"secret_key"`
	Passphrase string    `json:"passphrase"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewCredentialStore(path string, initial Credentials) (*CredentialStore, error) {
	s := &CredentialStore{
		path:      path,
		creds:     trimCredentials(initial),
		source:    "env",
		updatedAt: time.Now().UTC(),
	}
	if !credentialsConfigured(s.creds) {
		s.source = ""
		s.updatedAt = time.Time{}
	}
	if path == "" {
		return s, nil
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *CredentialStore) OKXCredentials() Credentials {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.creds
}

func (s *CredentialStore) Status() CredentialStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return credentialStatus(s.creds, s.source, s.updatedAt)
}

func (s *CredentialStore) Update(creds Credentials) (CredentialStatus, error) {
	creds = trimCredentials(creds)
	if !credentialsConfigured(creds) {
		return CredentialStatus{}, errors.New("api_key, secret_key and passphrase are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds = creds
	s.source = "file"
	s.updatedAt = time.Now().UTC()
	if s.path != "" {
		if err := s.saveLocked(); err != nil {
			return CredentialStatus{}, err
		}
	}
	return credentialStatus(s.creds, s.source, s.updatedAt), nil
}

func (s *CredentialStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var file credentialFile
	if err := json.Unmarshal(b, &file); err != nil {
		return err
	}
	creds := trimCredentials(Credentials{
		APIKey:     file.APIKey,
		SecretKey:  file.SecretKey,
		Passphrase: file.Passphrase,
	})
	if !credentialsConfigured(creds) {
		return nil
	}
	s.creds = creds
	s.source = "file"
	s.updatedAt = file.UpdatedAt.UTC()
	if s.updatedAt.IsZero() {
		s.updatedAt = time.Now().UTC()
	}
	return nil
}

func (s *CredentialStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil && filepath.Dir(s.path) != "." {
		return err
	}
	file := credentialFile{
		APIKey:     s.creds.APIKey,
		SecretKey:  s.creds.SecretKey,
		Passphrase: s.creds.Passphrase,
		UpdatedAt:  s.updatedAt,
	}
	b, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func credentialStatus(creds Credentials, source string, updatedAt time.Time) CredentialStatus {
	return CredentialStatus{
		Configured:    credentialsConfigured(creds),
		APIKeyMasked:  maskAPIKey(creds.APIKey),
		SecretKeySet:  strings.TrimSpace(creds.SecretKey) != "",
		PassphraseSet: strings.TrimSpace(creds.Passphrase) != "",
		Source:        source,
		UpdatedAt:     updatedAt,
	}
}

func trimCredentials(creds Credentials) Credentials {
	return Credentials{
		APIKey:     strings.TrimSpace(creds.APIKey),
		SecretKey:  strings.TrimSpace(creds.SecretKey),
		Passphrase: strings.TrimSpace(creds.Passphrase),
	}
}

func credentialsConfigured(creds Credentials) bool {
	return strings.TrimSpace(creds.APIKey) != "" &&
		strings.TrimSpace(creds.SecretKey) != "" &&
		strings.TrimSpace(creds.Passphrase) != ""
}

func maskAPIKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return "****" + apiKey
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}

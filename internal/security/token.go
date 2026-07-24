package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const Salt = "Yuhao@jiansutech.com"

type TokenService struct {
	secret string
}

func NewTokenService(secret string) TokenService {
	return TokenService{secret: secret}
}

func (s TokenService) Generate(payload string) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte(Salt))
	mac.Write([]byte("\n"))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s TokenService) Validate(payload, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if len(token) != 64 {
		return false
	}
	expected := s.Generate(payload)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

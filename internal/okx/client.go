package okx

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Credentials struct {
	APIKey     string
	SecretKey  string
	Passphrase string
}

func (c Credentials) Validate() error {
	if c.APIKey == "" || c.SecretKey == "" || c.Passphrase == "" {
		return errors.New("OKX API credentials are incomplete")
	}
	return nil
}

type Client struct {
	BaseURL     string
	Credentials Credentials
	Demo        bool
	HTTPClient  *http.Client
	Now         func() time.Time
}

type Envelope struct {
	Code    string          `json:"code"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
	InTime  string          `json:"inTime,omitempty"`
	OutTime string          `json:"outTime,omitempty"`
}

func (e Envelope) OK() bool {
	return e.Code == "0"
}

func (c Client) Do(ctx context.Context, method, path string, query url.Values, body any, private bool) (Envelope, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://www.okx.com"
	}
	requestPath := path
	if len(query) > 0 {
		requestPath += "?" + query.Encode()
	}
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return Envelope{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+requestPath, bytes.NewReader(bodyBytes))
	if err != nil {
		return Envelope{}, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Demo {
		req.Header.Set("x-simulated-trading", "1")
	}
	if private {
		if err := c.Credentials.Validate(); err != nil {
			return Envelope{}, err
		}
		ts := c.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		req.Header.Set("OK-ACCESS-KEY", c.Credentials.APIKey)
		req.Header.Set("OK-ACCESS-PASSPHRASE", c.Credentials.Passphrase)
		req.Header.Set("OK-ACCESS-TIMESTAMP", ts)
		req.Header.Set("OK-ACCESS-SIGN", sign(ts, method, requestPath, string(bodyBytes), c.Credentials.SecretKey))
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Envelope{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Envelope{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Envelope{}, fmt.Errorf("okx http status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var env Envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return Envelope{}, fmt.Errorf("decode okx response: %w", err)
	}
	if !env.OK() {
		return env, fmt.Errorf("okx code %s: %s", env.Code, env.Msg)
	}
	return env, nil
}

func sign(timestamp, method, requestPath, body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + strings.ToUpper(method) + requestPath + body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type SetLeverageRequest struct {
	InstID  string `json:"instId"`
	Lever   string `json:"lever"`
	MgnMode string `json:"mgnMode"`
	PosSide string `json:"posSide,omitempty"`
}

type PlaceOrderRequest struct {
	InstID         string              `json:"instId"`
	TDMode         string              `json:"tdMode"`
	ClOrdID        string              `json:"clOrdId"`
	Side           string              `json:"side"`
	PosSide        string              `json:"posSide,omitempty"`
	OrdType        string              `json:"ordType"`
	Sz             string              `json:"sz"`
	AttachAlgoOrds []map[string]string `json:"attachAlgoOrds,omitempty"`
}

type OrderAck struct {
	ClOrdID string `json:"clOrdId"`
	OrdID   string `json:"ordId"`
	SCode   string `json:"sCode"`
	SMsg    string `json:"sMsg"`
}

type Instrument struct {
	InstID string `json:"instId"`
	CtVal  string `json:"ctVal"`
	LotSz  string `json:"lotSz"`
	MinSz  string `json:"minSz"`
}

func (c Client) SetLeverage(ctx context.Context, req SetLeverageRequest) error {
	_, err := c.Do(ctx, http.MethodPost, "/api/v5/account/set-leverage", nil, req, true)
	return err
}

func (c Client) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (OrderAck, Envelope, error) {
	env, err := c.Do(ctx, http.MethodPost, "/api/v5/trade/order", nil, req, true)
	if err != nil {
		return OrderAck{}, env, err
	}
	var data []OrderAck
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return OrderAck{}, env, err
	}
	if len(data) == 0 {
		return OrderAck{}, env, errors.New("okx order response data is empty")
	}
	if data[0].SCode != "" && data[0].SCode != "0" {
		return data[0], env, fmt.Errorf("okx order rejected %s: %s", data[0].SCode, data[0].SMsg)
	}
	return data[0], env, nil
}

func (c Client) AccountBalance(ctx context.Context) (Envelope, error) {
	q := url.Values{}
	q.Set("ccy", "USDT")
	return c.Do(ctx, http.MethodGet, "/api/v5/account/balance", q, nil, true)
}

func (c Client) Instruments(ctx context.Context) (Envelope, error) {
	q := url.Values{}
	q.Set("instType", "SWAP")
	return c.Do(ctx, http.MethodGet, "/api/v5/public/instruments", q, nil, false)
}

func (c Client) SwapInstrument(ctx context.Context, instID string) (Instrument, error) {
	q := url.Values{}
	q.Set("instType", "SWAP")
	q.Set("instId", strings.ToUpper(strings.TrimSpace(instID)))
	env, err := c.Do(ctx, http.MethodGet, "/api/v5/public/instruments", q, nil, false)
	if err != nil {
		return Instrument{}, err
	}
	var data []Instrument
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return Instrument{}, err
	}
	for _, inst := range data {
		if strings.EqualFold(inst.InstID, instID) {
			return inst, nil
		}
	}
	if len(data) > 0 {
		return data[0], nil
	}
	return Instrument{}, fmt.Errorf("okx instrument %s not found", instID)
}

func (i Instrument) SymbolInfo() (tradingSymbolInfo, error) {
	ctVal, err := strconv.ParseFloat(i.CtVal, 64)
	if err != nil {
		return tradingSymbolInfo{}, fmt.Errorf("invalid ctVal %q: %w", i.CtVal, err)
	}
	lotSz, err := strconv.ParseFloat(i.LotSz, 64)
	if err != nil {
		return tradingSymbolInfo{}, fmt.Errorf("invalid lotSz %q: %w", i.LotSz, err)
	}
	minSz, err := strconv.ParseFloat(i.MinSz, 64)
	if err != nil {
		return tradingSymbolInfo{}, fmt.Errorf("invalid minSz %q: %w", i.MinSz, err)
	}
	return tradingSymbolInfo{InstID: i.InstID, CtVal: ctVal, LotSz: lotSz, MinSz: minSz}, nil
}

type tradingSymbolInfo struct {
	InstID string
	CtVal  float64
	LotSz  float64
	MinSz  float64
}

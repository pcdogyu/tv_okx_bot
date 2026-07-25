package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	APIKey    string
	SecretKey string
}

func (c Credentials) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return errors.New("Binance API credentials are incomplete")
	}
	return nil
}

type Client struct {
	BaseURL     string
	Credentials Credentials
	HTTPClient  *http.Client
	Now         func() time.Time
}

type APIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e APIError) Error() string {
	if e.Code == 0 && e.Msg == "" {
		return "binance api error"
	}
	return fmt.Sprintf("binance code %d: %s", e.Code, e.Msg)
}

type Balance struct {
	AccountAlias       string `json:"accountAlias"`
	Asset              string `json:"asset"`
	Balance            string `json:"balance"`
	CrossWalletBalance string `json:"crossWalletBalance"`
	CrossUnPnl         string `json:"crossUnPnl"`
	AvailableBalance   string `json:"availableBalance"`
	MaxWithdrawAmount  string `json:"maxWithdrawAmount"`
	MarginAvailable    bool   `json:"marginAvailable"`
	UpdateTime         int64  `json:"updateTime"`
}

type Position struct {
	Symbol                 string `json:"symbol"`
	PositionSide           string `json:"positionSide"`
	PositionAmt            string `json:"positionAmt"`
	EntryPrice             string `json:"entryPrice"`
	BreakEvenPrice         string `json:"breakEvenPrice"`
	MarkPrice              string `json:"markPrice"`
	UnRealizedProfit       string `json:"unRealizedProfit"`
	LiquidationPrice       string `json:"liquidationPrice"`
	IsolatedMargin         string `json:"isolatedMargin"`
	Notional               string `json:"notional"`
	MarginAsset            string `json:"marginAsset"`
	IsolatedWallet         string `json:"isolatedWallet"`
	InitialMargin          string `json:"initialMargin"`
	MaintMargin            string `json:"maintMargin"`
	PositionInitialMargin  string `json:"positionInitialMargin"`
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"`
	Adl                    int    `json:"adl"`
	BidNotional            string `json:"bidNotional"`
	AskNotional            string `json:"askNotional"`
	UpdateTime             int64  `json:"updateTime"`
	Leverage               string `json:"leverage,omitempty"`
	MarginType             string `json:"marginType,omitempty"`
}

type OpenOrder struct {
	AvgPrice      string `json:"avgPrice"`
	ClientOrderID string `json:"clientOrderId"`
	CumQuote      string `json:"cumQuote"`
	ExecutedQty   string `json:"executedQty"`
	OrderID       int64  `json:"orderId"`
	OrigQty       string `json:"origQty"`
	OrigType      string `json:"origType"`
	Price         string `json:"price"`
	ReduceOnly    bool   `json:"reduceOnly"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
	Status        string `json:"status"`
	StopPrice     string `json:"stopPrice"`
	ClosePosition bool   `json:"closePosition"`
	Symbol        string `json:"symbol"`
	Time          int64  `json:"time"`
	TimeInForce   string `json:"timeInForce"`
	Type          string `json:"type"`
	ActivatePrice string `json:"activatePrice"`
	PriceRate     string `json:"priceRate"`
	UpdateTime    int64  `json:"updateTime"`
	WorkingType   string `json:"workingType"`
	PriceProtect  bool   `json:"priceProtect"`
	PriceMatch    string `json:"priceMatch"`
}

type ExchangeInfo struct {
	Symbols []SymbolInfo `json:"symbols"`
}

type SymbolInfo struct {
	Symbol            string         `json:"symbol"`
	Pair              string         `json:"pair"`
	ContractType      string         `json:"contractType"`
	Status            string         `json:"status"`
	BaseAsset         string         `json:"baseAsset"`
	QuoteAsset        string         `json:"quoteAsset"`
	MarginAsset       string         `json:"marginAsset"`
	PricePrecision    int            `json:"pricePrecision"`
	QuantityPrecision int            `json:"quantityPrecision"`
	Filters           []SymbolFilter `json:"filters"`
}

type SymbolFilter struct {
	FilterType string `json:"filterType"`
	MinPrice   string `json:"minPrice,omitempty"`
	MaxPrice   string `json:"maxPrice,omitempty"`
	TickSize   string `json:"tickSize,omitempty"`
	MinQty     string `json:"minQty,omitempty"`
	MaxQty     string `json:"maxQty,omitempty"`
	StepSize   string `json:"stepSize,omitempty"`
	Notional   string `json:"notional,omitempty"`
}

type PlaceOrderRequest struct {
	Symbol           string
	Side             string
	PositionSide     string
	Type             string
	TimeInForce      string
	Quantity         string
	Price            string
	NewClientOrderID string
	ReduceOnly       bool
}

type OrderAck struct {
	OrderID       int64  `json:"orderId"`
	Symbol        string `json:"symbol"`
	Status        string `json:"status"`
	ClientOrderID string `json:"clientOrderId"`
	Price         string `json:"price"`
	OrigQty       string `json:"origQty"`
	ExecutedQty   string `json:"executedQty"`
	Type          string `json:"type"`
	Side          string `json:"side"`
	PositionSide  string `json:"positionSide"`
}

type AlgoOrderRequest struct {
	Symbol           string
	Side             string
	PositionSide     string
	Type             string
	Quantity         string
	TriggerPrice     string
	WorkingType      string
	NewClientOrderID string
	ReduceOnly       bool
}

type AlgoOrderAck struct {
	AlgoID       int64  `json:"algoId"`
	ClientAlgoID string `json:"clientAlgoId"`
	AlgoType     string `json:"algoType"`
	OrderType    string `json:"orderType"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	PositionSide string `json:"positionSide"`
	Quantity     string `json:"quantity"`
	AlgoStatus   string `json:"algoStatus"`
	TriggerPrice string `json:"triggerPrice"`
}

func (c Client) Do(ctx context.Context, method, path string, values url.Values, private bool) ([]byte, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if values == nil {
		values = url.Values{}
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://fapi.binance.com"
	}
	if private {
		if err := c.Credentials.Validate(); err != nil {
			return nil, err
		}
		values.Set("timestamp", strconv.FormatInt(c.Now().UTC().UnixMilli(), 10))
		values.Set("signature", sign(values.Encode(), c.Credentials.SecretKey))
	}
	var body io.Reader
	requestURL := base + path
	if method == http.MethodGet || method == http.MethodDelete {
		if encoded := values.Encode(); encoded != "" {
			requestURL += "?" + encoded
		}
	} else {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if method != http.MethodGet && method != http.MethodDelete {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if private {
		req.Header.Set("X-MBX-APIKEY", c.Credentials.APIKey)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if apiErr := decodeAPIError(respBody); apiErr.Msg != "" || apiErr.Code != 0 {
			return nil, apiErr
		}
		return nil, fmt.Errorf("binance http status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if apiErr := decodeAPIError(respBody); apiErr.Code < 0 {
		return nil, apiErr
	}
	return respBody, nil
}

func decodeAPIError(b []byte) APIError {
	var errResp APIError
	_ = json.Unmarshal(b, &errResp)
	return errResp
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c Client) AccountBalance(ctx context.Context) ([]Balance, error) {
	b, err := c.Do(ctx, http.MethodGet, "/fapi/v3/balance", nil, true)
	if err != nil {
		return nil, err
	}
	var out []Balance
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) Positions(ctx context.Context, symbol string) ([]Position, error) {
	q := url.Values{}
	if strings.TrimSpace(symbol) != "" {
		q.Set("symbol", strings.ToUpper(strings.TrimSpace(symbol)))
	}
	b, err := c.Do(ctx, http.MethodGet, "/fapi/v3/positionRisk", q, true)
	if err != nil {
		return nil, err
	}
	var out []Position
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) OpenOrders(ctx context.Context, symbol string) ([]OpenOrder, error) {
	q := url.Values{}
	if strings.TrimSpace(symbol) != "" {
		q.Set("symbol", strings.ToUpper(strings.TrimSpace(symbol)))
	}
	b, err := c.Do(ctx, http.MethodGet, "/fapi/v1/openOrders", q, true)
	if err != nil {
		return nil, err
	}
	var out []OpenOrder
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) ExchangeInfo(ctx context.Context) (ExchangeInfo, error) {
	b, err := c.Do(ctx, http.MethodGet, "/fapi/v1/exchangeInfo", nil, false)
	if err != nil {
		return ExchangeInfo{}, err
	}
	var out ExchangeInfo
	if err := json.Unmarshal(b, &out); err != nil {
		return ExchangeInfo{}, err
	}
	return out, nil
}

func (c Client) SymbolInfo(ctx context.Context, symbol string) (SymbolInfo, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	info, err := c.ExchangeInfo(ctx)
	if err != nil {
		return SymbolInfo{}, err
	}
	for _, item := range info.Symbols {
		if strings.EqualFold(item.Symbol, symbol) {
			return item, nil
		}
	}
	return SymbolInfo{}, fmt.Errorf("binance symbol %s not found", symbol)
}

func (c Client) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	q := url.Values{}
	q.Set("symbol", strings.ToUpper(strings.TrimSpace(symbol)))
	q.Set("leverage", strconv.Itoa(leverage))
	_, err := c.Do(ctx, http.MethodPost, "/fapi/v1/leverage", q, true)
	return err
}

func (c Client) ChangeMarginType(ctx context.Context, symbol, marginType string) error {
	q := url.Values{}
	q.Set("symbol", strings.ToUpper(strings.TrimSpace(symbol)))
	q.Set("marginType", strings.ToUpper(strings.TrimSpace(marginType)))
	_, err := c.Do(ctx, http.MethodPost, "/fapi/v1/marginType", q, true)
	if err != nil && strings.Contains(err.Error(), "-4046") {
		return nil
	}
	return err
}

func (c Client) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (OrderAck, error) {
	q := url.Values{}
	q.Set("symbol", strings.ToUpper(strings.TrimSpace(req.Symbol)))
	q.Set("side", strings.ToUpper(strings.TrimSpace(req.Side)))
	q.Set("type", strings.ToUpper(strings.TrimSpace(req.Type)))
	q.Set("quantity", strings.TrimSpace(req.Quantity))
	if req.PositionSide != "" {
		q.Set("positionSide", strings.ToUpper(strings.TrimSpace(req.PositionSide)))
	}
	if req.TimeInForce != "" {
		q.Set("timeInForce", strings.ToUpper(strings.TrimSpace(req.TimeInForce)))
	}
	if req.Price != "" {
		q.Set("price", strings.TrimSpace(req.Price))
	}
	if req.NewClientOrderID != "" {
		q.Set("newClientOrderId", strings.TrimSpace(req.NewClientOrderID))
	}
	if req.ReduceOnly {
		q.Set("reduceOnly", "true")
	}
	b, err := c.Do(ctx, http.MethodPost, "/fapi/v1/order", q, true)
	if err != nil {
		return OrderAck{}, err
	}
	var ack OrderAck
	if err := json.Unmarshal(b, &ack); err != nil {
		return OrderAck{}, err
	}
	return ack, nil
}

func (c Client) NewAlgoOrder(ctx context.Context, req AlgoOrderRequest) (AlgoOrderAck, error) {
	q := url.Values{}
	q.Set("algoType", "CONDITIONAL")
	q.Set("symbol", strings.ToUpper(strings.TrimSpace(req.Symbol)))
	q.Set("side", strings.ToUpper(strings.TrimSpace(req.Side)))
	q.Set("type", strings.ToUpper(strings.TrimSpace(req.Type)))
	q.Set("quantity", strings.TrimSpace(req.Quantity))
	q.Set("triggerPrice", strings.TrimSpace(req.TriggerPrice))
	if req.PositionSide != "" {
		q.Set("positionSide", strings.ToUpper(strings.TrimSpace(req.PositionSide)))
	}
	if req.WorkingType != "" {
		q.Set("workingType", strings.ToUpper(strings.TrimSpace(req.WorkingType)))
	}
	if req.NewClientOrderID != "" {
		q.Set("clientAlgoId", strings.TrimSpace(req.NewClientOrderID))
	}
	if req.ReduceOnly {
		q.Set("reduceOnly", "true")
	}
	b, err := c.Do(ctx, http.MethodPost, "/fapi/v1/algoOrder", q, true)
	if err != nil {
		return AlgoOrderAck{}, err
	}
	var ack AlgoOrderAck
	if err := json.Unmarshal(b, &ack); err != nil {
		return AlgoOrderAck{}, err
	}
	return ack, nil
}

func USDTBalanceFromAccount(balances []Balance) (Balance, bool) {
	for _, balance := range balances {
		if strings.EqualFold(strings.TrimSpace(balance.Asset), "USDT") {
			return balance, true
		}
	}
	return Balance{}, false
}

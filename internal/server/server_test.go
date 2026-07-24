package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/security"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	"github.com/pcdogyu/tv_okx_bot/internal/upgrade"
)

type fakeExecutor struct {
	calls chan trading.Signal
}

func (f fakeExecutor) ExecuteSignal(ctx context.Context, signal trading.Signal, cfg trading.RuntimeConfig) (trading.OrderResult, error) {
	f.calls <- signal
	return trading.OrderResult{InstID: "BTC-USDT-SWAP", ClOrdID: "test", OrdID: "1", OKXCode: "0"}, nil
}

func (f fakeExecutor) Check(ctx context.Context, cfg trading.RuntimeConfig) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

type fakeUpgradeRunner struct {
	done chan struct{}
}

func (f fakeUpgradeRunner) Run(ctx context.Context) (upgrade.Result, error) {
	f.done <- struct{}{}
	return upgrade.Result{
		Steps: []upgrade.Step{{Name: "git_pull", Command: "git pull --ff-only", Output: "ok"}},
	}, nil
}

func TestRoutes(t *testing.T) {
	srv := newTestServer(t)
	root := httptest.NewRecorder()
	srv.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusFound || root.Header().Get("Location") != "https://www.mext.go.jp/" {
		t.Fatalf("root response code=%d location=%q", root.Code, root.Header().Get("Location"))
	}
	bad := httptest.NewRecorder()
	srv.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if bad.Code != http.StatusNotFound {
		t.Fatalf("bad path code=%d", bad.Code)
	}
	method := httptest.NewRecorder()
	srv.ServeHTTP(method, httptest.NewRequest(http.MethodGet, "/tvorder", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("tvorder GET code=%d", method.Code)
	}
	admin := httptest.NewRecorder()
	srv.ServeHTTP(admin, httptest.NewRequest(http.MethodGet, "/tvbot/config", nil))
	if admin.Code != http.StatusUnauthorized {
		t.Fatalf("tvbot without token code=%d", admin.Code)
	}
	if admin.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected Basic auth challenge")
	}
	basicReq := httptest.NewRequest(http.MethodGet, "/tvbot/config", nil)
	basicReq.SetBasicAuth("admin", "Admin123")
	basic := httptest.NewRecorder()
	srv.ServeHTTP(basic, basicReq)
	if basic.Code != http.StatusOK {
		t.Fatalf("tvbot with basic auth code=%d body=%s", basic.Code, basic.Body.String())
	}
	uiReq := httptest.NewRequest(http.MethodGet, "/tvbot/", nil)
	uiReq.SetBasicAuth("admin", "Admin123")
	ui := httptest.NewRecorder()
	srv.ServeHTTP(ui, uiReq)
	if ui.Code != http.StatusOK {
		t.Fatalf("tvbot ui code=%d body=%s", ui.Code, ui.Body.String())
	}
	if got := ui.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("tvbot ui content-type=%q", got)
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("OKX Bot")) || !bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/config")) {
		t.Fatalf("tvbot ui body does not look like dashboard")
	}
	if bytes.Contains(ui.Body.Bytes(), []byte("max-width: 1240px")) || !bytes.Contains(ui.Body.Bytes(), []byte("Asia/Shanghai")) {
		t.Fatalf("tvbot ui should use full-width layout and Shanghai order times")
	}
}

func TestTVOrderAcceptsAndDeduplicates(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	srv.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Coinpair != "BTC" || got.Action != trading.ActionLong {
			t.Fatalf("bad executed signal: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
	second := httptest.NewRecorder()
	srv.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if second.Code != http.StatusAccepted {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	if !bytes.Contains(second.Body.Bytes(), []byte(`"status":"duplicate"`)) {
		t.Fatalf("expected duplicate response, got %s", second.Body.String())
	}
	select {
	case <-srv.Executor.(fakeExecutor).calls:
		t.Fatal("duplicate webhook executed again")
	case <-time.After(50 * time.Millisecond):
	}
	ordersReq := httptest.NewRequest(http.MethodGet, "/tvbot/orders", nil)
	ordersReq.SetBasicAuth("admin", "Admin123")
	ordersRR := httptest.NewRecorder()
	srv.ServeHTTP(ordersRR, ordersReq)
	if ordersRR.Code != http.StatusOK {
		t.Fatalf("orders status=%d body=%s", ordersRR.Code, ordersRR.Body.String())
	}
	var list struct {
		Orders []storage.OrderRecord `json:"orders"`
	}
	if err := json.Unmarshal(ordersRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Orders) != 2 || list.Orders[0].Status != storage.StatusDuplicate {
		t.Fatalf("duplicate signal should be listed in history: %#v", list.Orders)
	}
}

func TestTVOrderAllowsUnconfiguredCoinpair(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.ConfigStore.Get()
	cfg.Symbols = map[string]config.SymbolConfig{}
	srv.ConfigStore = config.NewStore("", cfg)
	signal := validSignal(t, srv)
	signal.Action = trading.ActionShort
	signal.Coinpair = "DOGEUSDT.P"
	signal.Ticker = "OKX:DOGEUSDT.P"
	signal.Token = srv.Token.Generate(signal.CanonicalTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Coinpair != "DOGEUSDT.P" || got.Action != trading.ActionShort {
			t.Fatalf("bad executed signal: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
}

func TestTVOrderPassesSelectedAPIID(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.APIID = "backup"
	signal.Token = srv.Token.Generate(signal.CanonicalTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.APIID != "backup" {
			t.Fatalf("api id = %q", got.APIID)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
}

func TestTVOrderRecordsRejectedSignals(t *testing.T) {
	srv := newTestServer(t)
	body := []byte(`{
		"token": "present-but-not-checked-before-signal-validation",
		"sent_at": "2026-07-24T03:00:00Z",
		"action": "{{strategy.order.action}}",
		"ticker": "ETHUSDT.P",
		"coinpair": "ETHUSDT.P",
		"price": "1893.55",
		"exchange": "OKX",
		"interval": "15",
		"condition": "{{strategy.order.comment}}",
		"text": "{{strategy.order.alert_message}}",
		"source": "tradingview"
	}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"error":"invalid_signal"`)) {
		t.Fatalf("expected invalid_signal response, got %s", rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		t.Fatalf("rejected signal should not execute: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
	ordersReq := httptest.NewRequest(http.MethodGet, "/tvbot/orders", nil)
	ordersReq.SetBasicAuth("admin", "Admin123")
	ordersRR := httptest.NewRecorder()
	srv.ServeHTTP(ordersRR, ordersReq)
	if ordersRR.Code != http.StatusOK {
		t.Fatalf("orders status=%d body=%s", ordersRR.Code, ordersRR.Body.String())
	}
	var list struct {
		Orders []storage.OrderRecord `json:"orders"`
	}
	if err := json.Unmarshal(ordersRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Orders) != 1 {
		t.Fatalf("orders len=%d records=%#v", len(list.Orders), list.Orders)
	}
	got := list.Orders[0]
	if got.Status != storage.StatusRejected || got.ErrorCode != "invalid_signal" || !strings.Contains(got.Error, "action must be") {
		t.Fatalf("bad rejected record: %#v", got)
	}
	if got.Action != trading.Side("{{strategy.order.action}}") || got.Coinpair != "ETHUSDT.P" || got.Price != "1893.55" || got.Amount != "100" {
		t.Fatalf("rejected record lost signal fields: %#v", got)
	}
}

func TestTVOrderRecordsBadJSONSignals(t *testing.T) {
	srv := newTestServer(t)
	body := []byte(`{"action":"buy","ticker":"BTCUSDT","price":"{{close}}","token":"x"}`)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	ordersReq := httptest.NewRequest(http.MethodGet, "/tvbot/orders", nil)
	ordersReq.SetBasicAuth("admin", "Admin123")
	ordersRR := httptest.NewRecorder()
	srv.ServeHTTP(ordersRR, ordersReq)
	if ordersRR.Code != http.StatusOK {
		t.Fatalf("orders status=%d body=%s", ordersRR.Code, ordersRR.Body.String())
	}
	var list struct {
		Orders []storage.OrderRecord `json:"orders"`
	}
	if err := json.Unmarshal(ordersRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Orders) != 1 || list.Orders[0].Status != storage.StatusRejected || list.Orders[0].ErrorCode != "bad_json" {
		t.Fatalf("bad_json signal should be listed in history: %#v", list.Orders)
	}
	if list.Orders[0].Action != trading.ActionLong || list.Orders[0].Ticker != "BTCUSDT" {
		t.Fatalf("bad_json preview lost readable fields: %#v", list.Orders[0])
	}
}

func TestTVOrderAppliesConfiguredOrderSettings(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.ConfigStore.Get()
	cfg.Trading.OrderAmountUSDT = 250
	cfg.Trading.Leverage = 8
	cfg.Trading.RiskType = string(trading.RiskTrailing)
	cfg.Trading.TrailingPct = 1.5
	srv.ConfigStore = config.NewStore("", cfg)
	signal := trading.Signal{
		Action:   trading.ActionLong,
		APIID:    "main",
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		SentAt:   "2026-07-24T03:00:00Z",
		Ticker:   "BTCUSDT",
	}
	signal.Normalize()
	signal.Token = srv.Token.Generate(signal.CanonicalWebhookTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Amount.Value != 250 || got.Leverage != 8 || got.Risk.Type != trading.RiskTrailing {
			t.Fatalf("configured settings not applied: %#v", got)
		}
		if got.Risk.TrailingPct == nil || got.Risk.TrailingPct.Value != 1.5 {
			t.Fatalf("trailing setting not applied: %#v", got.Risk)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
}

func TestTVBotTemplatesRequiresAdminAndReturnsJSON(t *testing.T) {
	srv := newTestServer(t)
	reqBody := []byte(`{"price_source":"high","api_id":"backup","leverage":3,"amount":50}`)
	req := httptest.NewRequest(http.MethodPost, "/tvbot/templates", bytes.NewReader(reqBody))
	req.Header.Set("X-Admin-Token", "admin")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp trading.TemplateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Token) != 88 || !bytes.Contains([]byte(resp.JSON), []byte(`"price": "{{high}}"`)) || !bytes.Contains([]byte(resp.JSON), []byte(`"api_id": "backup"`)) {
		t.Fatalf("bad template response: %#v", resp)
	}
	if bytes.Contains([]byte(resp.JSON), []byte(`"risk"`)) {
		t.Fatalf("template should not include risk: %s", resp.JSON)
	}
	if bytes.Contains([]byte(resp.JSON), []byte(`"amount"`)) || bytes.Contains([]byte(resp.JSON), []byte(`"leverage"`)) {
		t.Fatalf("template should not include server-side order settings: %s", resp.JSON)
	}
}

func TestTVBotAPIKeysSaveAndMask(t *testing.T) {
	srv := newTestServer(t)
	reqBody := []byte(`{"api_key":"abcd12345678wxyz","secret_key":"super-secret-value","passphrase":"phrase-value"}`)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/api-keys", bytes.NewReader(reqBody))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("super-secret-value")) || bytes.Contains(rr.Body.Bytes(), []byte("phrase-value")) {
		t.Fatalf("response leaked secret material: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("abcd...wxyz")) {
		t.Fatalf("response did not include masked api key: %s", rr.Body.String())
	}
	getReq := httptest.NewRequest(http.MethodGet, "/tvbot/api-keys", nil)
	getReq.SetBasicAuth("admin", "Admin123")
	get := httptest.NewRecorder()
	srv.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK || bytes.Contains(get.Body.Bytes(), []byte("super-secret-value")) || bytes.Contains(get.Body.Bytes(), []byte("phrase-value")) {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
}

func TestTVBotAPIKeysTestUsesSelectedStoredAccount(t *testing.T) {
	var seenAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/balance":
			seenAPIKey = r.Header.Get("OK-ACCESS-KEY")
			if r.Header.Get("x-simulated-trading") != "1" {
				t.Fatal("missing demo header")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	srv := newTestServer(t)
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = ts.URL
	srv.ConfigStore = config.NewStore("", cfg)
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID: "main",
		Credentials: okx.Credentials{
			APIKey:     "main-key",
			SecretKey:  "main-secret",
			Passphrase: "main-pass",
		},
		Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID: "backup",
		Credentials: okx.Credentials{
			APIKey:     "backup-key",
			SecretKey:  "backup-secret",
			Passphrase: "backup-pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/api-keys/test", bytes.NewReader([]byte(`{"id":"backup"}`)))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if seenAPIKey != "backup-key" {
		t.Fatalf("used api key %q", seenAPIKey)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"api_id":"backup"`)) {
		t.Fatalf("response missing api id: %s", rr.Body.String())
	}
}

func TestUpgradeEndpointRequiresAdminAndStartsRunner(t *testing.T) {
	srv := newTestServer(t)
	runner := fakeUpgradeRunner{done: make(chan struct{}, 1)}
	srv.Upgrade = upgrade.NewManager(runner)

	unauth := httptest.NewRecorder()
	srv.ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/upgrade", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("upgrade without auth code=%d", unauth.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/upgrade", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("upgrade start code=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-runner.done:
	case <-time.After(time.Second):
		t.Fatal("upgrade runner was not called")
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/upgrade", nil)
	statusReq.SetBasicAuth("admin", "Admin123")
	status := httptest.NewRecorder()
	srv.ServeHTTP(status, statusReq)
	if status.Code != http.StatusOK {
		t.Fatalf("upgrade status code=%d body=%s", status.Code, status.Body.String())
	}
	if !bytes.Contains(status.Body.Bytes(), []byte(`"status":"succeeded"`)) {
		t.Fatalf("expected succeeded status, got %s", status.Body.String())
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.DataFile = filepath.Join(t.TempDir(), "orders.json")
	orderStore, err := storage.NewOrderStore(cfg.DataFile)
	if err != nil {
		t.Fatal(err)
	}
	credentialStore, err := okx.NewCredentialStore(filepath.Join(t.TempDir(), "okx-credentials.json"), okx.Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		ConfigStore:    config.NewStore("", cfg),
		Orders:         orderStore,
		Token:          security.NewTokenService("unit-test-secret"),
		Executor:       fakeExecutor{calls: make(chan trading.Signal, 2)},
		OKXCredentials: credentialStore,
		AdminToken:     "admin",
		AdminUser:      "admin",
		AdminPass:      "Admin123",
		Upgrade:        upgrade.NewManager(fakeUpgradeRunner{done: make(chan struct{}, 1)}),
		Now: func() time.Time {
			return time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
		},
	}
}

func validSignal(t *testing.T, srv *Server) trading.Signal {
	t.Helper()
	tp := trading.NewFlexibleFloat(2)
	sl := trading.NewFlexibleFloat(1)
	signal := trading.Signal{
		Action:   trading.ActionLong,
		Coinpair: "BTC",
		Price:    trading.NewFlexibleFloat(50000),
		SentAt:   "2026-07-24T03:00:00Z",
		Ticker:   "BTCUSDT",
		Leverage: 5,
		Amount:   trading.NewFlexibleFloat(100),
		Risk:     trading.Risk{Type: trading.RiskTPSL, TPPct: &tp, SLPct: &sl},
	}
	signal.Normalize()
	signal.Token = srv.Token.Generate(signal.CanonicalTokenPayload())
	return signal
}

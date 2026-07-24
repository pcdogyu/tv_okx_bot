package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
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
}

func TestTVBotTemplatesRequiresAdminAndReturnsJSON(t *testing.T) {
	srv := newTestServer(t)
	reqBody := []byte(`{"action":"short","coinpair":"ETH","price_source":"high","leverage":3,"amount":50,"risk":{"type":"trailing","trailing_pct":1.5}}`)
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
	if len(resp.Token) != 64 || !bytes.Contains([]byte(resp.JSON), []byte(`"price": "{{high}}"`)) {
		t.Fatalf("bad template response: %#v", resp)
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
	return &Server{
		ConfigStore: config.NewStore("", cfg),
		Orders:      orderStore,
		Token:       security.NewTokenService("unit-test-secret"),
		Executor:    fakeExecutor{calls: make(chan trading.Signal, 2)},
		AdminToken:  "admin",
		AdminUser:   "admin",
		AdminPass:   "Admin123",
		Upgrade:     upgrade.NewManager(fakeUpgradeRunner{done: make(chan struct{}, 1)}),
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

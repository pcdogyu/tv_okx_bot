package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
	if admin.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("tvbot should not trigger browser Basic auth challenge")
	}
	docReq := httptest.NewRequest(http.MethodGet, "/tvbot/", nil)
	docReq.Header.Set("Accept", "text/html")
	doc := httptest.NewRecorder()
	srv.ServeHTTP(doc, docReq)
	if doc.Code != http.StatusFound || !strings.HasPrefix(doc.Header().Get("Location"), "/tvbot/login") {
		t.Fatalf("tvbot document without session code=%d location=%q", doc.Code, doc.Header().Get("Location"))
	}
	basicReq := httptest.NewRequest(http.MethodGet, "/tvbot/config", nil)
	basicReq.SetBasicAuth("admin", "Admin123")
	basic := httptest.NewRecorder()
	srv.ServeHTTP(basic, basicReq)
	if basic.Code != http.StatusOK {
		t.Fatalf("tvbot with basic auth code=%d body=%s", basic.Code, basic.Body.String())
	}
	loginPage := httptest.NewRecorder()
	srv.ServeHTTP(loginPage, httptest.NewRequest(http.MethodGet, "/tvbot/login?next=/tvbot/config", nil))
	if loginPage.Code != http.StatusOK || !bytes.Contains(loginPage.Body.Bytes(), []byte("管理员登录")) {
		t.Fatalf("login page code=%d body=%s", loginPage.Code, loginPage.Body.String())
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/tvbot/login", strings.NewReader("username=admin&password=Admin123&next=%2Ftvbot%2Fconfig"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login := httptest.NewRecorder()
	srv.ServeHTTP(login, loginReq)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/tvbot/config" {
		t.Fatalf("login code=%d location=%q body=%s", login.Code, login.Header().Get("Location"), login.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.HttpOnly {
		t.Fatalf("login did not set admin session cookie: %#v", login.Result().Cookies())
	}
	cookieReq := httptest.NewRequest(http.MethodGet, "/tvbot/config", nil)
	cookieReq.AddCookie(sessionCookie)
	cookieRR := httptest.NewRecorder()
	srv.ServeHTTP(cookieRR, cookieReq)
	if cookieRR.Code != http.StatusOK {
		t.Fatalf("tvbot with session cookie code=%d body=%s", cookieRR.Code, cookieRR.Body.String())
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
	if got := ui.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("tvbot ui cache-control=%q", got)
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("OKX Bot")) || !bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/config")) {
		t.Fatalf("tvbot ui body does not look like dashboard")
	}
	if bytes.Contains(ui.Body.Bytes(), []byte("max-width: 1240px")) || !bytes.Contains(ui.Body.Bytes(), []byte("Asia/Shanghai")) {
		t.Fatalf("tvbot ui should use full-width layout and Shanghai order times")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("订单分析")) {
		t.Fatalf("tvbot ui should include order analysis tab")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("持仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/positions")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("position-api-id")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("position-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signed-profit")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signed-loss")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signedCell")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("市价平仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("限价平仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/positions/close")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-position-close")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("<th>操作</th>")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("<th>更新时间</th>")) {
		t.Fatalf("tvbot ui should include current positions tab")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("币对配置")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("订单配置")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/symbols")) {
		t.Fatalf("tvbot ui should include symbol and order config tabs")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("菜单设置")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("menu-settings-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("saveMenuSettings")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("applyMenuSettings")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-menu-hidden")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-menu-move")) {
		t.Fatalf("tvbot ui should include menu settings tab")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("订单类型")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`id="order-type"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("市价单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("限价单")) {
		t.Fatalf("tvbot ui should include market/limit order type setting")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("资产估值")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("USDT估值")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("USDT估值 最近 3 天")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-total-eq")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-usdt-eq")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-balance-rows")) {
		t.Fatalf("tvbot ui should include OKX balance analysis")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("chart-grid")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("chartTimeLabel")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("chartTickIndexes")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("usdtValuationPoints")) {
		t.Fatalf("tvbot ui should render full-width chart grid and time axis")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("账户名称")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("OKX / 返回")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("order-okx")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("apiDisplayName")) {
		t.Fatalf("tvbot ui should render order API names and wider OKX return column")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("activateTab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("localStorage")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("location.hash")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("tvbot.active_tab")) {
		t.Fatalf("tvbot ui should remember active tab across refresh")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("USDT 可用")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("usdt_balance")) {
		t.Fatalf("tvbot ui should render USDT balance after API tests")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("Code by Yuhao@jiansutech.com - 2026-07-24T03:00:00Z - testhash - testbranch")) {
		t.Fatalf("tvbot ui should include build footer")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("data-retry-id")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/retry")) {
		t.Fatalf("tvbot ui should include retry controls")
	}
}

func TestTVBotConfigSavesOrderType(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/config", bytes.NewReader([]byte(`{"trading":{"order_type":"limit"}}`)))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rr.Code, rr.Body.String())
	}
	var cfg config.Config
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Trading.OrderType != string(trading.OrderTypeLimit) || cfg.OrderSettings().OrderType != trading.OrderTypeLimit {
		t.Fatalf("order type not saved: %#v", cfg.Trading)
	}

	badReq := httptest.NewRequest(http.MethodPut, "/tvbot/config", bytes.NewReader([]byte(`{"trading":{"order_type":"post_only"}}`)))
	badReq.SetBasicAuth("admin", "Admin123")
	bad := httptest.NewRecorder()
	srv.ServeHTTP(bad, badReq)
	if bad.Code != http.StatusBadRequest || !bytes.Contains(bad.Body.Bytes(), []byte("unsupported order_type")) {
		t.Fatalf("bad order type status=%d body=%s", bad.Code, bad.Body.String())
	}
}

func TestTVBotConfigSavesMenuSettings(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/config", bytes.NewReader([]byte(`{
		"ui": {
			"menu_items": [
				{"tab":"orders","hidden":true},
				{"tab":"dashboard","hidden":false},
				{"tab":"menuSettings","hidden":true},
				{"tab":"unknown","hidden":false}
			]
		}
	}`)))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rr.Code, rr.Body.String())
	}
	var cfg config.Config
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.UI.MenuItems) != len(config.DefaultMenuTabs) {
		t.Fatalf("menu items should be normalized to defaults: %#v", cfg.UI.MenuItems)
	}
	if cfg.UI.MenuItems[0].Tab != "orders" || !cfg.UI.MenuItems[0].Hidden {
		t.Fatalf("first menu item should preserve hidden orders: %#v", cfg.UI.MenuItems[0])
	}
	if cfg.UI.MenuItems[1].Tab != "dashboard" || cfg.UI.MenuItems[1].Hidden {
		t.Fatalf("second menu item should preserve visible dashboard: %#v", cfg.UI.MenuItems[1])
	}
	if cfg.UI.MenuItems[2].Tab != config.MenuSettingsTab || cfg.UI.MenuItems[2].Hidden {
		t.Fatalf("menu settings should be forced visible: %#v", cfg.UI.MenuItems[2])
	}
	for _, item := range cfg.UI.MenuItems {
		if item.Tab == "unknown" {
			t.Fatalf("unknown menu item should be removed: %#v", cfg.UI.MenuItems)
		}
	}
}

func TestTVBotSymbolsReturnsConfiguredAndOKXCatalog(t *testing.T) {
	var sawLive, sawDemo bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/public/instruments" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("instType") != "SWAP" {
			t.Fatalf("bad instruments query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("OK-ACCESS-KEY") != "" {
			t.Fatal("public instruments request should not be signed")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-simulated-trading") == "1" {
			sawDemo = true
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","baseCcy":"BTC","quoteCcy":"USDT","settleCcy":"USDT","ctVal":"0.01","ctValCcy":"BTC","lotSz":"0.01","minSz":"0.01","lever":"100","state":"live"},
				{"instType":"SWAP","instId":"DOGE-USDT-SWAP","baseCcy":"DOGE","quoteCcy":"USDT","settleCcy":"USDT","ctVal":"1000","ctValCcy":"DOGE","lotSz":"1","minSz":"1","lever":"50","state":"live"}
			]}`))
			return
		}
		sawLive = true
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
			{"instType":"SWAP","instId":"ETH-USDT-SWAP","baseCcy":"ETH","quoteCcy":"USDT","settleCcy":"USDT","ctVal":"0.1","ctValCcy":"ETH","lotSz":"0.01","minSz":"0.01","lever":"100","state":"live"},
			{"instType":"SWAP","instId":"BTC-USDT-SWAP","baseCcy":"BTC","quoteCcy":"USDT","settleCcy":"USDT","ctVal":"0.01","ctValCcy":"BTC","lotSz":"0.01","minSz":"0.01","lever":"100","state":"live"}
		]}`))
	}))
	defer ts.Close()

	srv := newTestServer(t)
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = ts.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = ts.Client()

	req := httptest.NewRequest(http.MethodGet, "/tvbot/symbols", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("symbols status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawLive || !sawDemo {
		t.Fatalf("expected live and demo instruments requests live=%v demo=%v", sawLive, sawDemo)
	}
	var resp symbolsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Symbols["BTC"].InstID != "BTC-USDT-SWAP" {
		t.Fatalf("response lost configured symbols: %#v", resp.Symbols)
	}
	if resp.OKX.Live.Count != 2 || resp.OKX.Demo.Count != 2 {
		t.Fatalf("bad OKX counts: %#v", resp.OKX)
	}
	if resp.OKX.Live.Instruments[0].InstID != "BTC-USDT-SWAP" || resp.OKX.Demo.Instruments[1].InstID != "DOGE-USDT-SWAP" {
		t.Fatalf("instruments should be sorted and parsed: %#v", resp.OKX)
	}
	if resp.OKX.Live.Error != "" || resp.OKX.Demo.Error != "" {
		t.Fatalf("unexpected OKX errors: %#v", resp.OKX)
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

func TestOrderRetryCreatesNewOrderAndExecutes(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.Action = trading.ActionShort
	signal.APIID = "backup"
	signal.Token = srv.Token.Generate(signal.CanonicalTokenPayload())
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	srv.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResp struct {
		SignalID string `json:"signal_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatal(err)
	}
	waitOrderStatus(t, srv.Orders, firstResp.SignalID, storage.StatusSubmitted)
	select {
	case <-srv.Executor.(fakeExecutor).calls:
	case <-time.After(time.Second):
		t.Fatal("initial order was not executed")
	}
	sourceID := firstResp.SignalID
	if err := srv.Orders.MarkFailed(sourceID, fmt.Errorf("okx failed"), srv.now()); err != nil {
		t.Fatal(err)
	}
	cfg := srv.ConfigStore.Get()
	cfg.Trading.Leverage = 8
	srv.ConfigStore = config.NewStore("", cfg)

	req := httptest.NewRequest(http.MethodPost, "/tvbot/orders/"+sourceID+"/retry", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", rr.Code, rr.Body.String())
	}
	var retryResp struct {
		SignalID string `json:"signal_id"`
		RetryOf  string `json:"retry_of"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &retryResp); err != nil {
		t.Fatal(err)
	}
	if retryResp.RetryOf != sourceID || retryResp.SignalID == "" || retryResp.SignalID == sourceID {
		t.Fatalf("bad retry response: %#v", retryResp)
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		if got.Action != trading.ActionShort || got.APIID != "backup" || got.Coinpair != "BTC" || got.Price.Value != 50000 {
			t.Fatalf("bad retry signal: %#v", got)
		}
		if got.Amount.Value != 100 || got.Risk.Type == "" {
			t.Fatalf("retry signal lost order settings: %#v", got)
		}
		if got.Leverage != 8 {
			t.Fatalf("retry should use current configured leverage, got signal: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("retry order was not executed")
	}
	gotRetry := waitOrderStatus(t, srv.Orders, retryResp.SignalID, storage.StatusSubmitted)
	orders := srv.Orders.List(10)
	if len(orders) != 2 {
		t.Fatalf("orders len=%d records=%#v", len(orders), orders)
	}
	if gotRetry.SignalID == sourceID {
		t.Fatalf("retry should create a new record: %#v", gotRetry)
	}
}

func TestOrderRetryRejectsNonFailedRecord(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	body, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	srv.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/tvorder", bytes.NewReader(body)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResp struct {
		SignalID string `json:"signal_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatal(err)
	}
	waitOrderStatus(t, srv.Orders, firstResp.SignalID, storage.StatusSubmitted)
	select {
	case <-srv.Executor.(fakeExecutor).calls:
	case <-time.After(time.Second):
		t.Fatal("initial order was not executed")
	}
	req := httptest.NewRequest(http.MethodPost, "/tvbot/orders/"+firstResp.SignalID+"/retry", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("retry non-failed status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case got := <-srv.Executor.(fakeExecutor).calls:
		t.Fatalf("non-failed order should not retry: %#v", got)
	case <-time.After(50 * time.Millisecond):
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
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("template cache-control=%q", got)
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
	if strings.LastIndex(resp.JSON, `"token"`) < strings.LastIndex(resp.JSON, `"source"`) {
		t.Fatalf("token should be the final JSON field: %s", resp.JSON)
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

func TestTVBotAnalysisRequiresAdminAndReturnsOKXStats(t *testing.T) {
	srv := newTestServer(t)
	fillTime1 := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC).UnixMilli()
	fillTime2 := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC).UnixMilli()
	candleTime1 := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC).UnixMilli()
	candleTime2 := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC).UnixMilli()
	var sawBalance, sawCandles, sawFills bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/balance":
			sawBalance = true
			if r.URL.RawQuery != "" {
				t.Fatalf("balance should request all assets, got query: %s", r.URL.RawQuery)
			}
			if r.Header.Get("x-simulated-trading") != "1" || r.Header.Get("OK-ACCESS-KEY") != "key" {
				t.Fatalf("missing private OKX headers")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"totalEq":"80078.07","adjEq":"80000","availEq":"79900","uTime":"1784880000000","details":[
				{"ccy":"BTC","eq":"1","eqUsd":"64973.4","availBal":"1","cashBal":"1","frozenBal":"0","uTime":"1784880000000"},
				{"ccy":"USDT","eq":"5000","eqUsd":"4996.65","availBal":"5000","cashBal":"5000","frozenBal":"0","uTime":"1784880000000"}
			]}]}`))
		case "/api/v5/market/candles":
			sawCandles = true
			if r.URL.Query().Get("instId") != "USDT-USD" || r.URL.Query().Get("bar") != "1H" || r.URL.Query().Get("limit") != "72" {
				t.Fatalf("bad candle query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":"0","msg":"","data":[["%d","0.9990","0.9992","0.9989","0.9991","10","10","10","1"],["%d","0.9991","0.9993","0.9990","0.9992","12","12","12","1"]]}`, candleTime2, candleTime1)))
		case "/api/v5/trade/fills-history":
			sawFills = true
			if r.Header.Get("x-simulated-trading") != "1" || r.Header.Get("OK-ACCESS-KEY") != "key" {
				t.Fatalf("missing private OKX headers")
			}
			if r.URL.Query().Get("instType") != "SWAP" || r.URL.Query().Get("limit") != "100" {
				t.Fatalf("bad fills query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","tradeId":"t1","ordId":"o1","side":"sell","fillPx":"50000","fillSz":"1","fillPnl":"2.5","fee":"-0.1","feeCcy":"USDT","fillTime":"%d"},
				{"instType":"SWAP","instId":"ETH-USDT-SWAP","tradeId":"t2","ordId":"o2","side":"buy","fillPx":"2500","fillSz":"1","fillPnl":"-1","fee":"-0.05","feeCcy":"USDT","fillTime":"%d"}
			]}`, fillTime2, fillTime1)))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "default",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	unauth := httptest.NewRecorder()
	srv.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/tvbot/analysis", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("analysis without auth code=%d", unauth.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/tvbot/analysis?refresh=true", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analysis code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawBalance || !sawCandles || !sawFills {
		t.Fatalf("expected OKX balance, candle and fills calls balance=%v candles=%v fills=%v", sawBalance, sawCandles, sawFills)
	}
	var resp analysisResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Balance.TotalEq != "80078.07" || len(resp.Balance.Details) != 2 || resp.Balance.Details[0].Ccy != "BTC" {
		t.Fatalf("bad balance data: %#v", resp.Balance)
	}
	if len(resp.PricePoints) != 2 || resp.PriceInstID != "USDT-USD" || resp.PriceBar != "1H" {
		t.Fatalf("bad price data: %#v", resp)
	}
	if len(resp.BalancePoints) != 1 || math.Abs(resp.BalancePoints[0].Value-4996.65) > 0.0000001 {
		t.Fatalf("bad balance points: %#v", resp.BalancePoints)
	}
	snapshots, err := srv.Orders.ListUSDTBalanceSnapshots("default", cfg.Trading.Env, time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].EqUsd != "4996.65" || snapshots[0].BucketTS != time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("analysis did not write USDT balance snapshot: %#v", snapshots)
	}
	if resp.Summary.TradeCount != 2 || resp.Summary.Wins != 1 || resp.Summary.Losses != 1 {
		t.Fatalf("bad summary counts: %#v", resp.Summary)
	}
	if math.Abs(resp.Summary.NetPnL-1.35) > 0.0000001 || resp.Summary.WinRate != 0.5 {
		t.Fatalf("bad summary metrics: %#v", resp.Summary)
	}
	if len(resp.Symbols) != 2 {
		t.Fatalf("expected symbol stats: %#v", resp.Symbols)
	}
}

func TestUSDTBalanceSamplerStoresConfiguredAccounts(t *testing.T) {
	srv := newTestServer(t)
	seen := map[string]bool{}
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/account/balance" {
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Fatalf("missing demo header")
		}
		apiKey := r.Header.Get("OK-ACCESS-KEY")
		seen[apiKey] = true
		eqUSD := map[string]string{"main-key": "100.5", "backup-key": "200.5"}[apiKey]
		if eqUSD == "" {
			t.Fatalf("unexpected api key %q", apiKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"totalEq":"` + eqUSD + `","uTime":"1784880000000","details":[{"ccy":"USDT","eq":"` + eqUSD + `","eqUsd":"` + eqUSD + `","availEq":"` + eqUSD + `","uTime":"1784880000000"}]}]}`))
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "main",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "main-key",
			SecretKey:  "main-secret",
			Passphrase: "main-pass",
		},
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

	srv.sampleConfiguredUSDTBalances(context.Background())
	if !seen["main-key"] || !seen["backup-key"] {
		t.Fatalf("expected both accounts sampled, seen=%#v", seen)
	}
	for apiID, eqUSD := range map[string]string{"main": "100.5", "backup": "200.5"} {
		snapshots, err := srv.Orders.ListUSDTBalanceSnapshots(apiID, cfg.Trading.Env, time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshots) != 1 || snapshots[0].EqUsd != eqUSD {
			t.Fatalf("bad %s snapshots: %#v", apiID, snapshots)
		}
	}
}

func TestTVBotPositionsRequiresAdminAndReturnsCurrentPositions(t *testing.T) {
	srv := newTestServer(t)
	var sawPositions bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/account/positions" {
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
		sawPositions = true
		if r.URL.Query().Get("instType") != "SWAP" {
			t.Fatalf("bad positions query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-simulated-trading") != "1" || r.Header.Get("OK-ACCESS-KEY") != "key" {
			t.Fatalf("missing private OKX headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
			{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"isolated","posId":"1","posSide":"long","pos":"0.5","availPos":"0.5","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","liqPx":"51000","notionalUsd":"32500","margin":"6500","mgnRatio":"100","uTime":"1784880000000"},
			{"instType":"SWAP","instId":"ETH-USDT-SWAP","mgnMode":"isolated","posId":"2","posSide":"short","pos":"0","availPos":"0","avgPx":"2500","markPx":"2490","upl":"0","uplRatio":"0","lever":"5","notionalUsd":"0","uTime":"1784880000000"}
		]}`))
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "default",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	unauth := httptest.NewRecorder()
	srv.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/tvbot/positions", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("positions without auth code=%d", unauth.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/tvbot/positions?api_id=default", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("positions code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawPositions {
		t.Fatal("expected OKX positions call")
	}
	var resp positionsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.APIID != "default" || resp.Count != 1 || len(resp.Positions) != 1 {
		t.Fatalf("bad positions response: %#v", resp)
	}
	if resp.Positions[0].InstID != "BTC-USDT-SWAP" || resp.Positions[0].Upl != "500" {
		t.Fatalf("bad position data: %#v", resp.Positions[0])
	}
}

func TestTVBotPositionMarketClosePlacesReduceOnlyOrder(t *testing.T) {
	srv := newTestServer(t)
	var sawOrder bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/positions":
			if r.URL.Query().Get("instType") != "SWAP" {
				t.Fatalf("bad positions query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"cross","posId":"1","posSide":"net","pos":"2","availPos":"2","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","notionalUsd":"130000","margin":"26000","uTime":"1784880000000"}]}`))
		case "/api/v5/trade/order":
			sawOrder = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["instId"] != "BTC-USDT-SWAP" || body["tdMode"] != "cross" || body["side"] != "sell" || body["ordType"] != "market" || body["sz"] != "2" || body["reduceOnly"] != true {
				t.Fatalf("unexpected market close order: %#v", body)
			}
			if _, ok := body["posSide"]; ok {
				t.Fatalf("net close order should not send posSide: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"market-1","clOrdId":"close-1","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "default",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	reqBody := []byte(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","pos_side":"net","mode":"market"}`)
	req := httptest.NewRequest(http.MethodPost, "/tvbot/positions/close", bytes.NewReader(reqBody))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("market close status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawOrder {
		t.Fatal("expected OKX market order")
	}
	var resp positionCloseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Status != "submitted" || resp.Mode != "market" || resp.OrdID != "market-1" {
		t.Fatalf("bad market close response: %#v", resp)
	}
}

func TestTVBotPositionLimitCloseReplacesAndFallsBackToMarket(t *testing.T) {
	oldPoll := positionClosePollInterval
	oldTimeout := positionCloseLimitTimeout
	oldJobs := positionCloseJobs
	positionClosePollInterval = 10 * time.Millisecond
	positionCloseLimitTimeout = 45 * time.Millisecond
	positionCloseJobs = newPositionCloseRegistry()
	t.Cleanup(func() {
		positionClosePollInterval = oldPoll
		positionCloseLimitTimeout = oldTimeout
		positionCloseJobs = oldJobs
	})

	srv := newTestServer(t)
	var mu sync.Mutex
	var orderBodies []map[string]any
	cancelCount := 0
	tickerCalls := 0
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/positions":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"isolated","posId":"1","posSide":"long","pos":"0.5","availPos":"0.5","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","notionalUsd":"32500","margin":"6500","uTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
				t.Fatalf("bad instrument query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","tickSz":"0.1","lotSz":"0.1","minSz":"0.1","ctVal":"0.01","state":"live"}]}`))
		case "/api/v5/market/ticker":
			mu.Lock()
			tickerCalls++
			call := tickerCalls
			mu.Unlock()
			if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
				t.Fatalf("bad ticker query: %s", r.URL.RawQuery)
			}
			if call == 1 {
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","bidPx":"99.9","askPx":"100.1","last":"100","ts":"1784880000000"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","bidPx":"100.1","askPx":"100.3","last":"100.2","ts":"1784880005000"}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			orderBodies = append(orderBodies, body)
			orderIndex := len(orderBodies)
			mu.Unlock()
			if body["instId"] != "BTC-USDT-SWAP" || body["tdMode"] != "isolated" || body["side"] != "sell" || body["sz"] != "0.5" || body["posSide"] != "long" {
				t.Fatalf("unexpected close order: %#v", body)
			}
			ordID := fmt.Sprintf("order-%d", orderIndex)
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"` + ordID + `","clOrdId":"close","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["instId"] != "BTC-USDT-SWAP" || body["ordId"] == "" {
				t.Fatalf("unexpected cancel order: %#v", body)
			}
			mu.Lock()
			cancelCount++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"` + body["ordId"] + `","clOrdId":"close","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "default",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/positions/close", bytes.NewReader([]byte(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","pos_side":"long","mode":"limit"}`)))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("limit close status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp positionCloseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Status != "running" || resp.Px != "99.9" {
		t.Fatalf("bad limit close response: %#v", resp)
	}

	deadline := time.Now().Add(time.Second)
	var got []map[string]any
	var gotCancels int
	for time.Now().Before(deadline) {
		mu.Lock()
		got = append([]map[string]any(nil), orderBodies...)
		gotCancels = cancelCount
		mu.Unlock()
		hasMarket := false
		for _, body := range got {
			if body["ordType"] == "market" {
				hasMarket = true
			}
		}
		if hasMarket {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(got) < 3 || got[0]["ordType"] != "limit" || got[0]["px"] != "99.9" || got[1]["ordType"] != "limit" || got[1]["px"] != "100.1" || got[len(got)-1]["ordType"] != "market" {
		t.Fatalf("expected initial limit, replacement limit, and fallback market orders: %#v", got)
	}
	if gotCancels < 2 {
		t.Fatalf("expected cancels before replacement and fallback, got %d", gotCancels)
	}
}

func TestLowMarginPositionMonitorStartsLimitClose(t *testing.T) {
	oldPoll := positionClosePollInterval
	oldTimeout := positionCloseLimitTimeout
	oldJobs := positionCloseJobs
	positionClosePollInterval = time.Hour
	positionCloseLimitTimeout = time.Hour
	positionCloseJobs = newPositionCloseRegistry()
	t.Cleanup(func() {
		positionClosePollInterval = oldPoll
		positionCloseLimitTimeout = oldTimeout
		positionCloseJobs = oldJobs
	})

	srv := newTestServer(t)
	var mu sync.Mutex
	var orders []map[string]any
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/positions":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"isolated","posId":"1","posSide":"long","pos":"0.5","availPos":"0.5","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","notionalUsd":"32500","margin":"9.99","uTime":"1784880000000"},
				{"instType":"SWAP","instId":"ETH-USDT-SWAP","mgnMode":"isolated","posId":"2","posSide":"short","pos":"1","availPos":"1","avgPx":"2500","markPx":"2490","upl":"10","uplRatio":"0.01","lever":"5","notionalUsd":"2490","margin":"10","uTime":"1784880000000"},
				{"instType":"SWAP","instId":"DOGE-USDT-SWAP","mgnMode":"isolated","posId":"3","posSide":"long","pos":"100","availPos":"100","avgPx":"0.2","markPx":"0.21","upl":"1","uplRatio":"0.01","lever":"5","notionalUsd":"21","margin":"","uTime":"1784880000000"}
			]}`))
		case "/api/v5/public/instruments":
			if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
				t.Fatalf("low margin monitor should only query BTC instrument, got %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","tickSz":"0.1","lotSz":"0.1","minSz":"0.1","ctVal":"0.01","state":"live"}]}`))
		case "/api/v5/market/ticker":
			if r.URL.Query().Get("instId") != "BTC-USDT-SWAP" {
				t.Fatalf("bad ticker query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","bidPx":"99.9","askPx":"100.1","last":"100","ts":"1784880000000"}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			orders = append(orders, body)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"low-margin-1","clOrdId":"close","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:     "default",
		Active: true,
		Credentials: okx.Credentials{
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
	}); err != nil {
		t.Fatal(err)
	}

	srv.closeLowMarginPositions(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(orders) != 1 {
		t.Fatalf("expected exactly one low margin close order, got %#v", orders)
	}
	got := orders[0]
	if got["instId"] != "BTC-USDT-SWAP" || got["ordType"] != "limit" || got["side"] != "sell" || got["px"] != "99.9" || got["sz"] != "0.5" || got["posSide"] != "long" {
		t.Fatalf("unexpected low margin close order: %#v", got)
	}
}

func TestTVBotAPIKeysTestUsesSelectedStoredAccount(t *testing.T) {
	var seenAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/balance":
			seenAPIKey = r.Header.Get("OK-ACCESS-KEY")
			if r.URL.RawQuery != "" {
				t.Fatalf("balance should request all assets, got query: %s", r.URL.RawQuery)
			}
			if r.Header.Get("x-simulated-trading") != "1" {
				t.Fatal("missing demo header")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"totalEq":"1001.25","details":[{"ccy":"USDT","eq":"999.75","availEq":"888.5","availBal":"887.5","cashBal":"900","frozenBal":"11.25","uTime":"1784886000000"}]}]}`))
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
	var resp struct {
		Found   bool `json:"usdt_balance_found"`
		Balance struct {
			Ccy             string `json:"ccy"`
			Equity          string `json:"eq"`
			AvailableEquity string `json:"avail_eq"`
			FrozenBalance   string `json:"frozen_bal"`
			UpdateTime      string `json:"u_time"`
		} `json:"usdt_balance"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Found || resp.Balance.Ccy != "USDT" || resp.Balance.AvailableEquity != "888.5" || resp.Balance.Equity != "999.75" || resp.Balance.FrozenBalance != "11.25" || resp.Balance.UpdateTime != "1784886000000" {
		t.Fatalf("bad usdt balance: %#v", resp)
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
	dir := t.TempDir()
	cfg.DataFile = filepath.Join(dir, "orders.json")
	cfg.DatabaseFile = filepath.Join(dir, "tvbot.db")
	orderStore, err := storage.NewSQLiteOrderStore(cfg.DatabaseFile, cfg.DataFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orderStore.Close() })
	credentialStore, err := okx.NewCredentialStore(filepath.Join(dir, "okx-credentials.json"), okx.Credentials{})
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
		BuildInfo:      BuildInfo{CommitTime: "2026-07-24T03:00:00Z", CommitHash: "testhash", CommitBranch: "testbranch"},
		Now: func() time.Time {
			return time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
		},
	}
}

func waitOrderStatus(t *testing.T, store *storage.OrderStore, signalID string, want storage.OrderStatus) storage.OrderRecord {
	t.Helper()
	var last storage.OrderRecord
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, rec := range store.List(50) {
			if rec.SignalID != signalID {
				continue
			}
			last = rec
			if rec.Status == want {
				return rec
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("order %s did not reach status %s, last=%#v", signalID, want, last)
	return storage.OrderRecord{}
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

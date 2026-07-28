package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
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
	if !bytes.Contains(ui.Body.Bytes(), []byte(`id="order-info-status" hidden`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(".status[hidden]")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("syncOrderInfoVisibility")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`tabID !== "orderSettings"`)) {
		t.Fatalf("tvbot ui should only show order info status on order settings tab")
	}
	if bytes.Contains(ui.Body.Bytes(), []byte("max-width: 1240px")) || !bytes.Contains(ui.Body.Bytes(), []byte("Asia/Shanghai")) {
		t.Fatalf("tvbot ui should use full-width layout and Shanghai order times")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("订单分析")) {
		t.Fatalf("tvbot ui should include order analysis tab")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("持仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/positions")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/pending-orders")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("position-exchange-summary")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionExchanges")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("position-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positions-table")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(".positions-table td")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("font-size: 12px")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pos-actions-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pos-position-amount-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pos-entry-time-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pos-holding-time-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("仓位金额")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`title: "下单时间"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`title: "持仓时间"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionAmount(row)")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionReturnRatio")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("formatHoldingSeconds")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionEntryTimeCell")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("upl / Math.abs(margin)")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("平10%")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`data-position-ratio`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("position-percent-close-btn")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("width: 36px")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("border-radius: 999px")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("font-size: 7px")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(".positions-table .position-actions")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("gap: 8px")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(".positions-table .pos-actions-col { width: 25%; }")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionTableColumnDefs")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`tableColumnCount("positions")`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pending-order-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pending-margin-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pending-actions-col")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pendingOrderTableColumnDefs")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`tableColumnCount("pending_orders")`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("当前挂单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("renderPendingOrders")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pendingOrdersSummaryText")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("normal_count")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("algo_count")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("total_count")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("OKX 普通单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("算法订单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("Binance 普通单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("算法单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("renderTableStructure")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("saveTableColumnOrder")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("initTableColumnDrag")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-table-columns")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("table_columns")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pending_orders")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("委托价格")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("中间价")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("保证金")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("追单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("停止追单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("取消")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-pending-cancel")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/pending-orders/chase")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/pending-orders/chase/stop")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/pending-orders/cancel")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signed-profit")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signed-loss")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("signedCell")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionSideKind")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("positionSideCell")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("多单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("空单")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("市价平仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("限价平仓")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/positions/close")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-position-close")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`title: "操作"`)) ||
		bytes.Contains(ui.Body.Bytes(), []byte(`<th>可用</th>`)) ||
		bytes.Contains(ui.Body.Bytes(), []byte("pos-available-col")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("<th>保证金模式</th>")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("<th>强平价</th>")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("<th>客户端 ID</th>")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("<th>更新时间</th>")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("挂单数 ")) {
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
		!bytes.Contains(ui.Body.Bytes(), []byte("<th>首页</th>")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("menu-default-tab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-menu-home")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("default_tab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-menu-label")) ||
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
	if !bytes.Contains(ui.Body.Bytes(), []byte("USDT估值")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("USDT余额")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("/tvbot/balances/overview")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("OKX USDT 余额")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("Binance USDT 余额")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("USDT 余额表")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`data-balance-minutes="129600"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("重置基准")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("同步历史")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("overview-okx-usdt-chart")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("overview-binance-usdt-chart")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-usdt-eq")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-balance-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-binance-balance-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("#analysis .mini-usdt-chart {\n      height: 360px;")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("#analysis .mini-usdt-chart { height: 346px; }")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("balance-pnl-block")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("OKX 盈亏分析")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("Binance 盈亏分析")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-okx-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-binance-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("成交历史")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-trade-rows")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-trade-page-info")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysisTradePageSize = 20")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysisBalanceRefreshIntervalMs = 60000")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("refreshAnalysisBalanceOverviewAuto")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("visibilitychange")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("formatPriceAmount")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("formatQuantityAmount")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("symbolPrecisions")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysisPNLWindowMinutes")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("pnl_minutes")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-symbol-table")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("analysis-trade-table")) {
		t.Fatalf("tvbot ui should include exchange balance analysis")
	}
	for _, removed := range [][]byte{
		[]byte("analysis-avail-eq"),
		[]byte("analysis-adj-eq"),
		[]byte("analysis-asset-count"),
		[]byte("analysis-binance-avail"),
		[]byte("analysis-binance-api"),
	} {
		if bytes.Contains(ui.Body.Bytes(), removed) {
			t.Fatalf("tvbot analysis balance UI should not include removed metric %q", removed)
		}
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("chart-grid")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("chartTimeLabel")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("chartTickIndexes")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("usdtBalancePoints")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`["cash_bal", "avail_bal", "eq", "eq_usd"]`)) {
		t.Fatalf("tvbot ui should render full-width chart grid and time axis")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("账户名称")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`data-api-key-exchange="okx"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`data-api-key-exchange="binance"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("Binance API Key")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("交易所 / 返回")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("信号来源")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("下单去向")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("raw-json-dialog")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("data-order-json-index")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("showOrderRawJSON")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("copy-raw-json")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("Webhook URL")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("template-webhook-url")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("copy-webhook-url")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte(`new URL("/tvorder"`)) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("target_exchange")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("tpl-target-exchange")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("position-exchange\"><option")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("position-api-id")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("order-okx")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("apiDisplayName")) {
		t.Fatalf("tvbot ui should render exchange-aware API, template and order history controls")
	}
	if !bytes.Contains(ui.Body.Bytes(), []byte("activateTab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("location.hash")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("history.replaceState")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("configuredDefaultTab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("effectiveDefaultTab")) ||
		!bytes.Contains(ui.Body.Bytes(), []byte("syncActiveTabAfterMenuSettings")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("localStorage.getItem")) ||
		bytes.Contains(ui.Body.Bytes(), []byte("tvbot.active_tab")) {
		t.Fatalf("tvbot ui should use hash/default tab navigation without localStorage override")
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
			"default_tab": "orders",
			"menu_items": [
				{"tab":"orders","hidden":true},
				{"tab":"dashboard","hidden":false,"label":"首页"},
				{"tab":"menuSettings","hidden":true,"label":"菜单管理"},
				{"tab":"upgrade","hidden":false,"label":"   "},
				{"tab":"apiKeys","hidden":false,"label":"下单设置"},
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
	if cfg.UI.DefaultTab != "orders" {
		t.Fatalf("default tab should be saved: %#v", cfg.UI)
	}
	if cfg.UI.MenuItems[0].Tab != "orders" || !cfg.UI.MenuItems[0].Hidden {
		t.Fatalf("first menu item should preserve hidden orders: %#v", cfg.UI.MenuItems[0])
	}
	if cfg.UI.MenuItems[1].Tab != "dashboard" || cfg.UI.MenuItems[1].Hidden {
		t.Fatalf("second menu item should preserve visible dashboard: %#v", cfg.UI.MenuItems[1])
	}
	if cfg.UI.MenuItems[1].Label != "首页" {
		t.Fatalf("custom menu label should be saved: %#v", cfg.UI.MenuItems[1])
	}
	if cfg.UI.MenuItems[2].Tab != config.MenuSettingsTab || cfg.UI.MenuItems[2].Hidden || cfg.UI.MenuItems[2].Label != "菜单管理" {
		t.Fatalf("menu settings should be forced visible: %#v", cfg.UI.MenuItems[2])
	}
	if cfg.UI.MenuItems[3].Tab != "upgrade" || cfg.UI.MenuItems[3].Label != "升级" {
		t.Fatalf("blank menu label should fall back to default: %#v", cfg.UI.MenuItems[3])
	}
	if cfg.UI.MenuItems[4].Tab != "apiKeys" || cfg.UI.MenuItems[4].Label != "API Key" {
		t.Fatalf("menu label that belongs to another tab should be repaired: %#v", cfg.UI.MenuItems[4])
	}
	for _, item := range cfg.UI.MenuItems {
		if item.Tab == "unknown" {
			t.Fatalf("unknown menu item should be removed: %#v", cfg.UI.MenuItems)
		}
	}
}

func TestTVBotConfigSavesTableColumns(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/config", bytes.NewReader([]byte(`{
		"ui": {
			"table_columns": {
				"positions": ["upl", "exchange", "bad", "upl"],
				"pending_orders": ["actions", "symbol", "unknown", "actions"]
			}
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
	if len(cfg.UI.TableColumns.Positions) != len(config.DefaultPositionTableColumns) ||
		cfg.UI.TableColumns.Positions[0] != "upl" ||
		cfg.UI.TableColumns.Positions[1] != "exchange" ||
		strings.Contains(strings.Join(cfg.UI.TableColumns.Positions, ","), "bad") {
		t.Fatalf("position table columns should be normalized and saved: %#v", cfg.UI.TableColumns.Positions)
	}
	if len(cfg.UI.TableColumns.PendingOrders) != len(config.DefaultPendingOrderTableColumns) ||
		cfg.UI.TableColumns.PendingOrders[0] != "actions" ||
		cfg.UI.TableColumns.PendingOrders[1] != "symbol" ||
		strings.Contains(strings.Join(cfg.UI.TableColumns.PendingOrders, ","), "unknown") {
		t.Fatalf("pending order table columns should be normalized and saved: %#v", cfg.UI.TableColumns.PendingOrders)
	}
}

func TestTVBotConfigRepairsInvalidDefaultTab(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/config", bytes.NewReader([]byte(`{"ui":{"default_tab":"missing-tab"}}`)))
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
	if cfg.UI.DefaultTab != config.DefaultHomeTab {
		t.Fatalf("invalid default tab should fall back to %q: %#v", config.DefaultHomeTab, cfg.UI)
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
		if got.TargetExchange != trading.ExchangeOKX {
			t.Fatalf("old webhook should default to OKX target exchange: %#v", got)
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
	if bytes.Contains(ordersRR.Body.Bytes(), []byte(signal.Token)) {
		t.Fatalf("orders response leaked webhook token: %s", ordersRR.Body.String())
	}
	var list struct {
		Orders []storage.OrderRecord `json:"orders"`
	}
	if err := json.Unmarshal(ordersRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var foundDuplicate bool
	for _, order := range list.Orders {
		if order.TargetExchange != trading.ExchangeOKX {
			t.Fatalf("orders should default target exchange to OKX: %#v", list.Orders)
		}
		if order.RawJSON == "" || !strings.Contains(order.RawJSON, `"token": "[redacted]"`) || strings.Contains(order.RawJSON, signal.Token) {
			t.Fatalf("order should include redacted raw json: %#v", order)
		}
		if order.Status == storage.StatusDuplicate {
			foundDuplicate = true
		}
	}
	if len(list.Orders) != 2 || !foundDuplicate {
		t.Fatalf("duplicate signal should be listed in history: %#v", list.Orders)
	}
}

func TestTVOrderRoutesTargetExchangeBinance(t *testing.T) {
	srv := newTestServer(t)
	signal := validSignal(t, srv)
	signal.TargetExchange = trading.ExchangeBinance
	signal.APIID = "binance-main"
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
		if got.TargetExchange != trading.ExchangeBinance || got.APIID != "binance-main" {
			t.Fatalf("signal should route to Binance target exchange: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
	records := srv.Orders.List(10)
	if len(records) != 1 || records[0].TargetExchange != trading.ExchangeBinance {
		t.Fatalf("order record should save Binance target exchange: %#v", records)
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
		"token_nonce": "raw-nonce-value",
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
	if got.RawJSON == "" ||
		!strings.Contains(got.RawJSON, `"token": "[redacted]"`) ||
		!strings.Contains(got.RawJSON, `"token_nonce": "[redacted]"`) ||
		strings.Contains(got.RawJSON, "present-but-not-checked-before-signal-validation") ||
		strings.Contains(got.RawJSON, "raw-nonce-value") {
		t.Fatalf("rejected record should include redacted raw json: %#v", got)
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

func TestTVBotBinanceAPIKeysSaveAndTest(t *testing.T) {
	srv := newTestServer(t)
	var sawAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v3/balance":
			sawAPIKey = r.Header.Get("X-MBX-APIKEY")
			_, _ = w.Write([]byte(`[{"asset":"USDT","balance":"1000.25","availableBalance":"900.25","crossWalletBalance":"980.25","updateTime":1784886000000}]`))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	srv.ConfigStore = config.NewStore("", cfg)
	reqBody := []byte(`{"exchange":"binance","id":"main","name":"Binance Main","api_key":"bnabcd12345678wxyz","secret_key":"binance-secret-value","active":true}`)
	req := httptest.NewRequest(http.MethodPut, "/tvbot/api-keys?exchange=binance", bytes.NewReader(reqBody))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("binance-secret-value")) || !bytes.Contains(rr.Body.Bytes(), []byte("bnab...wxyz")) {
		t.Fatalf("bad save response: %s", rr.Body.String())
	}

	testReq := httptest.NewRequest(http.MethodPost, "/tvbot/api-keys/test", bytes.NewReader([]byte(`{"exchange":"binance","id":"main"}`)))
	testReq.SetBasicAuth("admin", "Admin123")
	testRR := httptest.NewRecorder()
	srv.ServeHTTP(testRR, testReq)
	if testRR.Code != http.StatusOK {
		t.Fatalf("test status=%d body=%s", testRR.Code, testRR.Body.String())
	}
	if sawAPIKey != "bnabcd12345678wxyz" {
		t.Fatalf("used api key %q", sawAPIKey)
	}
	if !bytes.Contains(testRR.Body.Bytes(), []byte(`"exchange":"binance"`)) || !bytes.Contains(testRR.Body.Bytes(), []byte(`"api_id":"main"`)) || !bytes.Contains(testRR.Body.Bytes(), []byte(`"eq":"1000.25"`)) {
		t.Fatalf("bad test response: %s", testRR.Body.String())
	}
}

func TestTVBotAnalysisRequiresAdminAndReturnsExchangeSeparatedStats(t *testing.T) {
	srv := newTestServer(t)
	windowStart := srv.now().Add(-24 * time.Hour).UnixMilli()
	oldTradeTime := srv.now().Add(-25 * time.Hour).UnixMilli()
	fillTime1 := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC).UnixMilli()
	fillTime2 := time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC).UnixMilli()
	binanceTradeTime := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC).UnixMilli()
	candleTime1 := time.Date(2026, 7, 23, 2, 0, 0, 0, time.UTC).UnixMilli()
	candleTime2 := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC).UnixMilli()
	var sawBalance, sawCandles, sawFills bool
	expectedBinanceStart := windowStart
	sawBinanceSymbols := map[string]bool{}
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
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","tradeId":"t1b","ordId":"o1","side":"sell","fillPx":"50100","fillSz":"1","fillPnl":"0.5","fee":"-0.02","feeCcy":"USDT","fillTime":"%d"},
				{"instType":"SWAP","instId":"ETH-USDT-SWAP","tradeId":"t2","ordId":"o2","side":"buy","fillPx":"2500","fillSz":"1","fillPnl":"-1","fee":"-0.05","feeCcy":"USDT","fillTime":"%d"},
				{"instType":"SWAP","instId":"OLD-USDT-SWAP","tradeId":"t-old","ordId":"o-old","side":"sell","fillPx":"1","fillSz":"1","fillPnl":"999","fee":"0","feeCcy":"USDT","fillTime":"%d"}
			]}`, fillTime2, fillTime2+1000, fillTime1, oldTradeTime)))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/userTrades" {
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
		if r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key")
		}
		query := r.URL.Query()
		symbol := query.Get("symbol")
		if symbol == "" || query.Get("limit") != "1000" {
			t.Fatalf("bad Binance user trades query: %s", r.URL.RawQuery)
		}
		startMS, _ := strconv.ParseInt(query.Get("startTime"), 10, 64)
		endMS, _ := strconv.ParseInt(query.Get("endTime"), 10, 64)
		if startMS <= 0 || endMS <= 0 || endMS-startMS > int64((7*24*time.Hour).Milliseconds()) {
			t.Fatalf("bad Binance analysis time window: %s", r.URL.RawQuery)
		}
		if expectedBinanceStart > 0 && startMS != expectedBinanceStart {
			t.Fatalf("bad Binance analysis start time: got %d want %d query=%s", startMS, expectedBinanceStart, r.URL.RawQuery)
		}
		sawBinanceSymbols[symbol] = true
		if symbol == "BTCUSDT" && startMS <= binanceTradeTime && endMS >= binanceTradeTime {
			_, _ = w.Write([]byte(fmt.Sprintf(`[
				{"symbol":"BTCUSDT","side":"SELL","positionSide":"BOTH","price":"64000","qty":"0.01","realizedPnl":"4.2","commission":"0.2","commissionAsset":"USDT","time":%d,"id":9001,"orderId":8001},
				{"symbol":"BTCUSDT","side":"SELL","positionSide":"BOTH","price":"64010","qty":"0.02","realizedPnl":"0.8","commission":"0.05","commissionAsset":"USDT","time":%d,"id":9002,"orderId":8001},
				{"symbol":"BTCUSDT","side":"BUY","positionSide":"BOTH","price":"1","qty":"1","realizedPnl":"999","commission":"0","commissionAsset":"USDT","time":%d,"id":9003,"orderId":8002}
			]`, binanceTradeTime, binanceTradeTime+1000, oldTradeTime)))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	srv.BinanceHTTPClient = binanceServer.Client()
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
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:     "binance-main",
		Active: true,
		Credentials: binance.Credentials{
			APIKey:    "binance-key",
			SecretKey: "binance-secret",
		},
	}); err != nil {
		t.Fatal(err)
	}

	unauth := httptest.NewRecorder()
	srv.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/tvbot/analysis", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("analysis without auth code=%d", unauth.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/tvbot/analysis?refresh=true&pnl_days=60&pnl_minutes=1440", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("analysis code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawBalance || !sawCandles || !sawFills {
		t.Fatalf("expected OKX balance, candle and fills calls balance=%v candles=%v fills=%v", sawBalance, sawCandles, sawFills)
	}
	if !sawBinanceSymbols["BTCUSDT"] || !sawBinanceSymbols["ETHUSDT"] {
		t.Fatalf("expected Binance configured symbols to be queried, seen=%#v", sawBinanceSymbols)
	}
	var resp analysisResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.PNLMinutes != 1440 || resp.PNLDays != 1 || resp.BinanceAPIID != "binance-main" {
		t.Fatalf("bad analysis API/window metadata: %#v", resp)
	}
	if resp.Balance.TotalEq != "80078.07" || len(resp.Balance.Details) != 2 || resp.Balance.Details[0].Ccy != "BTC" {
		t.Fatalf("bad balance data: %#v", resp.Balance)
	}
	if len(resp.PricePoints) != 2 || resp.PriceInstID != "USDT-USD" || resp.PriceBar != "1H" {
		t.Fatalf("bad price data: %#v", resp)
	}
	if len(resp.BalancePoints) != 1 || math.Abs(resp.BalancePoints[0].Value-5000) > 0.0000001 {
		t.Fatalf("bad balance points: %#v", resp.BalancePoints)
	}
	snapshots, err := srv.Orders.ListUSDTBalanceSnapshots("default", cfg.Trading.Env, time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].EqUsd != "4996.65" || snapshots[0].BucketTS != time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("analysis did not write USDT balance snapshot: %#v", snapshots)
	}
	if resp.Summary.TradeCount != 3 || resp.Summary.Wins != 2 || resp.Summary.Losses != 1 {
		t.Fatalf("bad summary counts: %#v", resp.Summary)
	}
	if math.Abs(resp.Summary.NetPnL-6.58) > 0.0000001 || math.Abs(resp.Summary.WinRate-(2.0/3.0)) > 0.0000001 {
		t.Fatalf("bad summary metrics: %#v", resp.Summary)
	}
	if len(resp.ExchangeSummaries) != 2 {
		t.Fatalf("expected exchange summaries: %#v", resp.ExchangeSummaries)
	}
	byExchange := map[string]analysisSymbolStats{}
	for _, stats := range resp.ExchangeSummaries {
		byExchange[stats.Exchange] = stats
	}
	if byExchange["okx"].TradeCount != 2 || math.Abs(byExchange["okx"].NetPnL-1.83) > 0.0000001 {
		t.Fatalf("bad OKX exchange summary: %#v", resp.ExchangeSummaries)
	}
	if byExchange["binance"].TradeCount != 1 || math.Abs(byExchange["binance"].NetPnL-4.75) > 0.0000001 {
		t.Fatalf("bad Binance exchange summary: %#v", resp.ExchangeSummaries)
	}
	if len(resp.Symbols) != 3 {
		t.Fatalf("expected symbol stats: %#v", resp.Symbols)
	}
	byExchangeSymbol := map[string]analysisSymbolStats{}
	for _, stats := range resp.Symbols {
		byExchangeSymbol[stats.Exchange+"|"+stats.InstID] = stats
	}
	if byExchangeSymbol["okx|BTC-USDT-SWAP"].TradeCount != 1 || byExchangeSymbol["binance|BTCUSDT"].TradeCount != 1 {
		t.Fatalf("expected exchange-separated symbol stats: %#v", resp.Symbols)
	}
	if len(resp.Trades) != 3 || resp.Trades[0].Exchange != trading.ExchangeBinance || resp.Trades[0].InstID != "BTCUSDT" || resp.Trades[0].Fee != "-0.25" || resp.Trades[0].FillCount != 2 || resp.Trades[0].FillSz != "0.03" {
		t.Fatalf("bad trade history: %#v", resp.Trades)
	}

	expectedBinanceStart = 0
	capReq := httptest.NewRequest(http.MethodGet, "/tvbot/analysis?refresh=true&pnl_minutes=999999", nil)
	capReq.SetBasicAuth("admin", "Admin123")
	capRR := httptest.NewRecorder()
	srv.ServeHTTP(capRR, capReq)
	if capRR.Code != http.StatusOK {
		t.Fatalf("analysis cap code=%d body=%s", capRR.Code, capRR.Body.String())
	}
	var capResp analysisResponse
	if err := json.Unmarshal(capRR.Body.Bytes(), &capResp); err != nil {
		t.Fatal(err)
	}
	if capResp.PNLMinutes != maxAnalysisPNLMinutes || capResp.PNLDays != maxAnalysisPNLDays {
		t.Fatalf("bad capped analysis window: %#v", capResp)
	}
}

func TestUSDTBalanceSamplerStoresConfiguredAccounts(t *testing.T) {
	srv := newTestServer(t)
	if usdtSampleInterval != time.Minute {
		t.Fatalf("USDT balance sampler interval = %s, want 1m", usdtSampleInterval)
	}
	observedAt := time.Date(2026, 7, 24, 3, 4, 45, 0, time.UTC)
	srv.Now = func() time.Time { return observedAt }
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
		if len(snapshots) != 1 || snapshots[0].EqUsd != eqUSD || snapshots[0].BucketTS != observedAt.Truncate(time.Minute).UnixMilli() {
			t.Fatalf("bad %s snapshots: %#v", apiID, snapshots)
		}
	}
}

func TestTVBotPositionsRequiresAdminAndReturnsCurrentPositions(t *testing.T) {
	srv := newTestServer(t)
	entryFillTime := srv.now().Add(-2 * time.Hour)
	reduceFillTime := srv.now().Add(-time.Hour)
	var sawPositions, sawFills bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/positions":
			sawPositions = true
			if r.URL.Query().Get("instType") != "SWAP" {
				t.Fatalf("bad positions query: %s", r.URL.RawQuery)
			}
			if r.Header.Get("x-simulated-trading") != "1" || r.Header.Get("OK-ACCESS-KEY") != "key" {
				t.Fatalf("missing private OKX headers")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"isolated","posId":"1","posSide":"long","pos":"0.5","availPos":"0.5","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","liqPx":"51000","notionalUsd":"32500","margin":"6500","mgnRatio":"100","uTime":"1784880000000"},
				{"instType":"SWAP","instId":"ETH-USDT-SWAP","mgnMode":"isolated","posId":"2","posSide":"short","pos":"0","availPos":"0","avgPx":"2500","markPx":"2490","upl":"0","uplRatio":"0","lever":"5","notionalUsd":"0","uTime":"1784880000000"}
			]}`))
		case "/api/v5/trade/fills-history":
			sawFills = true
			if r.URL.Query().Get("instType") != "SWAP" || r.URL.Query().Get("limit") != "100" {
				t.Fatalf("bad fills query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","tradeId":"trade-2","ordId":"order-2","side":"sell","posSide":"long","fillSz":"0.2","fillTime":"%d"},
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","tradeId":"trade-1","ordId":"order-1","side":"buy","posSide":"long","fillSz":"0.7","fillTime":"%d"}
			]}`, reduceFillTime.UnixMilli(), entryFillTime.UnixMilli())))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"0.01","lotSz":"0.01","minSz":"0.01"},
				{"instId":"ETH-USDT-SWAP","tickSz":"0.01","ctVal":"0.1","lotSz":"0.001","minSz":"0.001"}
			]}`))
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
	if !sawFills {
		t.Fatal("expected OKX fills-history call")
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
	if resp.Positions[0].EntryFillTime != entryFillTime.UTC().Format(time.RFC3339Nano) || resp.Positions[0].HoldingSeconds != int64((2*time.Hour).Seconds()) || resp.Positions[0].EntryTimeSource != entryTimeSourceOKXFills {
		t.Fatalf("bad position entry time: %#v", resp.Positions[0])
	}
	if resp.Positions[0].PricePrecision == nil || *resp.Positions[0].PricePrecision != 1 || resp.Positions[0].QuantityPrecision == nil || *resp.Positions[0].QuantityPrecision != 2 {
		t.Fatalf("bad OKX position precision: %#v", resp.Positions[0])
	}
}

func TestTVBotBinancePositionsPendingOrdersAndBalanceOverview(t *testing.T) {
	srv := newTestServer(t)
	seen := map[string]bool{}
	entryTradeTime := srv.now().Add(-2 * time.Hour)
	reduceTradeTime := srv.now().Add(-time.Hour)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/exchangeInfo" && r.URL.Path != "/fapi/v1/ticker/bookTicker" && r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/fapi/v3/positionRisk":
			_, _ = w.Write([]byte(`[
				{"symbol":"BTCUSDT","positionSide":"BOTH","positionAmt":"0.2","entryPrice":"50000","markPrice":"50100","unRealizedProfit":"20","liquidationPrice":"40000","isolatedMargin":"1000","notional":"10020","marginAsset":"USDT","leverage":"10","marginType":"isolated","updateTime":1784880000000},
				{"symbol":"ETHUSDT","positionSide":"BOTH","positionAmt":"0","entryPrice":"3000","markPrice":"3000","unRealizedProfit":"0","notional":"0","marginAsset":"USDT","updateTime":1784880000000}
			]`))
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":123456,"clientOrderId":"client-1","price":"49900","origQty":"0.2","executedQty":"0.1","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","time":1784880000000,"updateTime":1784880005000}]`))
		case "/fapi/v1/openAlgoOrders":
			if r.URL.Query().Get("algoType") != "CONDITIONAL" {
				t.Fatalf("bad Binance open algo query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[
				{"symbol":"BTCUSDT","algoId":900,"clientAlgoId":"tp-900","algoType":"CONDITIONAL","orderType":"TAKE_PROFIT_MARKET","side":"SELL","positionSide":"BOTH","quantity":"0.1","algoStatus":"NEW","reduceOnly":true},
				{"symbol":"ETHUSDT","algoId":901,"clientAlgoId":"sl-901","algoType":"CONDITIONAL","orderType":"STOP_MARKET","side":"BUY","positionSide":"BOTH","quantity":"0.2","algoStatus":"NEW","reduceOnly":true}
			]`))
		case "/fapi/v1/userTrades":
			if r.URL.Query().Get("symbol") != "BTCUSDT" || r.URL.Query().Get("limit") != "1000" {
				t.Fatalf("bad user trades query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`[
				{"symbol":"BTCUSDT","side":"SELL","positionSide":"BOTH","qty":"0.1","time":%d,"id":2,"orderId":2},
				{"symbol":"BTCUSDT","side":"BUY","positionSide":"BOTH","qty":"0.3","time":%d,"id":1,"orderId":1}
			]`, reduceTradeTime.UnixMilli(), entryTradeTime.UnixMilli())))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":2,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v1/ticker/bookTicker":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("bad book ticker query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"49999.9","bidQty":"1","askPrice":"50000.1","askQty":"2","time":1784880000000}`))
		case "/fapi/v3/balance":
			_, _ = w.Write([]byte(`[{"asset":"USDT","balance":"2000.5","availableBalance":"1500.25","crossWalletBalance":"1800.25","updateTime":1784886000000}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = ts.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = ts.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:     "main",
		Active: true,
		Credentials: binance.Credentials{
			APIKey:    "binance-key",
			SecretKey: "binance-secret",
		},
	}); err != nil {
		t.Fatal(err)
	}

	positionsReq := httptest.NewRequest(http.MethodGet, "/tvbot/positions?exchange=binance", nil)
	positionsReq.SetBasicAuth("admin", "Admin123")
	positionsRR := httptest.NewRecorder()
	srv.ServeHTTP(positionsRR, positionsReq)
	if positionsRR.Code != http.StatusOK {
		t.Fatalf("positions status=%d body=%s", positionsRR.Code, positionsRR.Body.String())
	}
	var positions positionsResponse
	if err := json.Unmarshal(positionsRR.Body.Bytes(), &positions); err != nil {
		t.Fatal(err)
	}
	if positions.Exchange != trading.ExchangeBinance || positions.APIID != "main" || len(positions.Positions) != 1 || positions.Positions[0].InstID != "BTCUSDT" {
		t.Fatalf("bad Binance positions response: %#v", positions)
	}
	if positions.Positions[0].EntryFillTime != entryTradeTime.UTC().Format(time.RFC3339Nano) || positions.Positions[0].HoldingSeconds != int64((2*time.Hour).Seconds()) || positions.Positions[0].EntryTimeSource != entryTimeSourceBinanceTrade {
		t.Fatalf("bad Binance position entry time: %#v", positions.Positions[0])
	}
	if positions.Positions[0].PricePrecision == nil || *positions.Positions[0].PricePrecision != 2 || positions.Positions[0].QuantityPrecision == nil || *positions.Positions[0].QuantityPrecision != 3 {
		t.Fatalf("bad Binance position precision: %#v", positions.Positions[0])
	}

	pendingReq := httptest.NewRequest(http.MethodGet, "/tvbot/pending-orders?exchange=binance", nil)
	pendingReq.SetBasicAuth("admin", "Admin123")
	pendingRR := httptest.NewRecorder()
	srv.ServeHTTP(pendingRR, pendingReq)
	if pendingRR.Code != http.StatusOK {
		t.Fatalf("pending status=%d body=%s", pendingRR.Code, pendingRR.Body.String())
	}
	var pending pendingOrdersResponse
	if err := json.Unmarshal(pendingRR.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Exchange != trading.ExchangeBinance || pending.APIID != "main" || pending.Count != 1 || pending.NormalCount != 1 || pending.AlgoCount != 2 || pending.TotalCount != 3 || len(pending.Orders) != 1 || pending.Orders[0].OrdID != "123456" || pending.Orders[0].MidPx != "50000" || pending.Orders[0].ChasePx != "49999.9" || pending.Orders[0].Margin != "998" || pending.Orders[0].PriceError != "" {
		t.Fatalf("bad Binance pending response: %#v", pending)
	}
	if pending.Orders[0].PricePrecision == nil || *pending.Orders[0].PricePrecision != 2 || pending.Orders[0].QuantityPrecision == nil || *pending.Orders[0].QuantityPrecision != 3 {
		t.Fatalf("bad Binance pending precision: %#v", pending.Orders[0])
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/tvbot/balances/overview?days=3", nil)
	overviewReq.SetBasicAuth("admin", "Admin123")
	overviewRR := httptest.NewRecorder()
	srv.ServeHTTP(overviewRR, overviewReq)
	if overviewRR.Code != http.StatusOK {
		t.Fatalf("overview status=%d body=%s", overviewRR.Code, overviewRR.Body.String())
	}
	var overview balanceOverviewResponse
	if err := json.Unmarshal(overviewRR.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	var binanceOverview exchangeBalanceOverview
	for _, item := range overview.Exchanges {
		if item.Exchange == trading.ExchangeBinance {
			binanceOverview = item
		}
	}
	if binanceOverview.Status != "ok" || binanceOverview.APIID != "main" || len(binanceOverview.BalancePoints) != 1 || binanceOverview.Balance.Details[0].Eq != "2000.5" {
		t.Fatalf("bad Binance overview: %#v", overview)
	}
	for _, path := range []string{"/fapi/v3/positionRisk", "/fapi/v1/userTrades", "/fapi/v1/openOrders", "/fapi/v1/openAlgoOrders", "/fapi/v1/exchangeInfo", "/fapi/v1/ticker/bookTicker", "/fapi/v3/balance"} {
		if !seen[path] {
			t.Fatalf("expected %s to be called, seen=%#v", path, seen)
		}
	}
}

func TestTVBotPendingOrdersRequiresAdminAndReturnsCurrentOrders(t *testing.T) {
	srv := newTestServer(t)
	var sawPendingOrders, sawAlgoOrders bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			sawPendingOrders = true
			if r.URL.Query().Get("instType") != "SWAP" {
				t.Fatalf("bad pending orders query: %s", r.URL.RawQuery)
			}
			if r.Header.Get("x-simulated-trading") != "1" || r.Header.Get("OK-ACCESS-KEY") != "key" {
				t.Fatalf("missing private OKX headers")
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"ETH-USDT-SWAP","ordId":"200","clOrdId":"client-200","tdMode":"cross","side":"sell","posSide":"short","ordType":"limit","px":"2500","sz":"1","accFillSz":"0","state":"live","lever":"5","cTime":"1784880060000","uTime":"1784880060000"},
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"buy","posSide":"long","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0.1","avgPx":"63950","state":"partially_filled","lever":"5","cTime":"1784880000000","uTime":"1784880060000"}
			]}`))
		case "/api/v5/trade/orders-algo-pending":
			sawAlgoOrders = true
			ordType := r.URL.Query().Get("ordType")
			if r.URL.Query().Get("instType") != "SWAP" || ordType == "" {
				t.Fatalf("bad pending algo query: %s", r.URL.RawQuery)
			}
			if ordType != "conditional" {
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
				{"instType":"SWAP","instId":"BTC-USDT-SWAP","algoId":"900","algoClOrdId":"algo-900","side":"sell","posSide":"long","ordType":"conditional","sz":"1","state":"live"}
			]}`))
		case "/api/v5/public/instruments":
			instID := r.URL.Query().Get("instId")
			tick := "0.1"
			if instID == "ETH-USDT-SWAP" {
				tick = "0.01"
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"` + instID + `","tickSz":"` + tick + `","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			switch r.URL.Query().Get("instId") {
			case "BTC-USDT-SWAP":
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
			case "ETH-USDT-SWAP":
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"ETH-USDT-SWAP","bidPx":"2500","askPx":"2500.1","last":"2500.05"}]}`))
			default:
				t.Fatalf("unexpected ticker instId %s", r.URL.Query().Get("instId"))
			}
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
	srv.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/tvbot/pending-orders", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("pending orders without auth code=%d", unauth.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/tvbot/pending-orders?api_id=default", nil)
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pending orders code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawPendingOrders {
		t.Fatal("expected OKX pending orders call")
	}
	if !sawAlgoOrders {
		t.Fatal("expected OKX pending algo orders call")
	}
	var resp pendingOrdersResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.APIID != "default" || resp.Count != 2 || resp.NormalCount != 2 || resp.AlgoCount != 1 || resp.TotalCount != 3 || len(resp.Orders) != 2 {
		t.Fatalf("bad pending orders response: %#v", resp)
	}
	if resp.Orders[0].InstID != "BTC-USDT-SWAP" || resp.Orders[0].OrdID != "100" || resp.Orders[0].AccFillSz != "0.1" || resp.Orders[0].MidPx != "64000" || resp.Orders[0].ChasePx != "63999.9" || resp.Orders[0].Margin != "5120" {
		t.Fatalf("bad pending order sorting/data: %#v", resp.Orders)
	}
	if resp.Orders[0].PricePrecision == nil || *resp.Orders[0].PricePrecision != 1 || resp.Orders[0].QuantityPrecision == nil || *resp.Orders[0].QuantityPrecision != 0 {
		t.Fatalf("bad OKX pending precision: %#v", resp.Orders[0])
	}
}

func TestTVBotPendingOrderChaseAmendsPassiveBuyAndStopsWhenOrderDisappears(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = 10 * time.Millisecond
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var mu sync.Mutex
	pendingCalls := 0
	var amendBodies []map[string]string
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			mu.Lock()
			pendingCalls++
			call := pendingCalls
			mu.Unlock()
			if r.Header.Get("OK-ACCESS-KEY") != "key" {
				t.Fatalf("missing private OKX headers")
			}
			if call == 1 {
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"buy","posSide":"long","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0","state":"live","cTime":"1784880000000"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/amend-order":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			amendBodies = append(amendBodies, body)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			t.Fatal("追单不应调用撤单接口")
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("chase code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp pendingOrderChaseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Px != "63999.9" || resp.MidPx != "64000" || resp.Status != "running" {
		t.Fatalf("bad chase response: %#v", resp)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "BTC-USDT-SWAP", OrdID: "100", ClOrdID: "client-100"})
	for i := 0; i < 100 && pendingOrderChaseJobs.activeKey(key); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("chase job should stop when pending order disappears")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(amendBodies) != 1 || amendBodies[0]["instId"] != "BTC-USDT-SWAP" || amendBodies[0]["ordId"] != "100" || amendBodies[0]["newPx"] != "63999.9" {
		t.Fatalf("bad amend bodies: %#v", amendBodies)
	}
}

func TestTVBotBinancePendingOrderChaseAmendsPassiveBuyAndStops(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = time.Hour
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var modifyForms []url.Values
	var cancelCalled bool
	var marketCalled bool
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/exchangeInfo" && r.URL.Path != "/fapi/v1/ticker/bookTicker" && r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/fapi/v1/openOrders":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("bad open orders query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":123456,"clientOrderId":"client-1","price":"50000","origQty":"0.2","executedQty":"0","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","time":1784880000000,"updateTime":1784880005000}]`))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":2,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v1/ticker/bookTicker":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"49999.9","bidQty":"1","askPrice":"50000.1","askQty":"2","time":1784880000000}`))
		case "/fapi/v1/order":
			switch r.Method {
			case http.MethodPut:
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				modifyForms = append(modifyForms, cloneValues(r.Form))
				_, _ = w.Write([]byte(`{"orderId":123456,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"client-1","price":"49999.9","origQty":"0.2","executedQty":"0","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			case http.MethodDelete:
				cancelCalled = true
				_, _ = w.Write([]byte(`{"orderId":123456,"symbol":"BTCUSDT","status":"CANCELED","clientOrderId":"client-1","price":"49999.9","origQty":"0.2","executedQty":"0","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			case http.MethodPost:
				marketCalled = true
				_, _ = w.Write([]byte(`{"orderId":999,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"market-1","price":"0","origQty":"0.2","executedQty":"0","type":"MARKET","side":"BUY","positionSide":"BOTH"}`))
			default:
				t.Fatalf("unexpected order method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = binanceServer.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:          "main",
		Active:      true,
		Credentials: binance.Credentials{APIKey: "binance-key", SecretKey: "binance-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"exchange":"binance","api_id":"main","inst_id":"BTCUSDT","ord_id":"123456","cl_ord_id":"client-1"}`
	start := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(body))
	start.Header.Set("Content-Type", "application/json")
	start.SetBasicAuth("admin", "Admin123")
	startRR := httptest.NewRecorder()
	srv.ServeHTTP(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start chase code=%d body=%s", startRR.Code, startRR.Body.String())
	}
	var resp pendingOrderChaseResponse
	if err := json.Unmarshal(startRR.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Px != "49999.9" || resp.MidPx != "50000" || resp.Status != "running" {
		t.Fatalf("bad Binance chase response: %#v", resp)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{Exchange: trading.ExchangeBinance, APIID: "main", InstID: "BTCUSDT", OrdID: "123456", ClOrdID: "client-1"})
	if !pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("Binance chase job should be active")
	}
	stop := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase/stop", strings.NewReader(body))
	stop.Header.Set("Content-Type", "application/json")
	stop.SetBasicAuth("admin", "Admin123")
	stopRR := httptest.NewRecorder()
	srv.ServeHTTP(stopRR, stop)
	if stopRR.Code != http.StatusOK {
		t.Fatalf("stop chase code=%d body=%s", stopRR.Code, stopRR.Body.String())
	}
	if len(modifyForms) != 1 {
		t.Fatalf("expected one modify request, got %#v", modifyForms)
	}
	modify := modifyForms[0]
	if modify.Get("symbol") != "BTCUSDT" || modify.Get("side") != "BUY" || modify.Get("quantity") != "0.2" || modify.Get("price") != "49999.9" || modify.Get("orderId") != "123456" {
		t.Fatalf("bad modify form: %#v", modify)
	}
	if cancelCalled || marketCalled {
		t.Fatalf("stop should not cancel or market fallback cancel=%v market=%v", cancelCalled, marketCalled)
	}
}

func TestTVBotBinancePendingOrderChaseFallsBackToMarketAfterTimeout(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = 20 * time.Millisecond
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var mu sync.Mutex
	var cancelForms []url.Values
	var marketForms []url.Values
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/fapi/v1/exchangeInfo" && r.URL.Path != "/fapi/v1/ticker/bookTicker" && r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/fapi/v1/openOrders":
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":123456,"clientOrderId":"client-1","price":"50000","origQty":"0.2","executedQty":"0.1","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","time":1784880000000,"updateTime":1784880005000}]`))
		case "/fapi/v1/exchangeInfo":
			_, _ = w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","pricePrecision":2,"quantityPrecision":3,"filters":[{"filterType":"PRICE_FILTER","tickSize":"0.10"},{"filterType":"LOT_SIZE","minQty":"0.001","stepSize":"0.001"}]}]}`))
		case "/fapi/v1/ticker/bookTicker":
			_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"49999.9","bidQty":"1","askPrice":"50000.1","askQty":"2","time":1784880000000}`))
		case "/fapi/v1/order":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			switch r.Method {
			case http.MethodPut:
				_, _ = w.Write([]byte(`{"orderId":123456,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"client-1","price":"49999.9","origQty":"0.2","executedQty":"0.1","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			case http.MethodDelete:
				mu.Lock()
				cancelForms = append(cancelForms, cloneValues(r.Form))
				mu.Unlock()
				_, _ = w.Write([]byte(`{"orderId":123456,"symbol":"BTCUSDT","status":"CANCELED","clientOrderId":"client-1","price":"49999.9","origQty":"0.2","executedQty":"0.1","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
			case http.MethodPost:
				mu.Lock()
				marketForms = append(marketForms, cloneValues(r.Form))
				mu.Unlock()
				_, _ = w.Write([]byte(`{"orderId":999,"symbol":"BTCUSDT","status":"NEW","clientOrderId":"market-1","price":"0","origQty":"0.1","executedQty":"0","type":"MARKET","side":"BUY","positionSide":"BOTH"}`))
			default:
				t.Fatalf("unexpected order method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = binanceServer.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:          "main",
		Active:      true,
		Credentials: binance.Credentials{APIKey: "binance-key", SecretKey: "binance-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"exchange":"binance","api_id":"main","inst_id":"BTCUSDT","ord_id":"123456","cl_ord_id":"client-1"}`
	start := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(body))
	start.Header.Set("Content-Type", "application/json")
	start.SetBasicAuth("admin", "Admin123")
	startRR := httptest.NewRecorder()
	srv.ServeHTTP(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start chase code=%d body=%s", startRR.Code, startRR.Body.String())
	}
	for i := 0; i < 100; i++ {
		mu.Lock()
		done := len(marketForms) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{Exchange: trading.ExchangeBinance, APIID: "main", InstID: "BTCUSDT", OrdID: "123456", ClOrdID: "client-1"})
	for i := 0; i < 100 && pendingOrderChaseJobs.activeKey(key); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("Binance chase job should stop after market fallback")
	}
	if len(cancelForms) != 1 || cancelForms[0].Get("symbol") != "BTCUSDT" || cancelForms[0].Get("orderId") != "123456" {
		t.Fatalf("bad cancel forms: %#v", cancelForms)
	}
	if len(marketForms) != 1 {
		t.Fatalf("expected one market order, got %#v", marketForms)
	}
	market := marketForms[0]
	if market.Get("symbol") != "BTCUSDT" || market.Get("side") != "BUY" || market.Get("type") != "MARKET" || market.Get("quantity") != "0.1" || market.Get("positionSide") != "BOTH" || market.Get("newClientOrderId") == "" {
		t.Fatalf("bad market fallback form: %#v", market)
	}
}

func TestTVBotBinancePendingOrderCancelStopsChaseAndCancels(t *testing.T) {
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var cancelForms []url.Values
	binanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-MBX-APIKEY") != "binance-key" {
			t.Fatalf("missing Binance API key for %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/fapi/v1/openOrders":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("bad open orders query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"symbol":"BTCUSDT","orderId":123456,"clientOrderId":"client-1","price":"50000","origQty":"0.2","executedQty":"0","side":"BUY","positionSide":"BOTH","type":"LIMIT","status":"NEW","time":1784880000000,"updateTime":1784880005000}]`))
		case "/fapi/v1/order":
			if r.Method != http.MethodDelete {
				t.Fatalf("cancel should only call DELETE /fapi/v1/order, got %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			cancelForms = append(cancelForms, cloneValues(r.Form))
			_, _ = w.Write([]byte(`{"orderId":123456,"symbol":"BTCUSDT","status":"CANCELED","clientOrderId":"client-1","price":"50000","origQty":"0.2","executedQty":"0","type":"LIMIT","side":"BUY","positionSide":"BOTH"}`))
		default:
			t.Fatalf("unexpected Binance path %s", r.URL.Path)
		}
	}))
	defer binanceServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BinanceDemoBaseURL = binanceServer.URL
	srv.ConfigStore = config.NewStore("", cfg)
	srv.BinanceHTTPClient = binanceServer.Client()
	if _, err := srv.BinanceCredentials.UpdateAccount(binance.CredentialAccountUpdate{
		ID:          "main",
		Active:      true,
		Credentials: binance.Credentials{APIKey: "binance-key", SecretKey: "binance-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"exchange":"binance","api_id":"main","inst_id":"BTCUSDT","ord_id":"123456","cl_ord_id":"client-1"}`
	key := pendingOrderChaseKey(pendingOrderChaseRequest{Exchange: trading.ExchangeBinance, APIID: "main", InstID: "BTCUSDT", OrdID: "123456", ClOrdID: "client-1"})
	_, chaseCancel := context.WithCancel(context.Background())
	if !pendingOrderChaseJobs.start(key, chaseCancel) {
		t.Fatal("failed to seed chase job")
	}
	req := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp pendingOrderChaseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "canceled" || resp.APIID != "main" || resp.OrdID != "123456" || resp.ClOrdID != "client-1" {
		t.Fatalf("bad cancel response: %#v", resp)
	}
	if pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("cancel should stop active Binance chase job")
	}
	if len(cancelForms) != 1 || cancelForms[0].Get("symbol") != "BTCUSDT" || cancelForms[0].Get("orderId") != "123456" {
		t.Fatalf("bad cancel forms: %#v", cancelForms)
	}
}

func TestTVBotPendingOrderChaseRebuildsNakedOrderWithRiskControls(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = time.Hour
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var cancelBodies []map[string]string
	var orderBodies []map[string]any
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"buy","posSide":"long","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0.1","state":"live","cTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/cancel-order":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			cancelBodies = append(cancelBodies, body)
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			orderBodies = append(orderBodies, body)
			clOrdID, _ := body["clOrdId"].(string)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":"0","msg":"","data":[{"ordId":"101","clOrdId":%q,"sCode":"0","sMsg":""}]}`, clOrdID)))
		case "/api/v5/trade/amend-order":
			t.Fatal("naked protected chase should rebuild instead of amend original order")
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.TakeProfitPct = 2
	cfg.Trading.StopLossPct = 1
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("chase code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp pendingOrderChaseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "running" || resp.OrdID != "101" || resp.ClOrdID == "" || resp.Px != "63999.9" {
		t.Fatalf("bad chase response: %#v", resp)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "BTC-USDT-SWAP", OrdID: "101", ClOrdID: resp.ClOrdID})
	defer pendingOrderChaseJobs.stop(key)
	if !pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("rebuilt order should be tracked for chase")
	}
	if len(cancelBodies) != 1 || cancelBodies[0]["ordId"] != "100" {
		t.Fatalf("bad cancel bodies: %#v", cancelBodies)
	}
	if len(orderBodies) != 1 {
		t.Fatalf("expected rebuilt order body, got %#v", orderBodies)
	}
	order := orderBodies[0]
	if order["instId"] != "BTC-USDT-SWAP" || order["tdMode"] != "isolated" || order["side"] != "buy" || order["posSide"] != "long" || order["ordType"] != "limit" || order["px"] != "63999.9" || order["sz"] != "0.4" {
		t.Fatalf("bad rebuilt limit order: %#v", order)
	}
	attach, ok := order["attachAlgoOrds"].([]any)
	if !ok || len(attach) != 1 {
		t.Fatalf("missing attach algo on rebuilt order: %#v", order)
	}
	first, ok := attach[0].(map[string]any)
	if !ok || first["tpTriggerRatio"] != "0.02" || first["slTriggerRatio"] != "0.01" || first["tpOrdPx"] != "-1" || first["slOrdPx"] != "-1" {
		t.Fatalf("bad rebuilt attach algo: %#v", attach)
	}
}

func TestTVBotPendingOrderChaseAmendsExistingRiskControls(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = time.Hour
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var amendBodies []map[string]any
	var cancelCalled bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"sell","posSide":"short","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0","state":"live","attachAlgoOrds":[{"attachAlgoClOrdId":"client-100A","tpTriggerRatio":"-0.01","slTriggerRatio":"0.005"}],"cTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/amend-order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			amendBodies = append(amendBodies, body)
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			cancelCalled = true
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
		case "/api/v5/trade/order":
			t.Fatal("existing risk controls should amend, not rebuild")
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.TakeProfitPct = 3
	cfg.Trading.StopLossPct = 1.5
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("chase code=%d body=%s", rr.Code, rr.Body.String())
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "BTC-USDT-SWAP", OrdID: "100", ClOrdID: "client-100"})
	defer pendingOrderChaseJobs.stop(key)
	if cancelCalled || len(amendBodies) != 1 {
		t.Fatalf("expected one amend and no cancel: amend=%#v cancel=%v", amendBodies, cancelCalled)
	}
	amend := amendBodies[0]
	if amend["instId"] != "BTC-USDT-SWAP" || amend["ordId"] != "100" || amend["newPx"] != "64000.1" {
		t.Fatalf("bad amend body: %#v", amend)
	}
	attach, ok := amend["attachAlgoOrds"].([]any)
	if !ok || len(attach) != 1 {
		t.Fatalf("missing attach algo on amend: %#v", amend)
	}
	first, ok := attach[0].(map[string]any)
	if !ok || first["tpTriggerRatio"] != "-0.03" || first["slTriggerRatio"] != "0.015" || first["tpOrdPx"] != "-1" || first["slOrdPx"] != "-1" {
		t.Fatalf("bad amend attach algo: %#v", attach)
	}
}

func TestTVBotPendingOrderChaseRebuildsExistingTrailingRiskControls(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = time.Hour
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var cancelBodies []map[string]string
	var orderBodies []map[string]any
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"TRX-USDT-SWAP","ordId":"3779916965005234176","clOrdId":"TV17851527039509FE4C27EA093","tdMode":"isolated","side":"buy","posSide":"net","ordType":"limit","px":"0.3298","sz":"1.51","accFillSz":"0","state":"live","attachAlgoOrds":[{"attachAlgoClOrdId":"TV17851527039509FE4C27EA093T","attachAlgoId":"3779916964978577408","callbackRatio":"0.035"}],"cTime":"1785152703989"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"TRX-USDT-SWAP","tickSz":"0.0001","ctVal":"100","lotSz":"0.01","minSz":"0.01"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"TRX-USDT-SWAP","bidPx":"0.3306","askPx":"0.3308","last":"0.3307"}]}`))
		case "/api/v5/trade/cancel-order":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			cancelBodies = append(cancelBodies, body)
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"3779916965005234176","clOrdId":"TV17851527039509FE4C27EA093","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			orderBodies = append(orderBodies, body)
			clOrdID, _ := body["clOrdId"].(string)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":"0","msg":"","data":[{"ordId":"new-trx","clOrdId":%q,"sCode":"0","sMsg":""}]}`, clOrdID)))
		case "/api/v5/trade/amend-order":
			t.Fatal("existing trailing protection should rebuild instead of amend")
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.RiskType = string(trading.RiskTrailing)
	cfg.Trading.TrailingPct = 3.5
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(`{"api_id":"default","inst_id":"TRX-USDT-SWAP","ord_id":"3779916965005234176","cl_ord_id":"TV17851527039509FE4C27EA093"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("chase code=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(cancelBodies) != 1 || cancelBodies[0]["instId"] != "TRX-USDT-SWAP" || cancelBodies[0]["ordId"] != "3779916965005234176" {
		t.Fatalf("bad cancel bodies: %#v", cancelBodies)
	}
	if len(orderBodies) != 1 {
		t.Fatalf("expected one replacement order, got %#v", orderBodies)
	}
	replacement := orderBodies[0]
	if replacement["instId"] != "TRX-USDT-SWAP" || replacement["tdMode"] != "isolated" || replacement["side"] != "buy" || replacement["ordType"] != "limit" || replacement["px"] != "0.3306" || replacement["sz"] != "1.51" {
		t.Fatalf("bad replacement order: %#v", replacement)
	}
	replacementClOrdID, _ := replacement["clOrdId"].(string)
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "TRX-USDT-SWAP", OrdID: "new-trx", ClOrdID: replacementClOrdID})
	defer pendingOrderChaseJobs.stop(key)
	if _, ok := replacement["posSide"]; ok {
		t.Fatalf("net-mode replacement should not send posSide: %#v", replacement)
	}
	attach, ok := replacement["attachAlgoOrds"].([]any)
	if !ok || len(attach) != 1 {
		t.Fatalf("missing trailing attach algo: %#v", replacement)
	}
	first, ok := attach[0].(map[string]any)
	if !ok || first["ordType"] != "move_order_stop" || first["callbackRatio"] != "0.035" {
		t.Fatalf("bad trailing attach algo: %#v", attach)
	}
}

func TestTVBotPendingOrderChaseStopDoesNotCancelOrder(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var amendCount int
	var cancelCalled bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"isolated","side":"sell","posSide":"short","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0","state":"live","cTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/amend-order":
			amendCount++
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			cancelCalled = true
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.RiskType = string(trading.RiskNone)
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`
	start := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(body))
	start.Header.Set("Content-Type", "application/json")
	start.SetBasicAuth("admin", "Admin123")
	startRR := httptest.NewRecorder()
	srv.ServeHTTP(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start chase code=%d body=%s", startRR.Code, startRR.Body.String())
	}
	stop := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase/stop", strings.NewReader(body))
	stop.Header.Set("Content-Type", "application/json")
	stop.SetBasicAuth("admin", "Admin123")
	stopRR := httptest.NewRecorder()
	srv.ServeHTTP(stopRR, stop)
	if stopRR.Code != http.StatusOK {
		t.Fatalf("stop chase code=%d body=%s", stopRR.Code, stopRR.Body.String())
	}
	var resp pendingOrderChaseResponse
	if err := json.Unmarshal(stopRR.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "stopped" {
		t.Fatalf("bad stop response: %#v", resp)
	}
	if amendCount != 1 || cancelCalled {
		t.Fatalf("stop should only cancel background job: amendCount=%d cancelCalled=%v", amendCount, cancelCalled)
	}
}

func TestTVBotPendingOrderChaseFallsBackToMarketAfterTimeout(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = 20 * time.Millisecond
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var mu sync.Mutex
	cancelled := false
	var cancelBodies []map[string]string
	var marketBodies []map[string]any
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			mu.Lock()
			isCancelled := cancelled
			mu.Unlock()
			if isCancelled {
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"cross","side":"sell","posSide":"net","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0.1","state":"live","reduceOnly":"true","cTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/amend-order":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			cancelBodies = append(cancelBodies, body)
			cancelled = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			marketBodies = append(marketBodies, body)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"200","clOrdId":"market-200","sCode":"0","sMsg":""}]}`))
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
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`
	start := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(body))
	start.Header.Set("Content-Type", "application/json")
	start.SetBasicAuth("admin", "Admin123")
	startRR := httptest.NewRecorder()
	srv.ServeHTTP(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start chase code=%d body=%s", startRR.Code, startRR.Body.String())
	}

	for i := 0; i < 100; i++ {
		mu.Lock()
		done := len(marketBodies) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "BTC-USDT-SWAP", OrdID: "100", ClOrdID: "client-100"})
	for i := 0; i < 100 && pendingOrderChaseJobs.activeKey(key); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("chase job should stop after market fallback")
	}
	if len(cancelBodies) != 1 || cancelBodies[0]["instId"] != "BTC-USDT-SWAP" || cancelBodies[0]["ordId"] != "100" {
		t.Fatalf("bad cancel bodies: %#v", cancelBodies)
	}
	if len(marketBodies) != 1 {
		t.Fatalf("expected one market order, got %#v", marketBodies)
	}
	market := marketBodies[0]
	if market["instId"] != "BTC-USDT-SWAP" || market["tdMode"] != "cross" || market["side"] != "sell" || market["ordType"] != "market" || market["sz"] != "0.4" || market["reduceOnly"] != true {
		t.Fatalf("bad market fallback order: %#v", market)
	}
	if _, ok := market["attachAlgoOrds"]; ok {
		t.Fatalf("reduce-only fallback should not attach risk controls: %#v", market)
	}
	if _, ok := market["posSide"]; ok {
		t.Fatalf("net-mode market fallback should not send posSide: %#v", market)
	}
}

func TestTVBotPendingOrderChaseFallbackMarketIncludesRiskControls(t *testing.T) {
	oldInterval := pendingOrderChaseInterval
	oldTimeout := pendingOrderChaseTimeout
	oldJobs := pendingOrderChaseJobs
	pendingOrderChaseInterval = time.Hour
	pendingOrderChaseTimeout = 20 * time.Millisecond
	pendingOrderChaseJobs = newPendingOrderChaseRegistry()
	defer func() {
		pendingOrderChaseInterval = oldInterval
		pendingOrderChaseTimeout = oldTimeout
		pendingOrderChaseJobs = oldJobs
	}()

	srv := newTestServer(t)
	var mu sync.Mutex
	cancelled := false
	var marketBodies []map[string]any
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/trade/orders-pending":
			mu.Lock()
			isCancelled := cancelled
			mu.Unlock()
			if isCancelled {
				_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","ordId":"100","clOrdId":"client-100","tdMode":"cross","side":"buy","posSide":"net","ordType":"limit","px":"64000","sz":"0.5","accFillSz":"0.1","state":"live","attachAlgoOrds":[{"attachAlgoClOrdId":"client-100A","tpTriggerRatio":"0.01","slTriggerRatio":"0.005"}],"cTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","tickSz":"0.1","ctVal":"1","lotSz":"1","minSz":"1"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT-SWAP","bidPx":"63999","askPx":"64001","last":"64000"}]}`))
		case "/api/v5/trade/amend-order":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/cancel-order":
			mu.Lock()
			cancelled = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"100","clOrdId":"client-100","sCode":"0","sMsg":""}]}`))
		case "/api/v5/trade/order":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			marketBodies = append(marketBodies, body)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"200","clOrdId":"market-200","sCode":"0","sMsg":""}]}`))
		default:
			t.Fatalf("unexpected OKX path %s", r.URL.Path)
		}
	}))
	defer okxServer.Close()
	cfg := srv.ConfigStore.Get()
	cfg.Trading.BaseURL = okxServer.URL
	cfg.Trading.TakeProfitPct = 4
	cfg.Trading.StopLossPct = 2
	srv.ConfigStore = config.NewStore("", cfg)
	srv.OKXHTTPClient = okxServer.Client()
	if _, err := srv.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
		ID:          "default",
		Active:      true,
		Credentials: okx.Credentials{APIKey: "key", SecretKey: "secret", Passphrase: "pass"},
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"api_id":"default","inst_id":"BTC-USDT-SWAP","ord_id":"100","cl_ord_id":"client-100"}`
	start := httptest.NewRequest(http.MethodPost, "/tvbot/pending-orders/chase", strings.NewReader(body))
	start.Header.Set("Content-Type", "application/json")
	start.SetBasicAuth("admin", "Admin123")
	startRR := httptest.NewRecorder()
	srv.ServeHTTP(startRR, start)
	if startRR.Code != http.StatusAccepted {
		t.Fatalf("start chase code=%d body=%s", startRR.Code, startRR.Body.String())
	}

	for i := 0; i < 100; i++ {
		mu.Lock()
		done := len(marketBodies) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	key := pendingOrderChaseKey(pendingOrderChaseRequest{APIID: "default", InstID: "BTC-USDT-SWAP", OrdID: "100", ClOrdID: "client-100"})
	for i := 0; i < 100 && pendingOrderChaseJobs.activeKey(key); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if pendingOrderChaseJobs.activeKey(key) {
		t.Fatal("chase job should stop after market fallback")
	}
	if len(marketBodies) != 1 {
		t.Fatalf("expected one market order, got %#v", marketBodies)
	}
	market := marketBodies[0]
	if market["instId"] != "BTC-USDT-SWAP" || market["tdMode"] != "cross" || market["side"] != "buy" || market["ordType"] != "market" || market["sz"] != "0.4" {
		t.Fatalf("bad market fallback order: %#v", market)
	}
	attach, ok := market["attachAlgoOrds"].([]any)
	if !ok || len(attach) != 1 {
		t.Fatalf("missing attach algo on market fallback: %#v", market)
	}
	first, ok := attach[0].(map[string]any)
	if !ok || first["tpTriggerRatio"] != "0.04" || first["slTriggerRatio"] != "0.02" || first["tpOrdPx"] != "-1" || first["slOrdPx"] != "-1" {
		t.Fatalf("bad market fallback attach algo: %#v", attach)
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

func TestTVBotPositionLimitCloseRatioRoundsToLotSize(t *testing.T) {
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
	var sawOrder bool
	okxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v5/account/positions":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","mgnMode":"cross","posId":"1","posSide":"net","pos":"2","availPos":"2","avgPx":"64000","markPx":"65000","upl":"500","uplRatio":"0.015","lever":"5","notionalUsd":"130000","margin":"26000","uTime":"1784880000000"}]}`))
		case "/api/v5/public/instruments":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","tickSz":"0.1","lotSz":"0.2","minSz":"0.2","ctVal":"0.01","state":"live"}]}`))
		case "/api/v5/market/ticker":
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instType":"SWAP","instId":"BTC-USDT-SWAP","bidPx":"99.9","askPx":"100.1","last":"100","ts":"1784880000000"}]}`))
		case "/api/v5/trade/order":
			sawOrder = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["side"] != "sell" || body["ordType"] != "limit" || body["px"] != "99.9" || body["sz"] != "0.4" || body["reduceOnly"] != true {
				t.Fatalf("unexpected ratio close order: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"ratio-1","clOrdId":"close-ratio","sCode":"0","sMsg":""}]}`))
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

	reqBody := []byte(`{"api_id":"default","inst_id":"BTC-USDT-SWAP","pos_side":"net","mode":"limit","ratio":0.25}`)
	req := httptest.NewRequest(http.MethodPost, "/tvbot/positions/close", bytes.NewReader(reqBody))
	req.SetBasicAuth("admin", "Admin123")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("ratio limit close status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !sawOrder {
		t.Fatal("expected OKX ratio limit order")
	}
	var resp positionCloseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Status != "running" || resp.Mode != "limit" || resp.Px != "99.9" || resp.Sz != "0.4" {
		t.Fatalf("bad ratio limit close response: %#v", resp)
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
	binanceCredentialStore, err := binance.NewCredentialStore(filepath.Join(dir, "binance-credentials.json"), binance.Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		ConfigStore:        config.NewStore("", cfg),
		Orders:             orderStore,
		Token:              security.NewTokenService("unit-test-secret"),
		Executor:           fakeExecutor{calls: make(chan trading.Signal, 2)},
		OKXCredentials:     credentialStore,
		BinanceCredentials: binanceCredentialStore,
		AdminToken:         "admin",
		AdminUser:          "admin",
		AdminPass:          "Admin123",
		Upgrade:            upgrade.NewManager(fakeUpgradeRunner{done: make(chan struct{}, 1)}),
		BuildInfo:          BuildInfo{CommitTime: "2026-07-24T03:00:00Z", CommitHash: "testhash", CommitBranch: "testbranch"},
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

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, list := range values {
		for _, value := range list {
			out.Add(key, value)
		}
	}
	return out
}

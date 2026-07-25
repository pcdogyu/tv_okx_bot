package server

import (
	"html"
	"net/http"
	"strings"
)

type BuildInfo struct {
	CommitTime   string
	CommitHash   string
	CommitBranch string
}

func (b BuildInfo) FooterText() string {
	return "Code by Yuhao@jiansutech.com - " +
		buildInfoValue(b.CommitTime) + " - " +
		buildInfoValue(b.CommitHash) + " - " +
		buildInfoValue(b.CommitBranch)
}

func buildInfoValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func writeHTML(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}

func renderTVBotHTML(info BuildInfo) string {
	return strings.Replace(tvbotHTML, "{{APP_FOOTER}}", html.EscapeString(info.FooterText()), 1)
}

func renderTVBotLoginHTML(message, next string, info BuildInfo) string {
	errorHTML := ""
	if strings.TrimSpace(message) != "" {
		errorHTML = `<p class="error">` + html.EscapeString(message) + `</p>`
	}
	page := strings.Replace(tvbotLoginHTML, "{{ERROR}}", errorHTML, 1)
	page = strings.Replace(page, "{{NEXT}}", html.EscapeString(sanitizeAdminNext(next)), 1)
	page = strings.Replace(page, "{{APP_FOOTER}}", html.EscapeString(info.FooterText()), 1)
	return page
}

const tvbotLoginHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TV OKX Bot 登录</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f4f7fb;
      --panel: #ffffff;
      --line: #d8dee9;
      --text: #172033;
      --muted: #647089;
      --blue: #1f6feb;
      --red: #c24135;
      --shadow: 0 18px 44px rgba(23, 32, 51, 0.12);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 28px;
      font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background:
        linear-gradient(135deg, rgba(31, 111, 235, 0.08), rgba(19, 138, 85, 0.08)),
        var(--bg);
    }
    main {
      width: min(420px, 100%);
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      margin-bottom: 16px;
      font-weight: 800;
      font-size: 18px;
    }
    .logo {
      width: 32px;
      height: 32px;
      border-radius: 6px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      color: white;
      background: #172033;
      font-weight: 800;
      letter-spacing: 0;
    }
    form {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
      box-shadow: var(--shadow);
      padding: 24px;
    }
    h1 {
      margin: 0 0 18px;
      font-size: 22px;
      line-height: 1.2;
      letter-spacing: 0;
    }
    label {
      display: grid;
      gap: 6px;
      margin: 14px 0;
      color: var(--muted);
      font-weight: 650;
    }
    input {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 11px 12px;
      color: var(--text);
      background: #fff;
      font: inherit;
      outline: none;
    }
    input:focus {
      border-color: var(--blue);
      box-shadow: 0 0 0 3px rgba(31, 111, 235, 0.14);
    }
    button {
      width: 100%;
      border: 1px solid var(--blue);
      border-radius: 6px;
      padding: 11px 12px;
      margin-top: 8px;
      color: white;
      background: var(--blue);
      font: inherit;
      font-weight: 750;
      cursor: pointer;
    }
    .error {
      margin: 0 0 12px;
      padding: 10px 12px;
      border: 1px solid rgba(194, 65, 53, 0.35);
      border-radius: 6px;
      color: var(--red);
      background: rgba(194, 65, 53, 0.08);
    }
    footer {
      margin-top: 16px;
      color: var(--muted);
      text-align: center;
      font-size: 12px;
    }
  </style>
</head>
<body>
  <main>
    <div class="brand"><span class="logo">TV</span><span>OKX Bot</span></div>
    <form method="post" action="/tvbot/login" autocomplete="on">
      <h1>管理员登录</h1>
      {{ERROR}}
      <input type="hidden" name="next" value="{{NEXT}}">
      <label>用户名<input name="username" autocomplete="username" required autofocus></label>
      <label>密码<input name="password" type="password" autocomplete="current-password" required></label>
      <button type="submit">登录</button>
    </form>
    <footer>{{APP_FOOTER}}</footer>
  </main>
</body>
</html>`

const tvbotHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TV OKX Bot</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f7fb;
      --panel: #ffffff;
      --line: #d8dee9;
      --text: #172033;
      --muted: #647089;
      --blue: #1f6feb;
      --green: #138a55;
      --red: #c24135;
      --amber: #a16207;
      --shadow: 0 8px 28px rgba(23, 32, 51, 0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background: var(--bg);
    }
    header {
      position: sticky;
      top: 0;
      z-index: 5;
      background: rgba(255, 255, 255, 0.96);
      border-bottom: 1px solid var(--line);
      box-shadow: 0 2px 12px rgba(23, 32, 51, 0.04);
    }
    .bar {
      width: 100%;
      margin: 0 auto;
      padding: 14px 18px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 18px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 10px;
      min-width: 180px;
      font-weight: 700;
      font-size: 17px;
    }
    .mark {
      width: 28px;
      height: 28px;
      border-radius: 6px;
      background: #172033;
      color: #fff;
      display: inline-grid;
      place-items: center;
      font-size: 12px;
      letter-spacing: 0;
    }
    nav {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    nav button, .btn {
      border: 1px solid var(--line);
      background: #fff;
      color: var(--text);
      border-radius: 6px;
      padding: 8px 11px;
      font: inherit;
      cursor: pointer;
      min-height: 36px;
    }
    nav button[aria-selected="true"], .btn.primary {
      background: var(--blue);
      color: #fff;
      border-color: var(--blue);
    }
    .btn.danger {
      color: var(--red);
      border-color: #e3aaa3;
    }
    .btn.success {
      color: #fff;
      background: var(--green);
      border-color: var(--green);
    }
    main {
      width: 100%;
      flex: 1;
      margin: 0 auto;
      padding: 18px;
    }
    .status {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 16px;
    }
    .metric, section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
    }
    .metric {
      padding: 13px 14px;
      min-height: 78px;
    }
    .metric .label {
      color: var(--muted);
      font-size: 12px;
    }
    .metric .value {
      margin-top: 7px;
      font-size: 20px;
      font-weight: 700;
      overflow-wrap: anywhere;
    }
    section {
      display: none;
      padding: 16px;
    }
    section.active { display: block; }
    .section-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      margin-bottom: 14px;
    }
    h1, h2, h3 {
      margin: 0;
      letter-spacing: 0;
    }
    h2 { font-size: 18px; }
    h3 { font-size: 14px; }
    .muted { color: var(--muted); }
    .grid {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 12px;
    }
    .grid.two { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    label {
      display: grid;
      gap: 6px;
      color: var(--muted);
      font-size: 12px;
    }
    input, select, textarea {
      width: 100%;
      min-height: 36px;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 8px 9px;
      color: var(--text);
      background: #fff;
      font: inherit;
    }
    textarea {
      min-height: 190px;
      resize: vertical;
      font-family: Consolas, "SFMono-Regular", monospace;
      font-size: 12px;
    }
    .check {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--text);
      padding-top: 23px;
      min-height: 36px;
    }
    .check input {
      width: 16px;
      min-height: 16px;
    }
    .actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      margin-top: 14px;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      table-layout: fixed;
      background: #fff;
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
    }
    th, td {
      border-bottom: 1px solid var(--line);
      padding: 9px;
      text-align: left;
      vertical-align: top;
      overflow-wrap: anywhere;
    }
    td.time {
      white-space: nowrap;
      overflow-wrap: normal;
    }
    th.order-okx,
    td.order-okx {
      width: 37.5%;
    }
    th {
      color: var(--muted);
      font-size: 12px;
      background: #f8fafc;
      font-weight: 600;
    }
    tr:last-child td { border-bottom: 0; }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 24px;
      border-radius: 999px;
      padding: 2px 8px;
      font-size: 12px;
      border: 1px solid var(--line);
      color: var(--muted);
      background: #fff;
    }
    .pill.ok { color: var(--green); border-color: #9fd8bd; }
    .pill.warn { color: var(--amber); border-color: #e9d08d; }
    .pill.bad { color: var(--red); border-color: #efb3ac; }
    .split {
      display: grid;
      grid-template-columns: 0.95fr 1.05fr;
      gap: 14px;
    }
    .api-key-layout {
      display: grid;
      grid-template-columns: 1.2fr 0.8fr;
      gap: 14px;
      align-items: start;
    }
    .api-key-form {
      display: grid;
      gap: 12px;
      align-content: start;
    }
    .api-key-inputs {
      align-items: start;
    }
    .api-key-inputs label {
      align-content: start;
    }
    .api-key-inputs input {
      min-height: 29px;
      height: 29px;
      padding-top: 5px;
      padding-bottom: 5px;
    }
    .table-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .position-actions {
      display: flex;
      gap: 6px;
      flex-wrap: nowrap;
      white-space: nowrap;
    }
    .btn.small {
      min-height: 28px;
      padding: 4px 8px;
      font-size: 12px;
    }
    .btn:disabled {
      opacity: 0.58;
      cursor: not-allowed;
    }
    .okx-cell {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 8px;
    }
    .okx-text {
      flex: 1;
      min-width: 0;
    }
    .analysis-controls {
      display: flex;
      gap: 10px;
      align-items: end;
      flex-wrap: wrap;
    }
    .analysis-controls label { min-width: 260px; }
    .symbol-controls {
      display: flex;
      gap: 10px;
      align-items: end;
      flex-wrap: wrap;
    }
    .symbol-controls label { min-width: 220px; }
    .menu-hidden-check {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--text);
      min-height: 28px;
    }
    .menu-hidden-check input {
      width: 16px;
      min-height: 16px;
    }
    .menu-sort-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .analysis-metrics {
      display: grid;
      grid-template-columns: repeat(5, minmax(0, 1fr));
      gap: 10px;
      margin: 12px 0;
    }
    .asset-metrics {
      grid-template-columns: repeat(6, minmax(0, 1fr));
    }
    .symbol-metrics {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
    .position-metrics {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
    .analysis-card {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px;
      background: #fff;
      min-height: 70px;
    }
    .analysis-card .label {
      color: var(--muted);
      font-size: 12px;
    }
    .analysis-card .value {
      margin-top: 5px;
      font-size: 18px;
      font-weight: 700;
      overflow-wrap: anywhere;
    }
    .signed-profit {
      color: var(--green);
      font-weight: 700;
    }
    .signed-loss {
      color: var(--red);
      font-weight: 700;
    }
    .chart-wrap {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #fff;
      padding: 10px;
      min-height: 460px;
    }
    #usdt-chart {
      width: 100%;
      height: 420px;
      display: block;
    }
    .chart-grid {
      stroke: #edf1f7;
      stroke-width: 1;
    }
    .chart-axis {
      stroke: #d8dee9;
      stroke-width: 1;
    }
    .chart-label {
      fill: #647089;
      font-size: 12px;
    }
    .symbol-table-wrap {
      overflow-x: auto;
    }
    .symbol-table {
      min-width: 980px;
    }
    pre {
      margin: 0;
      white-space: pre-wrap;
      overflow-wrap: anywhere;
      background: #111827;
      color: #e5e7eb;
      border-radius: 8px;
      padding: 12px;
      min-height: 140px;
      font-size: 12px;
    }
    .toast {
      position: fixed;
      right: 18px;
      bottom: 18px;
      max-width: 420px;
      background: #172033;
      color: #fff;
      border-radius: 8px;
      padding: 12px 14px;
      box-shadow: var(--shadow);
      display: none;
      z-index: 10;
    }
    .toast.show { display: block; }
    .build-footer {
      width: 100%;
      text-align: center;
      color: var(--muted);
      font-size: 12px;
      line-height: 1.4;
      padding: 8px 18px 18px;
      overflow-wrap: anywhere;
    }
    @media (max-width: 880px) {
      .bar { align-items: flex-start; flex-direction: column; }
      nav { justify-content: flex-start; }
      .status, .grid, .grid.two, .split, .api-key-layout, .analysis-metrics, .asset-metrics, .symbol-metrics, .position-metrics { grid-template-columns: 1fr; }
      main { padding: 12px; }
      section { padding: 12px; }
      #usdt-chart { height: 320px; }
    }
  </style>
</head>
<body>
  <header>
    <div class="bar">
      <div class="brand"><span class="mark">TV</span><span>OKX Bot</span></div>
      <nav aria-label="sections">
        <button type="button" data-tab="dashboard" aria-selected="true">总览</button>
        <button type="button" data-tab="positions">持仓</button>
        <button type="button" data-tab="analysis">订单分析</button>
        <button type="button" data-tab="symbols">币对配置</button>
        <button type="button" data-tab="config">订单配置</button>
        <button type="button" data-tab="apiKeys">API Key</button>
        <button type="button" data-tab="orderSettings">下单设置</button>
        <button type="button" data-tab="template">告警模板</button>
        <button type="button" data-tab="orders">订单</button>
        <button type="button" data-tab="menuSettings">菜单设置</button>
        <button type="button" data-tab="upgrade">升级</button>
      </nav>
    </div>
  </header>

  <main>
    <div class="status">
      <div class="metric"><div class="label">交易环境</div><div class="value" id="metric-env">-</div></div>
      <div class="metric"><div class="label">OKX API</div><div class="value" id="metric-api-keys">-</div></div>
      <div class="metric"><div class="label">下单金额</div><div class="value" id="metric-amount">-</div></div>
      <div class="metric"><div class="label">最近信号</div><div class="value" id="metric-orders">-</div></div>
    </div>

    <section id="dashboard" class="active">
      <div class="section-head">
        <h2>总览</h2>
        <div class="actions" style="margin-top:0">
          <button class="btn" type="button" id="refresh-all">刷新</button>
          <button class="btn success" type="button" id="check-okx">检查 OKX</button>
        </div>
      </div>
      <div class="split">
        <div>
          <h3>运行配置</h3>
          <table style="margin-top:10px">
            <tbody id="dashboard-config"></tbody>
          </table>
        </div>
        <div>
          <h3>OKX 检查</h3>
          <pre id="okx-output">-</pre>
        </div>
      </div>
    </section>

    <section id="positions">
      <div class="section-head">
        <h2>当前持仓</h2>
        <div class="analysis-controls">
          <label>交易 API<select id="position-api-id"></select></label>
          <button class="btn primary" type="button" id="refresh-positions">刷新持仓</button>
        </div>
      </div>
      <div class="analysis-metrics position-metrics">
        <div class="analysis-card"><div class="label">持仓数</div><div class="value" id="positions-count">-</div></div>
        <div class="analysis-card"><div class="label">未实现盈亏</div><div class="value" id="positions-upl">-</div></div>
        <div class="analysis-card"><div class="label">名义价值</div><div class="value" id="positions-notional">-</div></div>
        <div class="analysis-card"><div class="label">更新时间</div><div class="value" id="positions-updated">-</div></div>
      </div>
      <div class="symbol-table-wrap">
        <table class="symbol-table">
          <thead>
            <tr><th>币对</th><th>方向</th><th>持仓量</th><th>可用</th><th>均价</th><th>标记价</th><th>未实现盈亏</th><th>收益率</th><th>杠杆</th><th>保证金模式</th><th>保证金</th><th>强平价</th><th>操作</th></tr>
          </thead>
          <tbody id="position-rows"></tbody>
        </table>
      </div>
    </section>

    <section id="analysis">
      <div class="section-head">
        <h2>订单分析</h2>
        <div class="analysis-controls">
          <label>交易 API<select id="analysis-api-id"></select></label>
          <button class="btn primary" type="button" id="refresh-analysis">刷新分析</button>
        </div>
      </div>
      <div class="analysis-metrics asset-metrics">
        <div class="analysis-card"><div class="label">资产估值</div><div class="value" id="analysis-total-eq">-</div></div>
        <div class="analysis-card"><div class="label">USDT估值</div><div class="value" id="analysis-usdt-eq">-</div></div>
        <div class="analysis-card"><div class="label">可用权益</div><div class="value" id="analysis-avail-eq">-</div></div>
        <div class="analysis-card"><div class="label">调整权益</div><div class="value" id="analysis-adj-eq">-</div></div>
        <div class="analysis-card"><div class="label">资产数</div><div class="value" id="analysis-asset-count">-</div></div>
        <div class="analysis-card"><div class="label">资产更新时间</div><div class="value" id="analysis-balance-updated">-</div></div>
      </div>
      <table style="margin-bottom:14px">
        <thead>
          <tr><th>币种</th><th>权益</th><th>估值 USD</th><th>可用余额</th><th>现金余额</th><th>冻结</th></tr>
        </thead>
        <tbody id="analysis-balance-rows"></tbody>
      </table>
      <div class="chart-wrap">
        <div class="section-head" style="margin-bottom:8px">
          <h3>USDT估值 最近 3 天</h3>
          <span class="muted" id="analysis-updated">-</span>
        </div>
        <svg id="usdt-chart" role="img" aria-label="USDT valuation chart"></svg>
      </div>
      <div class="analysis-metrics">
        <div class="analysis-card"><div class="label">净盈亏</div><div class="value" id="analysis-net-pnl">-</div></div>
        <div class="analysis-card"><div class="label">胜率</div><div class="value" id="analysis-win-rate">-</div></div>
        <div class="analysis-card"><div class="label">盈利因子</div><div class="value" id="analysis-profit-factor">-</div></div>
        <div class="analysis-card"><div class="label">盈亏比</div><div class="value" id="analysis-payoff-ratio">-</div></div>
        <div class="analysis-card"><div class="label">成交数</div><div class="value" id="analysis-trades">-</div></div>
      </div>
      <table>
        <thead>
          <tr><th>币对</th><th>成交数</th><th>盈利数</th><th>亏损数</th><th>净盈亏</th><th>手续费</th><th>胜率</th><th>盈利因子</th><th>盈亏比</th></tr>
        </thead>
        <tbody id="analysis-rows"></tbody>
      </table>
    </section>

    <section id="apiKeys">
      <div class="section-head">
        <h2>API Key</h2>
        <div class="actions" style="margin-top:0">
          <button class="btn primary" type="button" id="save-api-keys">保存 API Key</button>
          <button class="btn success" type="button" id="test-api-keys">测试 API</button>
        </div>
      </div>
      <div class="api-key-layout">
        <div class="api-key-form">
          <div class="grid two">
            <label>交易 API<select id="key-selected"></select></label>
            <label>账户名称<input id="key-name" autocomplete="off" spellcheck="false"></label>
            <label>API ID<input id="key-id" autocomplete="off" spellcheck="false" placeholder="default"></label>
            <label class="check"><input id="key-active" type="checkbox">设为交易 API</label>
          </div>
          <div class="grid api-key-inputs">
            <label>OKX API Key<input id="key-api" autocomplete="off" spellcheck="false"></label>
            <label>OKX Secret Key<input id="key-secret" type="password" autocomplete="new-password" spellcheck="false"></label>
            <label>OKX Passphrase<input id="key-passphrase" type="password" autocomplete="new-password" spellcheck="false"></label>
          </div>
          <div class="actions">
            <button class="btn" type="button" id="add-api-key">新增 API</button>
            <button class="btn danger" type="button" id="delete-api-key">删除当前 API</button>
          </div>
        </div>
        <div>
          <h3>当前状态</h3>
          <table style="margin-top:10px">
            <tbody id="api-key-status"></tbody>
          </table>
          <h3 style="margin-top:14px">API 列表</h3>
          <table style="margin-top:10px">
            <thead><tr><th>API ID</th><th>名称</th><th>状态</th><th>API Key</th></tr></thead>
            <tbody id="api-key-accounts"></tbody>
          </table>
        </div>
      </div>
    </section>

    <section id="symbols">
      <div class="section-head">
        <h2>币对配置</h2>
        <div class="symbol-controls">
          <label>环境<select id="symbol-env"><option value="all">全部</option><option value="live">live</option><option value="demo">模拟</option></select></label>
          <label>搜索<input id="symbol-search" autocomplete="off" spellcheck="false" placeholder="BTC-USDT-SWAP"></label>
          <button class="btn primary" type="button" id="refresh-symbols">刷新币对</button>
        </div>
      </div>
      <div class="analysis-metrics symbol-metrics">
        <div class="analysis-card"><div class="label">live 币对</div><div class="value" id="symbol-live-count">-</div></div>
        <div class="analysis-card"><div class="label">模拟币对</div><div class="value" id="symbol-demo-count">-</div></div>
        <div class="analysis-card"><div class="label">本地已配置</div><div class="value" id="symbol-configured-count">-</div></div>
        <div class="analysis-card"><div class="label">当前显示</div><div class="value" id="symbol-visible-count">-</div></div>
      </div>
      <div class="muted" id="symbol-errors" style="margin:0 0 10px"></div>
      <div class="symbol-table-wrap">
        <table class="symbol-table">
          <thead>
            <tr><th>交易环境</th><th>币对</th><th>本地配置</th><th>OKX 状态</th><th>基础 / 计价</th><th>结算币</th><th>合约面值</th><th>最小下单</th><th>数量步长</th><th>杠杆</th></tr>
          </thead>
          <tbody id="symbol-rows"></tbody>
        </table>
      </div>
    </section>

    <section id="config">
      <div class="section-head">
        <h2>订单配置</h2>
        <button class="btn primary" type="button" id="save-config">保存订单配置</button>
      </div>
      <div class="grid">
        <label>监听地址<input id="cfg-addr" autocomplete="off"></label>
        <label>数据文件<input id="cfg-data-file" autocomplete="off"></label>
        <label>SQLite 数据库<input id="cfg-database-file" autocomplete="off"></label>
        <label>OKX Base URL<input id="cfg-base-url" autocomplete="off"></label>
        <label>交易环境<select id="cfg-env"><option value="demo">demo</option><option value="live">live</option></select></label>
        <label>保证金模式<select id="cfg-margin"><option value="isolated">isolated</option><option value="cross">cross</option></select></label>
        <label>持仓模式<select id="cfg-position"><option value="net">net</option><option value="long_short">long_short</option></select></label>
        <label>信号有效秒数<input id="cfg-ttl" type="number" min="1" step="1"></label>
        <label class="check"><input id="cfg-live" type="checkbox">允许实盘交易</label>
      </div>
    </section>

    <section id="orderSettings">
      <div class="section-head">
        <h2>下单设置</h2>
        <button class="btn primary" type="button" id="save-order-settings">保存下单设置</button>
      </div>
      <div class="grid">
        <label>USDT 下单金额<input id="order-amount" type="number" min="0" step="0.01"></label>
        <label>杠杆<input id="order-leverage" type="number" min="1" step="1"></label>
        <label>订单类型<select id="order-type"><option value="market">市价单</option><option value="limit">限价单</option></select></label>
        <label>风控模式<select id="order-risk-type"><option value="tp_sl">固定止盈止损</option><option value="trailing">移动止损</option><option value="none">不设置</option></select></label>
        <label>固定止盈 %<input id="order-tp" type="number" min="0" step="0.01"></label>
        <label>固定止损 %<input id="order-sl" type="number" min="0" step="0.01"></label>
        <label>移动止损 %<input id="order-trailing" type="number" min="0" step="0.01"></label>
        <label>多单限价倍率<input id="order-long-multiplier" type="number" min="0" step="0.000001"></label>
        <label>空单限价倍率<input id="order-short-multiplier" type="number" min="0" step="0.000001"></label>
      </div>
      <table style="margin-top:14px">
        <tbody id="order-settings-preview"></tbody>
      </table>
    </section>

    <section id="template">
      <div class="section-head">
        <h2>告警模板</h2>
        <button class="btn primary" type="button" id="make-template">生成模板</button>
      </div>
      <div class="split">
        <div class="grid two">
          <label>交易 API<select id="tpl-api-id"></select></label>
          <label>价格源<select id="tpl-price-source"><option value="close">close</option><option value="high">high</option><option value="low">low</option></select></label>
        </div>
        <div>
          <textarea id="template-output" readonly></textarea>
          <div class="actions"><button class="btn" type="button" id="copy-template">复制 JSON</button></div>
        </div>
      </div>
    </section>

    <section id="orders">
      <div class="section-head">
        <h2>订单 / 信号历史</h2>
        <button class="btn" type="button" id="refresh-orders">刷新历史</button>
      </div>
      <table>
        <thead>
          <tr><th>时间</th><th>状态</th><th>API</th><th>方向</th><th>币对</th><th>价格</th><th>金额</th><th class="order-okx">OKX / 返回</th></tr>
        </thead>
        <tbody id="order-rows"></tbody>
      </table>
    </section>

    <section id="menuSettings">
      <div class="section-head">
        <h2>菜单设置</h2>
        <button class="btn primary" type="button" id="save-menu-settings">保存菜单设置</button>
      </div>
      <table>
        <thead>
          <tr><th>菜单</th><th>是否隐藏</th><th>排序</th></tr>
        </thead>
        <tbody id="menu-settings-rows"></tbody>
      </table>
    </section>

    <section id="upgrade">
      <div class="section-head">
        <h2>升级</h2>
        <div class="actions" style="margin-top:0">
          <button class="btn" type="button" id="refresh-upgrade">刷新状态</button>
          <button class="btn primary" type="button" id="start-upgrade">开始升级</button>
        </div>
      </div>
      <pre id="upgrade-output">-</pre>
    </section>
  </main>
  <footer class="build-footer">{{APP_FOOTER}}</footer>
  <div class="toast" id="toast"></div>

  <script>
    const activeTabStorageKey = "tvbot.active_tab";
    const state = { config: null, apiKeys: null, selectedAPIID: "", apiKeyTest: null, apiKeyTestID: "", orders: [], retrying: {}, positionClosing: {}, analysis: null, analysisError: "", positions: null, positionsError: "", symbols: null, symbolsError: "", upgrade: null };
    const defaultMenuItems = [
      { tab: "dashboard", label: "总览" },
      { tab: "positions", label: "持仓" },
      { tab: "analysis", label: "订单分析" },
      { tab: "symbols", label: "币对配置" },
      { tab: "config", label: "订单配置" },
      { tab: "apiKeys", label: "API Key" },
      { tab: "orderSettings", label: "下单设置" },
      { tab: "template", label: "告警模板" },
      { tab: "orders", label: "订单" },
      { tab: "menuSettings", label: "菜单设置", locked: true },
      { tab: "upgrade", label: "升级" }
    ];
    const $ = (id) => document.getElementById(id);

    async function api(path, options) {
      const res = await fetch(path, Object.assign({ credentials: "same-origin" }, options || {}));
      const text = await res.text();
      let data = null;
      try { data = text ? JSON.parse(text) : null; } catch (_) { data = text; }
      if (!res.ok) {
        const msg = data && data.message ? data.message : res.status + " " + res.statusText;
        throw new Error(msg);
      }
      return data;
    }

    function toast(message) {
      const node = $("toast");
      node.textContent = message;
      node.classList.add("show");
      window.setTimeout(() => node.classList.remove("show"), 2600);
    }

    function asText(v) {
      if (v === null || v === undefined || v === "") return "-";
      return String(v);
    }

    function formatNumber(v) {
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      return n.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 6 });
    }

    function formatUSD(v) {
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      return n.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + " USD";
    }

    function formatAssetAmount(v) {
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      return n.toLocaleString("zh-CN", { minimumFractionDigits: 0, maximumFractionDigits: 8 });
    }

    function formatPct(v) {
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      return (n * 100).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + "%";
    }

    function formatFactor(row) {
      if (row && row.profit_factor_text) return row.profit_factor_text;
      return formatNumber(row ? row.profit_factor : null);
    }

    function riskText(v) {
      if (v === "tp_sl") return "固定止盈止损";
      if (v === "trailing") return "移动止损";
      if (v === "none") return "不设置";
      return asText(v);
    }

    function orderTypeText(v) {
      if (v === "limit") return "限价单";
      if (v === "market") return "市价单";
      return asText(v);
    }

    function shanghaiTime(v) {
      if (!v) return "-";
      const date = new Date(v);
      if (Number.isNaN(date.getTime())) return asText(v);
      const parts = new Intl.DateTimeFormat("zh-CN", {
        timeZone: "Asia/Shanghai",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false
      }).formatToParts(date).reduce((acc, part) => {
        if (part.type !== "literal") acc[part.type] = part.value;
        return acc;
      }, {});
      return parts.year + "-" + parts.month + "-" + parts.day + " " + parts.hour + ":" + parts.minute + ":" + parts.second;
    }

    function shanghaiTimeFromOKX(v) {
      if (!v) return "-";
      const raw = String(v).trim();
      if (/^\d+$/.test(raw)) {
        const ms = Number(raw);
        if (Number.isFinite(ms)) return shanghaiTime(new Date(ms).toISOString());
      }
      return shanghaiTime(raw);
    }

    function balanceAmount(v) {
      if (v === null || v === undefined || v === "") return "-";
      const formatted = formatNumber(v);
      return (formatted === "-" ? asText(v) : formatted) + " USDT";
    }

    function chartPointDate(point) {
      if (!point) return null;
      if (point.date instanceof Date) return point.date;
      if (point.date) {
        const date = new Date(point.date);
        if (!Number.isNaN(date.getTime())) return date;
      }
      if (point.ts !== null && point.ts !== undefined && point.ts !== "") {
        const ms = Number(point.ts);
        if (Number.isFinite(ms)) return new Date(ms);
      }
      if (!point.time) return null;
      const date = new Date(point.time);
      return Number.isNaN(date.getTime()) ? null : date;
    }

    function chartTimeLabel(date) {
      if (!date || Number.isNaN(date.getTime())) return "";
      const parts = new Intl.DateTimeFormat("zh-CN", {
        timeZone: "Asia/Shanghai",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false
      }).formatToParts(date).reduce((acc, part) => {
        if (part.type !== "literal") acc[part.type] = part.value;
        return acc;
      }, {});
      return parts.month + "-" + parts.day + " " + parts.hour + ":" + parts.minute;
    }

    function chartTickIndexes(length, maxTicks) {
      if (length <= 0) return [];
      if (length === 1) return [0];
      const count = Math.min(maxTicks, length);
      const indexes = [];
      for (let i = 0; i < count; i++) {
        indexes.push(Math.round(i * (length - 1) / (count - 1)));
      }
      return Array.from(new Set(indexes));
    }

    function chartAxisValue(v) {
      const formatted = formatNumber(v);
      return formatted === "-" ? asText(v) : formatted;
    }

    function usdtBalanceDetail(balance) {
      const details = balance && Array.isArray(balance.details) ? balance.details : [];
      return details.find((row) => String(row.ccy || "").toUpperCase() === "USDT") || null;
    }

    function usdtValuationPoints(balancePoints, pricePoints, balance) {
      const stored = (Array.isArray(balancePoints) ? balancePoints : []).map((point, index) => {
        return {
          index: index,
          value: Number(point.value !== undefined ? point.value : (point.eq_usd || point.eq)),
          date: chartPointDate(point)
        };
      }).filter((point) => Number.isFinite(point.value));
      if (stored.length) return stored;
      const usdt = usdtBalanceDetail(balance);
      const currentValue = Number(usdt && usdt.eq_usd);
      if (!Number.isFinite(currentValue)) return [];
      const priced = (Array.isArray(pricePoints) ? pricePoints : []).map((point, index) => {
        return { index: index, price: Number(point.close), date: chartPointDate(point) };
      }).filter((point) => Number.isFinite(point.price));
      if (!priced.length) {
        return [{ index: 0, value: currentValue, date: balance && balance.updated_at ? new Date(balance.updated_at) : null }];
      }
      const latest = priced[priced.length - 1];
      const multiplier = latest.price > 0 ? currentValue / latest.price : 0;
      return priced.map((point) => {
        return {
          index: point.index,
          value: multiplier > 0 ? point.price * multiplier : currentValue,
          date: point.date
        };
      });
    }

    function pill(text, tone) {
      return '<span class="pill ' + (tone || "") + '">' + escapeHTML(asText(text)) + '</span>';
    }

    function escapeHTML(v) {
      return String(v).replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    }

    function menuDefinition(tabID) {
      return defaultMenuItems.find((item) => item.tab === tabID) || null;
    }

    function normalizeMenuItems(items) {
      const seen = {};
      const normalized = [];
      (Array.isArray(items) ? items : []).forEach((item) => {
        const tab = item && item.tab ? String(item.tab) : "";
        const def = menuDefinition(tab);
        if (!def || seen[tab]) return;
        normalized.push({ tab: tab, hidden: def.locked ? false : !!item.hidden });
        seen[tab] = true;
      });
      defaultMenuItems.forEach((def) => {
        if (!seen[def.tab]) normalized.push({ tab: def.tab, hidden: false });
      });
      return normalized;
    }

    function currentMenuItems() {
      const ui = state.config && state.config.ui ? state.config.ui : {};
      return normalizeMenuItems(ui.menu_items);
    }

    function setCurrentMenuItems(items) {
      if (!state.config) state.config = {};
      if (!state.config.ui) state.config.ui = {};
      state.config.ui.menu_items = normalizeMenuItems(items);
    }

    function tabButton(tabID) {
      return Array.from(document.querySelectorAll("nav button")).find((button) => button.dataset.tab === tabID) || null;
    }

    function firstVisibleTab() {
      const button = Array.from(document.querySelectorAll("nav button")).find((item) => !item.hidden && $(item.dataset.tab));
      return button ? button.dataset.tab : "menuSettings";
    }

    function applyMenuSettings() {
      const nav = document.querySelector("nav");
      if (!nav) return;
      const known = {};
      currentMenuItems().forEach((item) => {
        const button = tabButton(item.tab);
        const def = menuDefinition(item.tab);
        if (!button || !def) return;
        button.textContent = def.label;
        button.hidden = !!item.hidden;
        nav.appendChild(button);
        known[item.tab] = true;
      });
      document.querySelectorAll("nav button").forEach((button) => {
        if (!known[button.dataset.tab]) button.hidden = true;
      });
      const activeButton = document.querySelector('nav button[aria-selected="true"]');
      if (!activeButton || activeButton.hidden) {
        activateTab(firstVisibleTab(), true);
      }
    }

    function renderMenuSettings() {
      const items = currentMenuItems();
      $("menu-settings-rows").innerHTML = items.map((item, index) => {
        const def = menuDefinition(item.tab) || { label: item.tab };
        const hiddenCell = def.locked
          ? '<span class="muted">固定显示</span>'
          : '<label class="menu-hidden-check"><input type="checkbox" data-menu-hidden="' + escapeHTML(item.tab) + '"' + (item.hidden ? " checked" : "") + '>隐藏</label>';
        return "<tr>" +
          "<td>" + escapeHTML(def.label) + "</td>" +
          "<td>" + hiddenCell + "</td>" +
          '<td><div class="menu-sort-actions">' +
            '<button class="btn small" type="button" data-menu-index="' + index + '" data-menu-move="-1"' + (index === 0 ? " disabled" : "") + ">上移</button>" +
            '<button class="btn small" type="button" data-menu-index="' + index + '" data-menu-move="1"' + (index === items.length - 1 ? " disabled" : "") + ">下移</button>" +
          "</div></td>" +
          "</tr>";
      }).join("");
    }

    function moveMenuItem(index, direction) {
      const items = currentMenuItems();
      const next = index + direction;
      if (index < 0 || next < 0 || index >= items.length || next >= items.length) return;
      const current = items[index];
      items[index] = items[next];
      items[next] = current;
      setCurrentMenuItems(items);
      renderMenuSettings();
      applyMenuSettings();
    }

    function activateTab(tabID, persist) {
      let target = tabID || "dashboard";
      let button = tabButton(target);
      let section = $(target);
      if (!button || button.hidden || !section) {
        target = firstVisibleTab();
        button = tabButton(target);
        section = $(target);
      }
      document.querySelectorAll("nav button").forEach((b) => b.setAttribute("aria-selected", "false"));
      document.querySelectorAll("main section").forEach((s) => s.classList.remove("active"));
      if (button && section) {
        button.setAttribute("aria-selected", "true");
        section.classList.add("active");
      }
      if (persist) {
        try { localStorage.setItem(activeTabStorageKey, target); } catch (_) {}
        if (window.history && location.hash !== "#" + target) {
          history.replaceState(null, "", "#" + target);
        }
      }
      if (target === "analysis" && !state.analysis) {
        loadAnalysis(false).catch((err) => toast(err.message));
      }
      if (target === "positions" && !state.positions) {
        loadPositions().catch((err) => toast(err.message));
      }
      if (target === "symbols" && !state.symbols) {
        loadSymbols(true).catch((err) => toast(err.message));
      }
    }

    function initialTab() {
      const fromHash = location.hash ? location.hash.slice(1) : "";
      const hashButton = tabButton(fromHash);
      if (fromHash && hashButton && !hashButton.hidden && $(fromHash)) return fromHash;
      try {
        const stored = localStorage.getItem(activeTabStorageKey);
        const storedButton = tabButton(stored);
        if (stored && storedButton && !storedButton.hidden && $(stored)) return stored;
      } catch (_) {}
      return firstVisibleTab();
    }

    async function loadAll() {
      await Promise.allSettled([loadConfig(), loadAPIKeys(), loadOrders(), loadUpgrade()]);
      await loadAnalysis(false);
      renderDashboard();
    }

    async function loadConfig() {
      state.config = await api("/tvbot/config");
      applyMenuSettings();
      renderConfig();
      renderOrderSettings();
      renderMenuSettings();
      updateMetrics();
    }

    async function loadAPIKeys() {
      state.apiKeys = await api("/tvbot/api-keys");
      renderAPIKeys();
      renderTemplateAPIs();
      renderAnalysisAPIs();
      renderPositionAPIs();
      renderOrders();
      updateMetrics();
    }

    async function loadOrders() {
      const data = await api("/tvbot/orders?limit=50");
      state.orders = data.orders || [];
      renderOrders();
      updateMetrics();
    }

    async function loadUpgrade() {
      state.upgrade = await api("/upgrade");
      renderUpgrade();
      updateMetrics();
    }

    async function loadAnalysis(refresh) {
      const qs = new URLSearchParams({ price_days: "3", pnl_days: "30" });
      const selected = $("analysis-api-id") ? $("analysis-api-id").value : "";
      if (selected) qs.set("api_id", selected);
      if (refresh) qs.set("refresh", "true");
      try {
        state.analysis = await api("/tvbot/analysis?" + qs.toString());
        state.analysisError = "";
      } catch (err) {
        state.analysis = null;
        state.analysisError = err.message;
      }
      renderAnalysis();
    }

    async function loadPositions() {
      const qs = new URLSearchParams({ inst_type: "SWAP" });
      const selected = $("position-api-id") ? $("position-api-id").value : "";
      if (selected) qs.set("api_id", selected);
      try {
        state.positions = await api("/tvbot/positions?" + qs.toString());
        state.positionsError = "";
      } catch (err) {
        state.positions = null;
        state.positionsError = err.message;
      }
      renderPositions();
    }

    async function loadSymbols(showLoading) {
      if (showLoading) {
        $("symbol-rows").innerHTML = '<tr><td colspan="10" class="muted">载入中...</td></tr>';
      }
      try {
        state.symbols = await api("/tvbot/symbols");
        state.symbolsError = "";
      } catch (err) {
        state.symbols = null;
        state.symbolsError = err.message;
        renderSymbols();
        throw err;
      }
      renderSymbols();
    }

    function updateMetrics() {
      $("metric-env").textContent = state.config && state.config.trading ? state.config.trading.env : "-";
      $("metric-api-keys").textContent = state.apiKeys && state.apiKeys.configured ? (state.apiKeys.active_id || "已配置") : "未配置";
      $("metric-amount").textContent = state.config && state.config.trading ? asText(state.config.trading.order_amount_usdt) + " USDT" : "-";
      $("metric-orders").textContent = state.orders ? state.orders.length : "-";
    }

    function renderDashboard() {
      if (!state.config) return;
      const t = state.config.trading || {};
      const rows = [
        ["服务地址", state.config.server ? state.config.server.addr : "-"],
        ["数据文件", state.config.data_file],
        ["SQLite 数据库", state.config.database_file],
        ["交易环境", t.env],
        ["实盘开关", t.allow_live_trading ? "enabled" : "disabled"],
        ["保证金模式", t.default_margin_mode],
        ["持仓模式", t.position_mode],
        ["信号有效秒数", t.signal_ttl_seconds],
        ["USDT 下单金额", t.order_amount_usdt],
        ["杠杆", t.leverage],
        ["订单类型", orderTypeText(t.order_type || "market")],
        ["风控模式", riskText(t.risk_type)],
        ["多单限价", "当前价格 x " + asText(t.long_limit_price_multiplier)],
        ["空单限价", "当前价格 x " + asText(t.short_limit_price_multiplier)]
      ];
      $("dashboard-config").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(asText(row[1])) + "</td></tr>").join("");
    }

    function renderConfig() {
      const cfg = state.config || {};
      const trading = cfg.trading || {};
      $("cfg-addr").value = cfg.server && cfg.server.addr ? cfg.server.addr : "";
      $("cfg-data-file").value = cfg.data_file || "";
      $("cfg-database-file").value = cfg.database_file || "";
      $("cfg-base-url").value = trading.base_url || "";
      $("cfg-env").value = trading.env || "demo";
      $("cfg-margin").value = trading.default_margin_mode || "isolated";
      $("cfg-position").value = trading.position_mode || "net";
      $("cfg-ttl").value = trading.signal_ttl_seconds || 120;
      $("cfg-live").checked = !!trading.allow_live_trading;
    }

    function renderSymbols() {
      const data = state.symbols || {};
      const catalog = data.okx || {};
      const live = catalog.live || {};
      const demo = catalog.demo || {};
      const configured = data.symbols || {};
      const configuredLookup = configuredSymbolMap();
      const rows = filteredSymbolRows();
      $("symbol-live-count").textContent = asText(live.count || (Array.isArray(live.instruments) ? live.instruments.length : 0));
      $("symbol-demo-count").textContent = asText(demo.count || (Array.isArray(demo.instruments) ? demo.instruments.length : 0));
      $("symbol-configured-count").textContent = asText(Object.keys(configured).length);
      $("symbol-visible-count").textContent = asText(rows.length);
      const errors = [];
      if (state.symbolsError) errors.push(state.symbolsError);
      if (live.error) errors.push("live: " + live.error);
      if (demo.error) errors.push("模拟: " + demo.error);
      $("symbol-errors").textContent = errors.join(" / ");
      $("symbol-rows").innerHTML = rows.map((row) => {
        const inst = row.instrument || {};
        const base = inst.baseCcy || baseFromInstID(inst.instId);
        const quote = inst.quoteCcy || quoteFromInstID(inst.instId);
        const isConfigured = configuredLookup[String(inst.instId || "").toUpperCase()] || configuredLookup[String(base || "").toUpperCase()];
        return "<tr>" +
          "<td>" + pill(row.label, row.env === "live" ? "ok" : "warn") + "</td>" +
          "<td>" + escapeHTML(asText(inst.instId)) + "</td>" +
          "<td>" + pill(isConfigured ? "已配置" : "未配置", isConfigured ? "ok" : "") + "</td>" +
          "<td>" + escapeHTML(asText(inst.state)) + "</td>" +
          "<td>" + escapeHTML(asText(base) + " / " + asText(quote)) + "</td>" +
          "<td>" + escapeHTML(asText(inst.settleCcy)) + "</td>" +
          "<td>" + escapeHTML(valueWithUnit(inst.ctVal, inst.ctValCcy)) + "</td>" +
          "<td>" + escapeHTML(asText(inst.minSz)) + "</td>" +
          "<td>" + escapeHTML(asText(inst.lotSz)) + "</td>" +
          "<td>" + escapeHTML(asText(inst.lever)) + "</td>" +
          "</tr>";
      }).join("") || '<tr><td colspan="10" class="muted">' + escapeHTML(state.symbolsError || "暂无币对数据") + '</td></tr>';
    }

    function filteredSymbolRows() {
      const data = state.symbols || {};
      const catalog = data.okx || {};
      const envFilter = $("symbol-env") ? $("symbol-env").value : "all";
      const keyword = $("symbol-search") ? $("symbol-search").value.trim().toLowerCase() : "";
      const groups = [
        { env: "live", label: "live", set: catalog.live || {} },
        { env: "demo", label: "模拟", set: catalog.demo || {} }
      ];
      const rows = [];
      groups.forEach((group) => {
        if (envFilter !== "all" && envFilter !== group.env) return;
        const instruments = Array.isArray(group.set.instruments) ? group.set.instruments : [];
        instruments.forEach((instrument) => {
          if (keyword) {
            const haystack = [
              instrument.instId,
              instrument.baseCcy,
              instrument.quoteCcy,
              instrument.settleCcy,
              instrument.instFamily,
              instrument.uly
            ].join(" ").toLowerCase();
            if (!haystack.includes(keyword)) return;
          }
          rows.push({ env: group.env, label: group.label, instrument: instrument });
        });
      });
      return rows;
    }

    function configuredSymbolMap() {
      const configured = state.symbols && state.symbols.symbols ? state.symbols.symbols : {};
      const out = {};
      Object.keys(configured).forEach((key) => {
        const item = configured[key] || {};
        out[String(key).toUpperCase()] = true;
        if (item.coinpair) out[String(item.coinpair).toUpperCase()] = true;
        if (item.inst_id) out[String(item.inst_id).toUpperCase()] = true;
      });
      return out;
    }

    function baseFromInstID(instID) {
      const parts = String(instID || "").split("-");
      return parts[0] || "";
    }

    function quoteFromInstID(instID) {
      const parts = String(instID || "").split("-");
      return parts[1] || "";
    }

    function valueWithUnit(value, unit) {
      const text = asText(value);
      if (text === "-") return text;
      return unit ? text + " " + unit : text;
    }

    function renderOrderSettings() {
      const trading = state.config && state.config.trading ? state.config.trading : {};
      $("order-amount").value = trading.order_amount_usdt || 100;
      $("order-leverage").value = trading.leverage || 5;
      $("order-type").value = trading.order_type || "market";
      $("order-risk-type").value = trading.risk_type || "tp_sl";
      $("order-tp").value = trading.take_profit_pct || 2;
      $("order-sl").value = trading.stop_loss_pct || 1;
      $("order-trailing").value = trading.trailing_pct || 1;
      $("order-long-multiplier").value = trading.long_limit_price_multiplier || 0.997;
      $("order-short-multiplier").value = trading.short_limit_price_multiplier || 1.003;
      renderOrderSettingsPreview();
    }

    function renderOrderSettingsPreview() {
      const orderType = $("order-type").value || "market";
      const rows = [
        ["订单类型", orderTypeText(orderType)],
        [orderType === "limit" ? "多单限价单价格" : "市价单估算价格", orderType === "limit" ? "TradingView 当前价格 x " + asText($("order-long-multiplier").value) : "TradingView 当前价格"],
        [orderType === "limit" ? "空单限价单价格" : "OKX 下单价格", orderType === "limit" ? "TradingView 当前价格 x " + asText($("order-short-multiplier").value) : "市价"],
        ["固定止盈止损", asText($("order-tp").value) + "% / " + asText($("order-sl").value) + "%"],
        ["移动止损", asText($("order-trailing").value) + "%"]
      ];
      $("order-settings-preview").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(row[1]) + "</td></tr>").join("");
    }

    function renderAPIKeys() {
      const status = state.apiKeys || {};
      const accounts = apiAccounts();
      const select = $("key-selected");
      const previous = state.selectedAPIID || select.value || status.active_id || "";
      select.innerHTML = accounts.map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>').join("");
      if (!accounts.length) {
        select.innerHTML = '<option value="default">default - 新 API</option>';
      }
      const selected = accounts.some((account) => account.id === previous) ? previous : (status.active_id || (accounts[0] && accounts[0].id) || "default");
      select.value = selected;
      state.selectedAPIID = selected;
      fillAPIForm(selected);
      renderAPIKeyStatus(selected);
      $("api-key-accounts").innerHTML = accounts.map((account) => {
        return "<tr>" +
          "<td>" + escapeHTML(account.id) + "</td>" +
          "<td>" + escapeHTML(account.name || "-") + "</td>" +
          "<td>" + pill(account.active ? "交易 API" : (account.configured ? "已配置" : "未配置"), account.active ? "ok" : (account.configured ? "warn" : "bad")) + "</td>" +
          "<td>" + escapeHTML(account.api_key_masked || "-") + "</td>" +
          "</tr>";
      }).join("") || '<tr><td colspan="4" class="muted">-</td></tr>';
    }

    function renderAPIKeyStatus(selected) {
      const status = state.apiKeys || {};
      const rows = [
        ["配置状态", status.configured ? "已配置" : "未配置"],
        ["交易 API", status.active_id || "-"],
        ["API Key", status.api_key_masked || "-"],
        ["Secret Key", status.secret_key_set ? "已保存" : "未保存"],
        ["Passphrase", status.passphrase_set ? "已保存" : "未保存"],
        ["来源", status.source || "-"],
        ["更新时间", status.updated_at || "-"]
      ];
      const test = state.apiKeyTest;
      const testID = state.apiKeyTestID || (test && test.api_id) || "";
      if (test && (!selected || !testID || selected === testID || test.api_id === "input")) {
        const balance = test.usdt_balance || {};
        rows.push(["测试 API", test.api_id || testID || "-"]);
        if (test.usdt_balance_found && balance) {
          rows.push(["USDT 总权益", balanceAmount(balance.eq)]);
          rows.push(["USDT 可用", balanceAmount(balance.avail_eq || balance.avail_bal)]);
          rows.push(["USDT 冻结", balanceAmount(balance.frozen_bal)]);
          rows.push(["余额更新时间", shanghaiTimeFromOKX(balance.u_time)]);
        } else {
          rows.push(["USDT 余额", "OKX 未返回 USDT 明细"]);
        }
      }
      $("api-key-status").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(row[1]) + "</td></tr>").join("");
    }

    function apiAccounts() {
      return state.apiKeys && Array.isArray(state.apiKeys.credentials) ? state.apiKeys.credentials : [];
    }

    function selectedAPIAccount(id) {
      return apiAccounts().find((account) => account.id === id) || null;
    }

    function apiDisplayName(id) {
      const apiID = String(id || "").trim();
      if (!apiID) return "-";
      const account = selectedAPIAccount(apiID);
      return account && account.name ? account.name : apiID;
    }

    function fillAPIForm(id) {
      const account = selectedAPIAccount(id);
      $("key-id").value = account ? account.id : (id || "default");
      $("key-name").value = account ? (account.name || "") : "";
      $("key-active").checked = account ? !!account.active : true;
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
    }

    function renderTemplateAPIs() {
      const options = apiAccounts().map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>');
      $("tpl-api-id").innerHTML = '<option value="">默认交易 API</option>' + options.join("");
      if (state.apiKeys && state.apiKeys.active_id) {
        $("tpl-api-id").value = state.apiKeys.active_id;
      }
    }

    function renderAnalysisAPIs() {
      const select = $("analysis-api-id");
      const current = select.value;
      const options = apiAccounts().map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>');
      select.innerHTML = '<option value="">默认交易 API</option>' + options.join("");
      select.value = current || (state.apiKeys && state.apiKeys.active_id ? state.apiKeys.active_id : "");
    }

    function renderPositionAPIs() {
      const select = $("position-api-id");
      const current = select.value;
      const options = apiAccounts().map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>');
      select.innerHTML = '<option value="">默认交易 API</option>' + options.join("");
      select.value = current || (state.apiKeys && state.apiKeys.active_id ? state.apiKeys.active_id : "");
    }

    function positionSideText(posSide, pos) {
      const side = String(posSide || "").toLowerCase();
      if (side === "long") return "多";
      if (side === "short") return "空";
      if (side === "net") {
        const value = Number(pos);
        if (Number.isFinite(value) && value < 0) return "净空";
        if (Number.isFinite(value) && value > 0) return "净多";
        return "净持仓";
      }
      return asText(posSide);
    }

    function positionPercent(v) {
      if (v === null || v === undefined || v === "") return "-";
      const formatted = formatPct(v);
      return formatted === "-" ? asText(v) : formatted;
    }

    function positionSum(rows, field) {
      return rows.reduce((sum, row) => {
        const value = Number(row[field]);
        return Number.isFinite(value) ? sum + value : sum;
      }, 0);
    }

    function signedToneClass(v) {
      const value = Number(v);
      if (!Number.isFinite(value) || value === 0) return "";
      return value > 0 ? "signed-profit" : "signed-loss";
    }

    function signedCell(v, formatted) {
      const tone = signedToneClass(v);
      return '<td' + (tone ? ' class="' + tone + '"' : "") + ">" + escapeHTML(formatted) + "</td>";
    }

    function positionCloseRowKey(row) {
      return String(row.instId || "").toUpperCase() + "|" + String(row.posSide || "net").toLowerCase();
    }

    function positionActionCell(row) {
      const key = positionCloseRowKey(row);
      const closing = !!state.positionClosing[key];
      const instID = escapeHTML(asText(row.instId));
      const posSide = escapeHTML(String(row.posSide || ""));
      return '<td><div class="position-actions">' +
        '<button class="btn small danger" type="button" data-position-close="market" data-inst-id="' + instID + '" data-pos-side="' + posSide + '"' + (closing ? " disabled" : "") + '>市价平仓</button>' +
        '<button class="btn small" type="button" data-position-close="limit" data-inst-id="' + instID + '" data-pos-side="' + posSide + '"' + (closing ? " disabled" : "") + '>限价平仓</button>' +
        '</div></td>';
    }

    function renderPositions() {
      const rows = state.positions && Array.isArray(state.positions.positions) ? state.positions.positions : [];
      const totalUpl = positionSum(rows, "upl");
      $("positions-count").textContent = state.positions ? asText(state.positions.count || rows.length) : "-";
      $("positions-upl").textContent = state.positions ? formatNumber(totalUpl) + " USDT" : "-";
      $("positions-upl").className = ["value", state.positions ? signedToneClass(totalUpl) : ""].filter(Boolean).join(" ");
      $("positions-notional").textContent = state.positions ? formatUSD(positionSum(rows, "notionalUsd")) : "-";
      $("positions-updated").textContent = state.positions && state.positions.refreshed_at ? shanghaiTime(state.positions.refreshed_at) : "-";
      if (!state.positions) {
        $("position-rows").innerHTML = '<tr><td colspan="13" class="muted">' + escapeHTML(state.positionsError || "-") + '</td></tr>';
        return;
      }
      $("position-rows").innerHTML = rows.map((row) => {
        return "<tr>" +
          "<td>" + escapeHTML(asText(row.instId)) + "</td>" +
          "<td>" + escapeHTML(positionSideText(row.posSide, row.pos)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.pos)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.availPos)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.avgPx)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.markPx)) + "</td>" +
          signedCell(row.upl, formatNumber(row.upl)) +
          signedCell(row.uplRatio, positionPercent(row.uplRatio)) +
          "<td>" + escapeHTML(asText(row.lever)) + "</td>" +
          "<td>" + escapeHTML(asText(row.mgnMode)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.margin)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.liqPx)) + "</td>" +
          positionActionCell(row) +
          "</tr>";
      }).join("") || '<tr><td colspan="13" class="muted">暂无当前持仓</td></tr>';
    }

    function renderAnalysis() {
      if (!state.analysis) {
        $("analysis-updated").textContent = state.analysisError || "-";
        renderAnalysisBalance(null);
        $("analysis-net-pnl").textContent = "-";
        $("analysis-win-rate").textContent = "-";
        $("analysis-profit-factor").textContent = "-";
        $("analysis-payoff-ratio").textContent = "-";
        $("analysis-trades").textContent = "-";
        $("analysis-rows").innerHTML = '<tr><td colspan="9" class="muted">' + escapeHTML(state.analysisError || "-") + '</td></tr>';
        drawUSDTChart([]);
        return;
      }
      const summary = state.analysis.summary || {};
      renderAnalysisBalance(state.analysis.balance || null);
      $("analysis-updated").textContent = "更新时间 " + shanghaiTime(state.analysis.refreshed_at) + " / API " + asText(state.analysis.api_id);
      $("analysis-net-pnl").textContent = formatNumber(summary.net_pnl) + " USDT";
      $("analysis-win-rate").textContent = formatPct(summary.win_rate);
      $("analysis-profit-factor").textContent = formatFactor(summary);
      $("analysis-payoff-ratio").textContent = formatNumber(summary.payoff_ratio);
      $("analysis-trades").textContent = asText(summary.trade_count);
      const rows = (state.analysis.symbols || []).map((row) => {
        return "<tr>" +
          "<td>" + escapeHTML(asText(row.inst_id)) + "</td>" +
          "<td>" + escapeHTML(asText(row.trade_count)) + "</td>" +
          "<td>" + escapeHTML(asText(row.wins)) + "</td>" +
          "<td>" + escapeHTML(asText(row.losses)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.net_pnl)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.fees)) + "</td>" +
          "<td>" + escapeHTML(formatPct(row.win_rate)) + "</td>" +
          "<td>" + escapeHTML(formatFactor(row)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.payoff_ratio)) + "</td>" +
          "</tr>";
      });
      $("analysis-rows").innerHTML = rows.join("") || '<tr><td colspan="9" class="muted">暂无 OKX 成交历史</td></tr>';
      drawUSDTChart(usdtValuationPoints(state.analysis.balance_points || [], state.analysis.price_points || [], state.analysis.balance || null));
    }

    function renderAnalysisBalance(balance) {
      const details = balance && Array.isArray(balance.details) ? balance.details : [];
      $("analysis-total-eq").textContent = balance ? formatUSD(balance.total_eq) : "-";
      const usdt = usdtBalanceDetail(balance);
      $("analysis-usdt-eq").textContent = usdt ? formatUSD(usdt.eq_usd || usdt.eq) : "-";
      $("analysis-avail-eq").textContent = balance ? formatUSD(balance.avail_eq) : "-";
      $("analysis-adj-eq").textContent = balance ? formatUSD(balance.adj_eq) : "-";
      $("analysis-asset-count").textContent = balance ? String(details.length) : "-";
      $("analysis-balance-updated").textContent = balance && balance.updated_at ? shanghaiTime(balance.updated_at) : "-";
      const rows = details.map((row) => {
        return "<tr>" +
          "<td>" + escapeHTML(asText(row.ccy)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.eq)) + "</td>" +
          "<td>" + escapeHTML(formatUSD(row.eq_usd)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.avail_bal || row.avail_eq)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.cash_bal)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.frozen_bal)) + "</td>" +
          "</tr>";
      });
      $("analysis-balance-rows").innerHTML = rows.join("") || '<tr><td colspan="6" class="muted">暂无 OKX 资产余额</td></tr>';
    }

    function drawUSDTChart(points) {
      const svg = $("usdt-chart");
      const rect = svg.getBoundingClientRect();
      const parentWidth = svg.parentElement ? svg.parentElement.clientWidth : 0;
      const width = Math.max(900, Math.floor(rect.width || parentWidth || (window.innerWidth - 72) || 900));
      const height = Math.max(320, Math.floor(rect.height || svg.clientHeight || 420));
      const pad = { left: 64, right: 24, top: 18, bottom: 58 };
      const plotWidth = width - pad.left - pad.right;
      const plotHeight = height - pad.top - pad.bottom;
      const plotBottom = height - pad.bottom;
      svg.setAttribute("viewBox", "0 0 " + width + " " + height);
      svg.innerHTML = "";
      if (!points.length) {
        svg.innerHTML = '<text x="' + (width / 2) + '" y="' + (height / 2) + '" text-anchor="middle" fill="#647089">暂无 USDT估值数据</text>';
        return;
      }
      const chartPoints = points.map((point, index) => {
        return { point: point, index: index, value: Number(point.value), date: chartPointDate(point) };
      }).filter((point) => Number.isFinite(point.value));
      if (!chartPoints.length) {
        svg.innerHTML = '<text x="' + (width / 2) + '" y="' + (height / 2) + '" text-anchor="middle" fill="#647089">暂无 USDT估值数据</text>';
        return;
      }
      const values = chartPoints.map((point) => point.value);
      const min = Math.min.apply(null, values);
      const max = Math.max.apply(null, values);
      const span = max - min || 0.0001;
      const timed = chartPoints.filter((point) => point.date);
      const timeValues = timed.map((point) => point.date.getTime());
      const minTime = timeValues.length ? Math.min.apply(null, timeValues) : 0;
      const maxTime = timeValues.length ? Math.max.apply(null, timeValues) : 0;
      const timeSpan = maxTime - minTime;
      const x = (point) => {
        if (timeSpan > 0 && point.date) return pad.left + (point.date.getTime() - minTime) * plotWidth / timeSpan;
        if (chartPoints.length === 1) return pad.left + plotWidth / 2;
        return pad.left + point.index * plotWidth / Math.max(1, points.length - 1);
      };
      const y = (v) => pad.top + (max - v) * plotHeight / span;
      const path = chartPoints.map((point, i) => (i === 0 ? "M" : "L") + x(point).toFixed(2) + " " + y(point.value).toFixed(2)).join(" ");
      let grid = "";
      const yTickCount = 5;
      for (let i = 0; i < yTickCount; i++) {
        const ratio = yTickCount === 1 ? 0 : i / (yTickCount - 1);
        const lineY = pad.top + ratio * plotHeight;
        const labelValue = max - ratio * span;
        grid += '<line class="chart-grid" x1="' + pad.left + '" y1="' + lineY.toFixed(2) + '" x2="' + (width - pad.right) + '" y2="' + lineY.toFixed(2) + '"/>';
        grid += '<text class="chart-label" x="8" y="' + (lineY + 4).toFixed(2) + '">' + chartAxisValue(labelValue) + '</text>';
      }
      const xTicks = chartTickIndexes(chartPoints.length, 6);
      xTicks.forEach((tickIndex) => {
        const point = chartPoints[tickIndex];
        const lineX = x(point);
        const anchor = tickIndex === 0 ? "start" : (tickIndex === chartPoints.length - 1 ? "end" : "middle");
        grid += '<line class="chart-grid" x1="' + lineX.toFixed(2) + '" y1="' + pad.top + '" x2="' + lineX.toFixed(2) + '" y2="' + plotBottom + '"/>';
        grid += '<text class="chart-label" x="' + lineX.toFixed(2) + '" y="' + (height - 22) + '" text-anchor="' + anchor + '">' + escapeHTML(chartTimeLabel(point.date)) + '</text>';
      });
      const last = chartPoints[chartPoints.length - 1];
      svg.innerHTML =
        grid +
        '<line class="chart-axis" x1="' + pad.left + '" y1="' + pad.top + '" x2="' + pad.left + '" y2="' + plotBottom + '"/>' +
        '<line class="chart-axis" x1="' + pad.left + '" y1="' + plotBottom + '" x2="' + (width - pad.right) + '" y2="' + plotBottom + '"/>' +
        '<path d="' + path + '" fill="none" stroke="#1f6feb" stroke-width="2.4"/>' +
        '<circle cx="' + x(last).toFixed(2) + '" cy="' + y(last.value).toFixed(2) + '" r="4" fill="#1f6feb"/>';
    }

    function renderOrders() {
      const rows = (state.orders || []).map((order) => {
        const okxResult = order.result && (order.result.ord_id || order.result.okx_code) ? [order.result.ord_id, order.result.okx_code].filter(Boolean).join(" / ") : "";
        const errorText = [order.error_code, order.error].filter(Boolean).join(": ");
        const okx = okxResult || errorText || "-";
        const apiID = order.api_id || (order.result && order.result.api_id);
        const tone = order.status === "submitted" ? "ok" : (order.status === "failed" || order.status === "rejected" ? "bad" : "warn");
        const canRetry = order.status === "failed" && order.signal_id;
        const retrying = canRetry && state.retrying[order.signal_id];
        const retryButton = canRetry ? '<button class="btn small" type="button" data-retry-id="' + escapeHTML(order.signal_id) + '"' + (retrying ? " disabled" : "") + ">" + (retrying ? "重试中" : "重试") + "</button>" : "";
        return "<tr>" +
          '<td class="time">' + escapeHTML(shanghaiTime(order.accepted_at)) + "</td>" +
          "<td>" + pill(order.status, tone) + "</td>" +
          "<td>" + escapeHTML(apiDisplayName(apiID)) + "</td>" +
          "<td>" + escapeHTML(asText(order.action)) + "</td>" +
          "<td>" + escapeHTML(asText(order.coinpair)) + "</td>" +
          "<td>" + escapeHTML(asText(order.price)) + "</td>" +
          "<td>" + escapeHTML(asText(order.amount)) + "</td>" +
          '<td class="order-okx"><div class="okx-cell"><span class="okx-text">' + escapeHTML(okx) + "</span>" + retryButton + "</div></td>" +
          "</tr>";
      });
      $("order-rows").innerHTML = rows.join("") || '<tr><td colspan="8" class="muted">-</td></tr>';
    }

    function renderUpgrade() {
      $("upgrade-output").textContent = JSON.stringify(state.upgrade || {}, null, 2);
    }

    async function saveConfig() {
      const patch = {
        server: { addr: $("cfg-addr").value.trim() },
        data_file: $("cfg-data-file").value.trim(),
        database_file: $("cfg-database-file").value.trim(),
        trading: {
          env: $("cfg-env").value,
          allow_live_trading: $("cfg-live").checked,
          base_url: $("cfg-base-url").value.trim(),
          default_margin_mode: $("cfg-margin").value,
          position_mode: $("cfg-position").value,
          signal_ttl_seconds: Number($("cfg-ttl").value)
        }
      };
      state.config = await api("/tvbot/config", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
      applyMenuSettings();
      renderConfig();
      renderDashboard();
      renderMenuSettings();
      updateMetrics();
      toast("订单配置已保存");
    }

    async function saveOrderSettings() {
      const patch = {
        trading: {
          order_amount_usdt: Number($("order-amount").value),
          leverage: Number($("order-leverage").value),
          order_type: $("order-type").value,
          risk_type: $("order-risk-type").value,
          take_profit_pct: Number($("order-tp").value),
          stop_loss_pct: Number($("order-sl").value),
          trailing_pct: Number($("order-trailing").value),
          long_limit_price_multiplier: Number($("order-long-multiplier").value),
          short_limit_price_multiplier: Number($("order-short-multiplier").value)
        }
      };
      state.config = await api("/tvbot/config", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
      applyMenuSettings();
      renderOrderSettings();
      renderDashboard();
      renderMenuSettings();
      updateMetrics();
      toast("下单设置已保存");
    }

    async function saveMenuSettings() {
      const patch = { ui: { menu_items: currentMenuItems() } };
      state.config = await api("/tvbot/config", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
      applyMenuSettings();
      renderMenuSettings();
      toast("菜单设置已保存");
    }

    async function saveAPIKeys() {
      const body = {
        id: $("key-id").value.trim(),
        name: $("key-name").value.trim(),
        api_key: $("key-api").value.trim(),
        secret_key: $("key-secret").value.trim(),
        passphrase: $("key-passphrase").value.trim(),
        active: $("key-active").checked
      };
      state.apiKeys = await api("/tvbot/api-keys", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      state.selectedAPIID = body.id || state.apiKeys.active_id || "default";
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
      renderAPIKeys();
      renderTemplateAPIs();
      renderAnalysisAPIs();
      renderPositionAPIs();
      renderOrders();
      updateMetrics();
      toast("API Key 已保存");
    }

    async function testAPIKeys() {
      const body = {
        id: $("key-id").value.trim(),
        api_key: $("key-api").value.trim(),
        secret_key: $("key-secret").value.trim(),
        passphrase: $("key-passphrase").value.trim()
      };
      $("okx-output").textContent = "checking...";
      const result = await api("/tvbot/api-keys/test", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      state.apiKeyTest = result;
      state.apiKeyTestID = result.api_id || body.id || $("key-selected").value || "";
      $("okx-output").textContent = JSON.stringify(result, null, 2);
      renderAPIKeyStatus(body.id || state.selectedAPIID || result.api_id || "");
      toast("API 可用");
    }

    async function deleteAPIKey() {
      const id = $("key-id").value.trim() || $("key-selected").value;
      if (!id) return;
      state.apiKeys = await api("/tvbot/api-keys?id=" + encodeURIComponent(id), { method: "DELETE" });
      state.selectedAPIID = state.apiKeys.active_id || "";
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      renderAPIKeys();
      renderTemplateAPIs();
      renderAnalysisAPIs();
      renderPositionAPIs();
      renderOrders();
      updateMetrics();
      toast("API Key 已删除");
    }

    async function makeTemplate() {
      const req = {
        api_id: $("tpl-api-id").value,
        price_source: $("tpl-price-source").value
      };
      const resp = await api("/tvbot/templates", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(req) });
      $("template-output").value = resp.json || "";
      toast("模板已生成");
    }

    async function retryOrder(signalID) {
      if (!signalID || state.retrying[signalID]) return;
      state.retrying[signalID] = true;
      renderOrders();
      try {
        const result = await api("/tvbot/orders/" + encodeURIComponent(signalID) + "/retry", { method: "POST" });
        toast("重试已触发 " + asText(result.signal_id));
        await loadOrders();
        window.setTimeout(() => loadOrders().catch((err) => toast(err.message)), 1600);
      } finally {
        delete state.retrying[signalID];
        renderOrders();
      }
    }

    async function closePosition(button) {
      const mode = button.dataset.positionClose;
      const instID = button.dataset.instId || "";
      const posSide = button.dataset.posSide || "";
      const key = instID.toUpperCase() + "|" + (posSide || "net").toLowerCase();
      if (!instID || !mode || state.positionClosing[key]) return;
      state.positionClosing[key] = true;
      renderPositions();
      try {
        const result = await api("/tvbot/positions/close", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            api_id: $("position-api-id").value,
            inst_id: instID,
            pos_side: posSide,
            mode
          })
        });
        toast(mode === "market" ? "市价平仓已提交" : "限价平仓已启动 " + asText(result.px));
        await loadPositions();
        window.setTimeout(() => loadPositions().catch((err) => toast(err.message)), mode === "market" ? 1600 : 5200);
      } finally {
        delete state.positionClosing[key];
        renderPositions();
      }
    }

    async function checkOKX() {
      $("okx-output").textContent = "checking...";
      try {
        const body = state.apiKeys && state.apiKeys.active_id ? { api_id: state.apiKeys.active_id } : {};
        const result = await api("/tvbot/check-okx", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        $("okx-output").textContent = JSON.stringify(result, null, 2);
      } catch (err) {
        $("okx-output").textContent = err.message;
      }
    }

    async function startUpgrade() {
      $("upgrade-output").textContent = "starting...";
      state.upgrade = await api("/upgrade", { method: "POST" });
      renderUpgrade();
      updateMetrics();
      toast("升级已开始");
      pollUpgrade();
    }

    async function pollUpgrade() {
      for (let i = 0; i < 80; i++) {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        try {
          await loadUpgrade();
          if (state.upgrade && state.upgrade.status && state.upgrade.status !== "running") break;
        } catch (_) {}
      }
    }

    document.querySelectorAll("nav button").forEach((button) => {
      button.addEventListener("click", () => {
        activateTab(button.dataset.tab, true);
      });
    });

    $("refresh-all").addEventListener("click", () => loadAll().then(() => toast("已刷新")).catch((err) => toast(err.message)));
    $("check-okx").addEventListener("click", () => checkOKX());
    $("save-config").addEventListener("click", () => saveConfig().catch((err) => toast(err.message)));
    $("refresh-symbols").addEventListener("click", () => loadSymbols(true).then(() => toast("币对已刷新")).catch((err) => toast(err.message)));
    $("symbol-search").addEventListener("input", () => renderSymbols());
    $("symbol-env").addEventListener("change", () => renderSymbols());
    $("save-order-settings").addEventListener("click", () => saveOrderSettings().catch((err) => toast(err.message)));
    ["order-amount", "order-leverage", "order-type", "order-risk-type", "order-tp", "order-sl", "order-trailing", "order-long-multiplier", "order-short-multiplier"].forEach((id) => {
      $(id).addEventListener("input", () => renderOrderSettingsPreview());
      $(id).addEventListener("change", () => renderOrderSettingsPreview());
    });
    $("save-menu-settings").addEventListener("click", () => saveMenuSettings().catch((err) => toast(err.message)));
    $("menu-settings-rows").addEventListener("change", (event) => {
      const input = event.target.closest("input[data-menu-hidden]");
      if (!input) return;
      const items = currentMenuItems();
      const item = items.find((entry) => entry.tab === input.dataset.menuHidden);
      if (!item) return;
      item.hidden = input.checked;
      setCurrentMenuItems(items);
      renderMenuSettings();
      applyMenuSettings();
    });
    $("menu-settings-rows").addEventListener("click", (event) => {
      const button = event.target.closest("button[data-menu-move]");
      if (!button) return;
      moveMenuItem(Number(button.dataset.menuIndex), Number(button.dataset.menuMove));
    });
    $("save-api-keys").addEventListener("click", () => saveAPIKeys().catch((err) => toast(err.message)));
    $("test-api-keys").addEventListener("click", () => testAPIKeys().catch((err) => {
      $("okx-output").textContent = err.message;
      toast(err.message);
    }));
    $("delete-api-key").addEventListener("click", () => deleteAPIKey().catch((err) => toast(err.message)));
    $("add-api-key").addEventListener("click", () => {
      state.selectedAPIID = "";
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      $("key-selected").value = "";
      $("key-id").value = "";
      $("key-name").value = "";
      $("key-active").checked = !state.apiKeys || !state.apiKeys.configured;
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
      renderAPIKeyStatus("");
      $("key-id").focus();
    });
    $("key-selected").addEventListener("change", () => {
      state.selectedAPIID = $("key-selected").value;
      fillAPIForm(state.selectedAPIID);
      renderAPIKeyStatus(state.selectedAPIID);
    });
    $("analysis-api-id").addEventListener("change", () => loadAnalysis(false).catch((err) => toast(err.message)));
    $("refresh-analysis").addEventListener("click", () => loadAnalysis(true).then(() => toast("分析已刷新")).catch((err) => toast(err.message)));
    $("position-api-id").addEventListener("change", () => loadPositions().catch((err) => toast(err.message)));
    $("refresh-positions").addEventListener("click", () => loadPositions().then(() => toast("持仓已刷新")).catch((err) => toast(err.message)));
    $("make-template").addEventListener("click", () => makeTemplate().catch((err) => toast(err.message)));
    $("copy-template").addEventListener("click", async () => {
      await navigator.clipboard.writeText($("template-output").value);
      toast("已复制");
    });
    $("order-rows").addEventListener("click", (event) => {
      const button = event.target.closest("button[data-retry-id]");
      if (!button) return;
      retryOrder(button.dataset.retryId).catch((err) => toast(err.message));
    });
    $("position-rows").addEventListener("click", (event) => {
      const button = event.target.closest("button[data-position-close]");
      if (!button) return;
      closePosition(button).catch((err) => toast(err.message));
    });
    $("refresh-orders").addEventListener("click", () => loadOrders().then(() => toast("订单已刷新")).catch((err) => toast(err.message)));
    $("refresh-upgrade").addEventListener("click", () => loadUpgrade().then(() => toast("升级状态已刷新")).catch((err) => toast(err.message)));
    $("start-upgrade").addEventListener("click", () => startUpgrade().catch((err) => toast(err.message)));

    activateTab(initialTab(), false);
    loadAll().catch((err) => toast(err.message));
  </script>
</body>
</html>`

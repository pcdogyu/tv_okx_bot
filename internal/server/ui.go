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
    .status[hidden] {
      display: none;
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
    .status-cell {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
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
    .exchange-tabs {
      display: inline-flex;
      gap: 6px;
      align-items: center;
      flex-wrap: wrap;
      margin-top: 8px;
    }
    .exchange-tabs button {
      border: 1px solid var(--line);
      border-radius: 6px;
      background: #fff;
      color: var(--text);
      min-height: 32px;
      padding: 6px 10px;
      font: inherit;
      cursor: pointer;
    }
    .exchange-tabs button[aria-selected="true"] {
      background: var(--blue);
      border-color: var(--blue);
      color: #fff;
    }
    .table-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .position-actions {
      display: flex;
      gap: 4px;
      flex-wrap: nowrap;
      white-space: nowrap;
    }
    .btn.small {
      min-height: 28px;
      padding: 4px 8px;
      font-size: 12px;
    }
    .btn.order-json-button {
      min-height: 24px;
      padding: 2px 7px;
    }
    .positions-table .position-actions {
      gap: 6px;
      flex-wrap: wrap;
    }
    .position-protection-btn {
      min-width: 42px;
    }
    .position-percent-close-btn {
      width: 36px;
      height: 36px;
      min-width: 36px;
      min-height: 36px;
      padding: 0;
      border-radius: 999px;
      font-size: 7px;
      line-height: 1;
      flex: 0 0 36px;
    }
    .btn:disabled, .btn.is-disabled {
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
    .exchange-summary {
      display: inline-flex;
      align-items: center;
      min-height: 36px;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 8px 10px;
      color: var(--muted);
      background: #f8fafc;
      font-weight: 650;
    }
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
    .menu-label-input {
      max-width: 260px;
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
    .balance-window-toolbar {
      display: flex;
      align-items: center;
      gap: 7px;
      flex-wrap: wrap;
      margin: 0 0 14px;
    }
    .balance-window-toolbar .balance-window-btn {
      font-size: 16px;
    }
    .balance-window-btn[aria-selected="true"] {
      background: var(--blue);
      border-color: var(--blue);
      color: #fff;
    }
    .dashboard-balance-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
      margin-top: 14px;
    }
    .analysis-balance-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
      margin-bottom: 14px;
    }
    .balance-chart-card {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #fff;
      padding: 10px;
      min-height: 360px;
    }
    .exchange-balance-card {
      display: grid;
      gap: 10px;
      align-content: start;
    }
    .exchange-balance-metrics {
      grid-template-columns: repeat(2, minmax(0, 1fr));
      min-height: 0;
      align-content: start;
      margin: 0;
    }
    .balance-table-wrap {
      height: auto;
      max-height: 188px;
      overflow: auto;
    }
    .exchange-balance-card table {
      margin-top: 0;
    }
    .balance-table-wrap th {
      position: sticky;
      top: 0;
      z-index: 1;
    }
    #usdt-chart {
      width: 100%;
      height: 420px;
      display: block;
    }
    .mini-usdt-chart {
      width: 100%;
      height: 250px;
      display: block;
    }
    #usdt-chart.mini-usdt-chart {
      height: 250px;
    }
    #analysis .mini-usdt-chart {
      height: 360px;
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
    .analysis-table-wrap {
      overflow-x: auto;
    }
    .balance-pnl-block {
      border-top: 1px solid var(--line);
      padding-top: 10px;
    }
    .balance-pnl-block .section-head {
      margin-bottom: 8px;
    }
    .balance-pnl-block .analysis-metrics {
      grid-template-columns: repeat(5, minmax(88px, 1fr));
      gap: 8px;
      margin: 8px 0;
    }
    .balance-pnl-block .analysis-card {
      min-height: 58px;
      padding: 8px;
    }
    .balance-pnl-block .analysis-card .value {
      font-size: 15px;
    }
    .balance-pnl-block .analysis-table-wrap {
      max-height: 188px;
      overflow: auto;
    }
    .balance-pnl-block .analysis-symbol-table {
      min-width: 920px;
    }
    .balance-pnl-block th {
      position: sticky;
      top: 0;
      z-index: 1;
    }
    .analysis-symbol-table {
      min-width: 1080px;
    }
    .analysis-trade-table {
      min-width: 1280px;
    }
    .analysis-subsection {
      border-top: 1px solid var(--line);
      margin-top: 16px;
      padding-top: 12px;
    }
    #analysis-trade-history-section {
      grid-column: 1 / -1;
      margin-top: 0;
    }
    .analysis-usdt-chart-card {
      min-height: 0;
    }
    .analysis-exchange-block + .analysis-exchange-block {
      margin-top: 14px;
    }
    .analysis-pagination {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 8px;
      flex-wrap: wrap;
    }
    .positions-table {
      min-width: 1400px;
    }
    .positions-table th,
    .positions-table td {
      font-size: 12px;
      line-height: 1.35;
    }
    .symbol-table th[draggable="true"] {
      cursor: grab;
      user-select: none;
    }
    .symbol-table th.is-dragging {
      opacity: 0.55;
    }
    .symbol-table th.is-drop-target {
      outline: 2px solid var(--accent);
      outline-offset: -2px;
    }
    .positions-table .pos-exchange-col { width: 4.8%; }
    .positions-table .pos-symbol-col { width: 7.4%; }
    .positions-table .pos-side-col { width: 5.2%; }
    .positions-table .pos-size-col { width: 6.1%; }
    .positions-table .pos-price-col { width: 5.6%; }
    .positions-table .pos-margin-col { width: 5.9%; }
    .positions-table .pos-leverage-col { width: 3.9%; }
    .positions-table .pos-position-amount-col { width: 6.3%; }
    .positions-table .pos-pnl-col { width: 6.5%; }
    .positions-table .pos-rate-col { width: 5.2%; }
    .positions-table .pos-entry-time-col { width: 6.9%; }
    .positions-table .pos-holding-time-col { width: 5.6%; }
    .positions-table .pos-actions-col { width: 27.5%; }
    .pending-order-table {
      min-width: 1280px;
    }
    .pending-order-table .pending-exchange-col { width: 6%; }
    .pending-order-table .pending-time-col { width: 10%; }
    .pending-order-table .pending-symbol-col { width: 10.5%; }
    .pending-order-table .pending-side-col { width: 5.5%; }
    .pending-order-table .pending-pos-side-col { width: 6.5%; }
    .pending-order-table .pending-type-col { width: 6.5%; }
    .pending-order-table .pending-price-col { width: 8.5%; }
    .pending-order-table .pending-mid-col { width: 8.5%; }
    .pending-order-table .pending-size-col { width: 8%; }
    .pending-order-table .pending-margin-col { width: 8%; }
    .pending-order-table .pending-filled-col { width: 7%; }
    .pending-order-table .pending-state-col { width: 6.5%; }
    .pending-order-table .pending-actions-col { width: 8.5%; }
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
    .raw-json-dialog {
      width: min(780px, calc(100vw - 32px));
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 0;
      color: var(--text);
      background: #fff;
      box-shadow: var(--shadow);
    }
    .raw-json-dialog::backdrop {
      background: rgba(15, 23, 42, 0.45);
    }
    .raw-json-dialog:not([open]) {
      display: none;
    }
    .raw-json-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      padding: 12px 14px;
      border-bottom: 1px solid var(--line);
    }
    .raw-json-dialog pre {
      border-radius: 0;
      min-height: 260px;
      max-height: min(62vh, 560px);
      overflow: auto;
    }
    .raw-json-actions {
      padding: 12px 14px;
      margin-top: 0;
      border-top: 1px solid var(--line);
      justify-content: flex-end;
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
      .status, .grid, .grid.two, .split, .api-key-layout, .analysis-metrics, .asset-metrics, .symbol-metrics, .position-metrics, .dashboard-balance-grid, .analysis-balance-grid { grid-template-columns: 1fr; }
      main { padding: 12px; }
      section { padding: 12px; }
      #usdt-chart { height: 320px; }
      #usdt-chart.mini-usdt-chart { height: 240px; }
      .mini-usdt-chart { height: 240px; }
      #analysis .mini-usdt-chart { height: 346px; }
      .exchange-balance-metrics { min-height: 0; }
      .balance-table-wrap { height: auto; max-height: 260px; }
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
    <div class="status" id="order-info-status" hidden>
      <div class="metric"><div class="label">交易环境</div><div class="value" id="metric-env">-</div></div>
      <div class="metric"><div class="label">交易 API</div><div class="value" id="metric-api-keys">-</div></div>
      <div class="metric"><div class="label">下单金额</div><div class="value" id="metric-amount">-</div></div>
      <div class="metric"><div class="label">最近信号</div><div class="value" id="metric-orders">-</div></div>
    </div>

    <section id="dashboard" class="active">
      <div class="section-head">
        <h2>总览</h2>
        <div class="actions" style="margin-top:0">
          <button class="btn" type="button" id="refresh-all">刷新</button>
          <button class="btn success" type="button" id="check-okx">检查交易所</button>
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
          <h3>交易所检查</h3>
          <pre id="okx-output">-</pre>
        </div>
      </div>
      <div class="dashboard-balance-grid">
        <div class="balance-chart-card">
          <div class="section-head" style="margin-bottom:8px">
            <h3>OKX USDT 余额</h3>
            <span class="muted" id="overview-okx-status">-</span>
          </div>
          <div class="analysis-metrics symbol-metrics">
            <div class="analysis-card"><div class="label">USDT估值</div><div class="value" id="overview-okx-eq">-</div></div>
            <div class="analysis-card"><div class="label">可用</div><div class="value" id="overview-okx-avail">-</div></div>
            <div class="analysis-card"><div class="label">交易 API</div><div class="value" id="overview-okx-api">-</div></div>
            <div class="analysis-card"><div class="label">更新时间</div><div class="value" id="overview-okx-updated">-</div></div>
          </div>
          <svg id="overview-okx-usdt-chart" class="mini-usdt-chart" role="img" aria-label="OKX USDT balance chart"></svg>
        </div>
        <div class="balance-chart-card">
          <div class="section-head" style="margin-bottom:8px">
            <h3>Binance USDT 余额</h3>
            <span class="muted" id="overview-binance-status">-</span>
          </div>
          <div class="analysis-metrics symbol-metrics">
            <div class="analysis-card"><div class="label">USDT估值</div><div class="value" id="overview-binance-eq">-</div></div>
            <div class="analysis-card"><div class="label">可用</div><div class="value" id="overview-binance-avail">-</div></div>
            <div class="analysis-card"><div class="label">交易 API</div><div class="value" id="overview-binance-api">-</div></div>
            <div class="analysis-card"><div class="label">更新时间</div><div class="value" id="overview-binance-updated">-</div></div>
          </div>
          <svg id="overview-binance-usdt-chart" class="mini-usdt-chart" role="img" aria-label="Binance USDT balance chart"></svg>
        </div>
      </div>
    </section>

    <section id="positions">
      <div class="section-head">
        <h2>当前持仓</h2>
        <div class="analysis-controls">
          <span class="exchange-summary" id="position-exchange-summary">OKX / Binance USDⓈ-M</span>
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
        <table class="symbol-table positions-table">
          <colgroup id="position-cols"></colgroup>
          <thead id="position-head"></thead>
          <tbody id="position-rows"></tbody>
        </table>
      </div>
      <div class="section-head" style="margin:18px 0 10px">
        <h3>当前挂单</h3>
        <span class="muted" id="pending-orders-updated">-</span>
      </div>
      <div class="symbol-table-wrap">
        <table class="symbol-table pending-order-table">
          <colgroup id="pending-order-cols"></colgroup>
          <thead id="pending-order-head"></thead>
          <tbody id="pending-order-rows"></tbody>
        </table>
      </div>
    </section>

    <section id="analysis">
      <div class="section-head">
        <h2>订单分析</h2>
        <div class="analysis-controls">
          <span class="muted" id="analysis-updated">-</span>
          <label>交易 API<select id="analysis-api-id"></select></label>
          <button class="btn primary" type="button" id="refresh-analysis">刷新分析</button>
        </div>
      </div>
      <div class="balance-window-toolbar" role="group" aria-label="USDT余额周期">
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="0">当前</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="5">5m</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="10">10m</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="15">15m</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="30">30m</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="60">1h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="240">4h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="480">8h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="720">12h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="1440">24h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="2880">48h</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="4320">3d</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="10080">7d</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="43200">30d</button>
        <button class="btn small balance-window-btn" type="button" data-balance-minutes="129600">90d</button>
        <button class="btn small" type="button" id="reset-balance-baseline">重置基准</button>
        <button class="btn small" type="button" id="sync-balance-history">同步历史</button>
      </div>
      <div class="analysis-balance-grid">
        <div class="balance-chart-card exchange-balance-card">
          <div class="section-head" style="margin-bottom:0">
            <h3>USDT 余额表</h3>
            <span class="muted" id="analysis-okx-balance-status">-</span>
          </div>
          <div class="analysis-metrics symbol-metrics exchange-balance-metrics">
            <div class="analysis-card"><div class="label">USDT估值</div><div class="value" id="analysis-usdt-eq">-</div></div>
            <div class="analysis-card"><div class="label">更新时间</div><div class="value" id="analysis-balance-updated">-</div></div>
          </div>
          <div class="analysis-exchange-block balance-pnl-block">
            <div class="section-head" style="margin-top:0">
              <h3>OKX 盈亏分析</h3>
              <span class="muted" id="analysis-okx-symbol-status">-</span>
            </div>
            <div class="analysis-metrics">
              <div class="analysis-card"><div class="label">净盈亏</div><div class="value" id="analysis-okx-net-pnl">-</div></div>
              <div class="analysis-card"><div class="label">胜率</div><div class="value" id="analysis-okx-win-rate">-</div></div>
              <div class="analysis-card"><div class="label">盈利因子</div><div class="value" id="analysis-okx-profit-factor">-</div></div>
              <div class="analysis-card"><div class="label">盈亏比</div><div class="value" id="analysis-okx-payoff-ratio">-</div></div>
              <div class="analysis-card"><div class="label">成交数</div><div class="value" id="analysis-okx-trades">-</div></div>
            </div>
            <div class="analysis-table-wrap">
              <table class="analysis-symbol-table">
                <thead>
                  <tr><th>币对</th><th>成交数</th><th>盈利数</th><th>亏损数</th><th>净盈亏</th><th>手续费</th><th>胜率</th><th>盈利因子</th><th>盈亏比</th></tr>
                </thead>
                <tbody id="analysis-okx-rows"></tbody>
              </table>
            </div>
          </div>
          <div class="balance-table-wrap">
            <table>
              <thead>
                <tr><th>币种</th><th>权益</th><th>可用余额</th><th>现金余额</th><th>冻结</th></tr>
              </thead>
              <tbody id="analysis-balance-rows"></tbody>
            </table>
          </div>
        </div>
        <div class="balance-chart-card exchange-balance-card">
          <div class="section-head" style="margin-bottom:0">
            <h3>USDT 余额表</h3>
            <span class="muted" id="analysis-binance-balance-status">-</span>
          </div>
          <div class="analysis-metrics symbol-metrics exchange-balance-metrics">
            <div class="analysis-card"><div class="label">USDT估值</div><div class="value" id="analysis-binance-usdt-eq">-</div></div>
            <div class="analysis-card"><div class="label">更新时间</div><div class="value" id="analysis-binance-balance-updated">-</div></div>
          </div>
          <div class="analysis-exchange-block balance-pnl-block">
            <div class="section-head" style="margin-top:0">
              <h3>Binance 盈亏分析</h3>
              <span class="muted" id="analysis-binance-symbol-status">-</span>
            </div>
            <div class="analysis-metrics">
              <div class="analysis-card"><div class="label">净盈亏</div><div class="value" id="analysis-binance-net-pnl">-</div></div>
              <div class="analysis-card"><div class="label">胜率</div><div class="value" id="analysis-binance-win-rate">-</div></div>
              <div class="analysis-card"><div class="label">盈利因子</div><div class="value" id="analysis-binance-profit-factor">-</div></div>
              <div class="analysis-card"><div class="label">盈亏比</div><div class="value" id="analysis-binance-payoff-ratio">-</div></div>
              <div class="analysis-card"><div class="label">成交数</div><div class="value" id="analysis-binance-trades">-</div></div>
            </div>
            <div class="analysis-table-wrap">
              <table class="analysis-symbol-table">
                <thead>
                  <tr><th>币对</th><th>成交数</th><th>盈利数</th><th>亏损数</th><th>净盈亏</th><th>手续费</th><th>胜率</th><th>盈利因子</th><th>盈亏比</th></tr>
                </thead>
                <tbody id="analysis-binance-rows"></tbody>
              </table>
            </div>
          </div>
          <div class="balance-table-wrap">
            <table>
              <thead>
                <tr><th>币种</th><th>权益</th><th>可用余额</th><th>现金余额</th><th>冻结</th></tr>
              </thead>
              <tbody id="analysis-binance-balance-rows"></tbody>
            </table>
          </div>
        </div>
        <div class="analysis-subsection" id="analysis-trade-history-section">
          <div class="section-head" style="margin-top:0">
            <h3>成交历史</h3>
            <div class="analysis-pagination">
              <span class="muted" id="analysis-trade-page-info">-</span>
              <button class="btn small" type="button" id="analysis-trade-prev">上一页</button>
              <button class="btn small" type="button" id="analysis-trade-next">下一页</button>
            </div>
          </div>
          <div class="analysis-table-wrap">
            <table class="analysis-trade-table">
              <thead>
                <tr><th>交易所</th><th>成交时间</th><th>币对</th><th>方向</th><th>成交价</th><th>成交量</th><th>已实现盈亏</th><th>手续费</th><th>订单 ID</th><th>成交 ID</th></tr>
              </thead>
              <tbody id="analysis-trade-rows"></tbody>
            </table>
          </div>
        </div>
        <div class="balance-chart-card exchange-balance-card analysis-usdt-chart-card">
          <div class="section-head" style="margin:2px 0 0">
            <h3 id="analysis-okx-usdt-title">USDT余额</h3>
          </div>
          <svg id="usdt-chart" class="mini-usdt-chart" role="img" aria-label="OKX USDT balance chart"></svg>
        </div>
        <div class="balance-chart-card exchange-balance-card analysis-usdt-chart-card">
          <div class="section-head" style="margin:2px 0 0">
            <h3 id="analysis-binance-usdt-title">USDT余额</h3>
          </div>
          <svg id="analysis-binance-usdt-chart" class="mini-usdt-chart" role="img" aria-label="Binance USDT balance chart"></svg>
        </div>
      </div>
    </section>

    <section id="apiKeys">
      <div class="section-head">
        <div>
          <h2>API Key</h2>
          <div class="exchange-tabs" id="api-key-exchange-tabs">
            <button type="button" data-api-key-exchange="okx" aria-selected="true">OKX</button>
            <button type="button" data-api-key-exchange="binance">Binance</button>
          </div>
        </div>
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
            <label id="key-api-label">OKX API Key<input id="key-api" autocomplete="off" spellcheck="false"></label>
            <label id="key-secret-label">OKX Secret Key<input id="key-secret" type="password" autocomplete="new-password" spellcheck="false"></label>
            <label id="key-passphrase-label">OKX Passphrase<input id="key-passphrase" type="password" autocomplete="new-password" spellcheck="false"></label>
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
        <label>Binance Base URL<input id="cfg-binance-base-url" autocomplete="off"></label>
        <label>Binance Demo Base URL<input id="cfg-binance-demo-base-url" autocomplete="off"></label>
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
          <label>Webhook URL<input id="template-webhook-url" readonly></label>
          <label>下单去向<select id="tpl-target-exchange"><option value="okx">OKX</option><option value="binance">Binance USDⓈ-M</option></select></label>
          <label>交易 API<select id="tpl-api-id"></select></label>
          <label>价格源<select id="tpl-price-source"><option value="close">close</option><option value="high">high</option><option value="low">low</option></select></label>
          <div class="actions" style="margin-top:0"><button class="btn" type="button" id="copy-webhook-url">复制 URL</button></div>
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
          <tr><th>时间</th><th>状态</th><th>信号来源</th><th>下单去向</th><th>方向</th><th>币对</th><th>价格</th><th>金额</th><th class="order-okx">交易所 / 返回</th></tr>
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
          <tr><th>首页</th><th>默认菜单</th><th>菜单名称</th><th>是否隐藏</th><th>排序</th></tr>
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
  <dialog class="raw-json-dialog" id="raw-json-dialog">
    <div class="raw-json-head">
      <h3>原始 JSON</h3>
      <button class="btn small" type="button" id="close-raw-json">关闭</button>
    </div>
    <pre id="raw-json-output">-</pre>
    <div class="actions raw-json-actions">
      <button class="btn" type="button" id="copy-raw-json">复制 JSON</button>
    </div>
  </dialog>

  <script>
    const state = {
      config: null,
      apiKeys: null,
      apiKeysByExchange: { okx: null, binance: null },
      apiKeyExchange: "okx",
      selectedAPIID: "",
      selectedAPIIDs: { okx: "", binance: "" },
      apiKeyTest: null,
      apiKeyTestID: "",
      apiKeyTestExchange: "okx",
      orders: [],
      retrying: {},
      positionClosing: {},
      positionProtecting: {},
      pendingOrderActions: {},
      analysis: null,
      analysisError: "",
      analysisTradePage: 1,
      symbolPrecisions: {},
      balanceOverview: null,
      balanceOverviewError: "",
      balanceWindowMinutes: 720,
      positions: null,
      positionsError: "",
      pendingOrders: null,
      pendingOrdersError: "",
      symbols: null,
      symbolsError: "",
      upgrade: null
    };
    let positionViewPollTimer = null;
    let positionViewPollBusy = false;
    let positionEntryTimeSyncTimer = null;
    let positionEntryTimeSyncBusy = false;
    let analysisBalanceRefreshTimer = null;
    let analysisBalanceRefreshBusy = false;
    let menuSettingsSynced = false;
    let tableColumnDrag = null;
    const analysisTradePageSize = 20;
    const positionViewPollIntervalMs = 5000;
    const missingPositionEntrySyncIntervalMs = 180000;
    const analysisBalanceRefreshIntervalMs = 60000;
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
    const positionExchanges = ["okx", "binance"];
    const balanceWindowOptions = [
      { minutes: 0, label: "当前" },
      { minutes: 5, label: "5m" },
      { minutes: 10, label: "10m" },
      { minutes: 15, label: "15m" },
      { minutes: 30, label: "30m" },
      { minutes: 60, label: "1h" },
      { minutes: 240, label: "4h" },
      { minutes: 480, label: "8h" },
      { minutes: 720, label: "12h" },
      { minutes: 1440, label: "24h" },
      { minutes: 2880, label: "48h" },
      { minutes: 4320, label: "3d" },
      { minutes: 10080, label: "7d" },
      { minutes: 43200, label: "30d" },
      { minutes: 129600, label: "90d" }
    ];
    const $ = (id) => document.getElementById(id);

    const positionTableColumnDefs = [
      { id: "exchange", title: "交易所", colClass: "pos-exchange-col", cell: (row) => textTableCell(exchangeLabel(normalizeExchange(row._exchange || "okx"))) },
      { id: "symbol", title: "币对", colClass: "pos-symbol-col", cell: (row) => textTableCell(asText(row.instId)) },
      { id: "side", title: "方向", colClass: "pos-side-col", cell: (row) => positionSideCell(row) },
      { id: "size", title: "持仓量", colClass: "pos-size-col", cell: (row) => textTableCell(formatQuantityAmount(row, row.pos)) },
      { id: "avg_price", title: "均价", colClass: "pos-price-col", cell: (row) => textTableCell(formatPriceAmount(row, row.avgPx)) },
      { id: "margin", title: "保证金", colClass: "pos-margin-col", cell: (row) => textTableCell(formatNumber(row.margin)) },
      { id: "leverage", title: "杠杆", colClass: "pos-leverage-col", cell: (row) => textTableCell(asText(row.lever)) },
      { id: "position_amount", title: "仓位金额", colClass: "pos-position-amount-col", cell: (row) => textTableCell(positionAmount(row)) },
      { id: "mark_price", title: "标记价", colClass: "pos-price-col", cell: (row) => textTableCell(formatPriceAmount(row, row.markPx)) },
      { id: "upl", title: "未实现盈亏", colClass: "pos-pnl-col", cell: (row) => signedCell(row.upl, formatNumber(row.upl)) },
      { id: "return_rate", title: "收益率", colClass: "pos-rate-col", cell: (row) => signedCell(positionReturnRatio(row), positionReturnPercent(row)) },
      { id: "entry_time", title: "下单时间", colClass: "pos-entry-time-col", cell: (row) => positionEntryTimeCell(row) },
      { id: "holding_time", title: "持仓时间", colClass: "pos-holding-time-col", cell: (row) => positionHoldingTimeCell(row) },
      { id: "actions", title: "操作", colClass: "pos-actions-col", cell: (row) => positionActionCell(row) }
    ];
    const pendingOrderTableColumnDefs = [
      { id: "exchange", title: "交易所", colClass: "pending-exchange-col", cell: (row) => textTableCell(exchangeLabel(normalizeExchange(row._exchange || "okx"))) },
      { id: "time", title: "时间", colClass: "pending-time-col", cell: (row) => timeTableCell(shanghaiTimeFromOKX(row.cTime || row.uTime)) },
      { id: "symbol", title: "币对", colClass: "pending-symbol-col", cell: (row) => textTableCell(asText(row.instId)) },
      { id: "side", title: "方向", colClass: "pending-side-col", cell: (row) => textTableCell(tradeSideText(row.side)) },
      { id: "position_side", title: "持仓方向", colClass: "pending-pos-side-col", cell: (row) => textTableCell(positionSideText(row.posSide, "")) },
      { id: "type", title: "类型", colClass: "pending-type-col", cell: (row) => textTableCell(orderTypeText(row.ordType)) },
      { id: "price", title: "委托价格", colClass: "pending-price-col", cell: (row) => textTableCell(formatPriceAmount(row, row.px)) },
      { id: "mid_price", title: "中间价", colClass: "pending-mid-col", cell: (row) => textTableCell(row.price_error ? row.price_error : formatPriceAmount(row, row.mid_px)) },
      { id: "size", title: "委托量", colClass: "pending-size-col", cell: (row) => textTableCell(formatQuantityAmount(row, row.sz)) },
      { id: "margin", title: "保证金", colClass: "pending-margin-col", cell: (row) => textTableCell(formatNumber(row.margin)) },
      { id: "filled", title: "已成交", colClass: "pending-filled-col", cell: (row) => textTableCell(formatQuantityAmount(row, row.accFillSz)) },
      { id: "state", title: "状态", colClass: "pending-state-col", cell: (row) => textTableCell(pendingOrderStateText(row.state)) },
      { id: "actions", title: "操作", colClass: "pending-actions-col", cell: (row) => pendingOrderActionCell(row) }
    ];

    function tableColumnDefs(tableID) {
      return tableID === "pending_orders" ? pendingOrderTableColumnDefs : positionTableColumnDefs;
    }

    function tableColumnPartIDs(tableID) {
      if (tableID === "pending_orders") return { cols: "pending-order-cols", head: "pending-order-head" };
      return { cols: "position-cols", head: "position-head" };
    }

    function normalizeTableColumnOrder(tableID, order) {
      const defs = tableColumnDefs(tableID);
      const known = {};
      const seen = {};
      const out = [];
      defs.forEach((def) => { known[def.id] = true; });
      (Array.isArray(order) ? order : []).forEach((raw) => {
        const id = String(raw || "").trim();
        if (!known[id] || seen[id]) return;
        out.push(id);
        seen[id] = true;
      });
      defs.forEach((def) => {
        if (!seen[def.id]) out.push(def.id);
      });
      return out;
    }

    function currentTableColumnOrder(tableID) {
      const ui = state.config && state.config.ui ? state.config.ui : {};
      const columns = ui.table_columns || {};
      return normalizeTableColumnOrder(tableID, columns[tableID]);
    }

    function setCurrentTableColumnOrder(tableID, order) {
      if (!state.config) state.config = {};
      if (!state.config.ui) state.config.ui = {};
      if (!state.config.ui.table_columns) state.config.ui.table_columns = {};
      state.config.ui.table_columns[tableID] = normalizeTableColumnOrder(tableID, order);
    }

    function currentTableColumnDefs(tableID) {
      const byID = {};
      tableColumnDefs(tableID).forEach((def) => { byID[def.id] = def; });
      return currentTableColumnOrder(tableID).map((id) => byID[id]).filter(Boolean);
    }

    function tableColumnCount(tableID) {
      return currentTableColumnDefs(tableID).length;
    }

    function renderTableStructure(tableID) {
      const parts = tableColumnPartIDs(tableID);
      const cols = $(parts.cols);
      const head = $(parts.head);
      const columnDefs = currentTableColumnDefs(tableID);
      if (cols) {
        cols.innerHTML = columnDefs.map((col) => '<col class="' + escapeHTML(col.colClass) + '">').join("");
      }
      if (head) {
        head.innerHTML = "<tr>" + columnDefs.map((col) =>
          '<th draggable="true" data-table-columns="' + escapeHTML(tableID) + '" data-column-id="' + escapeHTML(col.id) + '" title="拖动调整栏目顺序">' + escapeHTML(col.title) + "</th>"
        ).join("") + "</tr>";
      }
    }

    function currentTableColumnsPatch() {
      return {
        positions: currentTableColumnOrder("positions"),
        pending_orders: currentTableColumnOrder("pending_orders")
      };
    }

    function renderTableByID(tableID) {
      if (tableID === "pending_orders") {
        renderPendingOrders();
        return;
      }
      renderPositions();
    }

    async function saveTableColumnOrder(tableID, previousOrder) {
      try {
        const patch = { ui: { table_columns: currentTableColumnsPatch() } };
        state.config = await api("/tvbot/config", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
        toast("栏目顺序已保存");
      } catch (err) {
        setCurrentTableColumnOrder(tableID, previousOrder);
        renderTableByID(tableID);
        toast(err.message);
      }
    }

    function clearTableColumnDropTargets() {
      document.querySelectorAll(".symbol-table th.is-drop-target, .symbol-table th.is-dragging").forEach((node) => {
        node.classList.remove("is-drop-target");
        node.classList.remove("is-dragging");
      });
    }

    function handleTableColumnDragStart(event) {
      const th = event.target.closest("th[data-table-columns][data-column-id]");
      if (!th) return;
      tableColumnDrag = { tableID: th.dataset.tableColumns, columnID: th.dataset.columnId };
      th.classList.add("is-dragging");
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", tableColumnDrag.columnID);
      }
    }

    function handleTableColumnDragOver(event) {
      const th = event.target.closest("th[data-table-columns][data-column-id]");
      if (!th || !tableColumnDrag || th.dataset.tableColumns !== tableColumnDrag.tableID) return;
      event.preventDefault();
      th.classList.add("is-drop-target");
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    }

    function handleTableColumnDragLeave(event) {
      const th = event.target.closest("th[data-table-columns][data-column-id]");
      if (th) th.classList.remove("is-drop-target");
    }

    function handleTableColumnDragEnd() {
      tableColumnDrag = null;
      clearTableColumnDropTargets();
    }

    function handleTableColumnDrop(event) {
      const th = event.target.closest("th[data-table-columns][data-column-id]");
      if (!th || !tableColumnDrag || th.dataset.tableColumns !== tableColumnDrag.tableID) return;
      event.preventDefault();
      const tableID = tableColumnDrag.tableID;
      const fromID = tableColumnDrag.columnID;
      const toID = th.dataset.columnId;
      const previousOrder = currentTableColumnOrder(tableID);
      const fromIndex = previousOrder.indexOf(fromID);
      const toIndex = previousOrder.indexOf(toID);
      tableColumnDrag = null;
      clearTableColumnDropTargets();
      if (fromIndex < 0 || toIndex < 0 || fromIndex === toIndex) return;
      const nextOrder = previousOrder.slice();
      const moved = nextOrder.splice(fromIndex, 1)[0];
      nextOrder.splice(toIndex, 0, moved);
      setCurrentTableColumnOrder(tableID, nextOrder);
      renderTableByID(tableID);
      saveTableColumnOrder(tableID, previousOrder).catch((err) => toast(err.message));
    }

    function initTableColumnDrag() {
      ["position-head", "pending-order-head"].forEach((id) => {
        const head = $(id);
        if (!head) return;
        head.addEventListener("dragstart", handleTableColumnDragStart);
        head.addEventListener("dragover", handleTableColumnDragOver);
        head.addEventListener("dragleave", handleTableColumnDragLeave);
        head.addEventListener("dragend", handleTableColumnDragEnd);
        head.addEventListener("drop", handleTableColumnDrop);
      });
    }

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

    function normalizeExchange(value) {
      const raw = String(value || "").trim().toLowerCase();
      if (raw === "binance" || raw === "binance_usdm" || raw === "binance-usdm" || raw === "usdm") return "binance";
      return "okx";
    }

    function exchangeLabel(value) {
      return normalizeExchange(value) === "binance" ? "Binance" : "OKX";
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

    function normalizedPrecision(value) {
      const precision = Number(value);
      if (!Number.isFinite(precision) || precision < 0) return null;
      return Math.min(20, Math.floor(precision));
    }

    function formatFixedPrecision(v, precision, fallback) {
      if (v === null || v === undefined || String(v).trim() === "") return "-";
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      const digits = normalizedPrecision(precision);
      if (digits === null) return fallback(v);
      return n.toLocaleString("zh-CN", { minimumFractionDigits: digits, maximumFractionDigits: digits });
    }

    function formatPriceAmount(row, value) {
      return formatFixedPrecision(value, row ? row.price_precision : null, formatNumber);
    }

    function formatQuantityAmount(row, value) {
      return formatFixedPrecision(value, row ? row.quantity_precision : null, formatAssetAmount);
    }

    function symbolPrecisionKey(exchange, instID) {
      const normalized = normalizeExchange(exchange);
      const key = normalizePrecisionInstID(normalized, instID);
      return key ? normalized + "|" + key : "";
    }

    function normalizePrecisionInstID(exchange, value) {
      let raw = String(value || "").trim().toUpperCase();
      if (!raw) return "";
      const colon = raw.lastIndexOf(":");
      if (colon >= 0) raw = raw.slice(colon + 1);
      raw = raw.replace(/\.P$/, "").replace(/\.PERP$/, "").replace(/PERP$/, "");
      raw = raw.replace(/\s+/g, "");
      if (normalizeExchange(exchange) === "binance") {
        raw = raw.replace(/[-_/]/g, "");
        if (raw && !raw.endsWith("USDT")) raw += "USDT";
        return raw;
      }
      raw = raw.replace(/_/g, "-").replace(/\//g, "-");
      if (raw.endsWith("-SWAP")) return raw;
      if (raw.includes("-")) {
        const parts = raw.split("-").filter(Boolean);
        if (parts.length >= 2) return parts[0] + "-" + parts[1] + "-SWAP";
      }
      if (raw.endsWith("USDT") && raw.length > 4) return raw.slice(0, -4) + "-USDT-SWAP";
      return raw + "-USDT-SWAP";
    }

    function rememberSymbolPrecision(exchange, row) {
      const key = symbolPrecisionKey(exchange, row && row.instId);
      if (!key) return;
      const pricePrecision = normalizedPrecision(row && row.price_precision);
      const quantityPrecision = normalizedPrecision(row && row.quantity_precision);
      if (pricePrecision === null && quantityPrecision === null) return;
      const current = state.symbolPrecisions[key] || {};
      state.symbolPrecisions[key] = Object.assign({}, current, {
        price_precision: pricePrecision === null ? current.price_precision : pricePrecision,
        quantity_precision: quantityPrecision === null ? current.quantity_precision : quantityPrecision
      });
    }

    function symbolPrecision(exchange, instID) {
      return state.symbolPrecisions[symbolPrecisionKey(exchange, instID)] || null;
    }

    function formatCachedSymbolPrice(exchange, instID, value) {
      const precision = symbolPrecision(exchange, instID);
      return formatFixedPrecision(value, precision ? precision.price_precision : null, formatNumber);
    }

    function formatUSDTBalance(v) {
      const n = Number(v);
      if (!Number.isFinite(n)) return "-";
      return Math.round(n).toLocaleString("zh-CN", { minimumFractionDigits: 0, maximumFractionDigits: 0 });
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

    function formatHoldingSeconds(value) {
      const seconds = Number(value);
      if (!Number.isFinite(seconds) || seconds < 0) return "-";
      if (seconds < 60) return "<1分钟";
      const totalMinutes = Math.floor(seconds / 60);
      const days = Math.floor(totalMinutes / 1440);
      const hours = Math.floor((totalMinutes % 1440) / 60);
      const minutes = totalMinutes % 60;
      const hh = String(hours).padStart(2, "0");
      const mm = String(minutes).padStart(2, "0");
      return days > 0 ? (days + "天 " + hh + ":" + mm) : (hh + ":" + mm);
    }

    function entryTimeSourceText(value) {
      const source = String(value || "").toLowerCase();
      if (source === "okx_fills_history") return "OKX 成交明细";
      if (source === "binance_user_trades") return "Binance 成交明细";
      if (source === "exchange_position_time") return "交易所持仓时间";
      return asText(value);
    }

    function balanceAmount(v) {
      if (v === null || v === undefined || v === "") return "-";
      const formatted = formatNumber(v);
      return (formatted === "-" ? asText(v) : formatted) + " USDT";
    }

    function balanceWindowLabel(minutes) {
      const normalized = Number(minutes || 0);
      const found = balanceWindowOptions.find((item) => item.minutes === normalized);
      if (found) return found.label;
      if (normalized <= 0) return "当前";
      if (normalized % 1440 === 0) return (normalized / 1440) + "d";
      if (normalized % 60 === 0) return (normalized / 60) + "h";
      return normalized + "m";
    }

    function analysisPNLWindowMinutes() {
      const minutes = Math.max(0, Number(state.balanceWindowMinutes || 0));
      if (!Number.isFinite(minutes) || minutes <= 0) return 5;
      return Math.min(minutes, 30 * 24 * 60);
    }

    function balanceOverviewPath(forceRefresh) {
      const qs = new URLSearchParams({ minutes: String(Math.max(0, Number(state.balanceWindowMinutes || 0))) });
      if (forceRefresh) qs.set("refresh", "true");
      return "/tvbot/balances/overview?" + qs.toString();
    }

    function updateBalanceWindowButtons() {
      document.querySelectorAll("[data-balance-minutes]").forEach((button) => {
        const selected = Number(button.dataset.balanceMinutes || "0") === Number(state.balanceWindowMinutes || 0);
        button.setAttribute("aria-selected", selected ? "true" : "false");
      });
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
      const formatted = formatUSDTBalance(v);
      return formatted === "-" ? asText(v) : formatted;
    }

    function usdtBalanceDetail(balance) {
      const details = balance && Array.isArray(balance.details) ? balance.details : [];
      return details.find((row) => String(row.ccy || "").toUpperCase() === "USDT") || null;
    }

    function usdtBalanceRawValue(row) {
      for (const key of ["cash_bal", "avail_bal", "eq", "eq_usd"]) {
        const raw = row && row[key];
        if (raw !== undefined && raw !== null && String(raw).trim() !== "") return raw;
      }
      return "";
    }

    function usdtBalancePoints(balancePoints, balance) {
      const stored = (Array.isArray(balancePoints) ? balancePoints : []).map((point, index) => {
        return {
          index: index,
          value: Number(point.value !== undefined ? point.value : usdtBalanceRawValue(point)),
          date: chartPointDate(point)
        };
      }).filter((point) => Number.isFinite(point.value));
      if (stored.length) return stored;
      const usdt = usdtBalanceDetail(balance);
      const currentValue = Number(usdtBalanceRawValue(usdt));
      if (!Number.isFinite(currentValue)) return [];
      return [{ index: 0, value: currentValue, date: balance && balance.updated_at ? new Date(balance.updated_at) : null }];
    }

    function pill(text, tone) {
      return '<span class="pill ' + (tone || "") + '">' + escapeHTML(asText(text)) + '</span>';
    }

    function orderRawJSONText(order) {
      const raw = order && order.raw_json ? order.raw_json : "";
      if (!raw) return "";
      if (typeof raw !== "string") return JSON.stringify(raw, null, 2);
      try {
        return JSON.stringify(JSON.parse(raw), null, 2);
      } catch (err) {
        return raw;
      }
    }

    function showOrderRawJSON(order) {
      const text = orderRawJSONText(order);
      if (!text) {
        toast("暂无原始 JSON");
        return;
      }
      $("raw-json-output").textContent = text;
      const dialog = $("raw-json-dialog");
      if (dialog.showModal) {
        dialog.showModal();
      } else {
        dialog.setAttribute("open", "open");
      }
    }

    function closeRawJSONDialog() {
      const dialog = $("raw-json-dialog");
      if (dialog.close) {
        dialog.close();
      } else {
        dialog.removeAttribute("open");
      }
    }

    function escapeHTML(v) {
      return String(v).replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    }

    function textTableCell(value) {
      return "<td>" + escapeHTML(value) + "</td>";
    }

    function timeTableCell(value) {
      return '<td class="time">' + escapeHTML(value) + "</td>";
    }

    function menuDefinition(tabID) {
      return defaultMenuItems.find((item) => item.tab === tabID) || null;
    }

    function defaultMenuTabByLabel(label) {
      const normalized = String(label || "").trim();
      const found = defaultMenuItems.find((item) => item.label === normalized);
      return found ? found.tab : "";
    }

    function menuLabel(item, def) {
      const label = item && item.label ? String(item.label).trim() : "";
      if (!label) return def ? def.label : "";
      const owner = defaultMenuTabByLabel(label);
      if (owner && def && owner !== def.tab) return def.label;
      return label;
    }

    function normalizeMenuItems(items) {
      const seen = {};
      const normalized = [];
      (Array.isArray(items) ? items : []).forEach((item) => {
        const tab = item && item.tab ? String(item.tab) : "";
        const def = menuDefinition(tab);
        if (!def || seen[tab]) return;
        normalized.push({ tab: tab, hidden: def.locked ? false : !!item.hidden, label: menuLabel(item, def) });
        seen[tab] = true;
      });
      defaultMenuItems.forEach((def) => {
        if (!seen[def.tab]) normalized.push({ tab: def.tab, hidden: false, label: def.label });
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

    function configuredDefaultTab() {
      const ui = state.config && state.config.ui ? state.config.ui : {};
      const tab = ui.default_tab ? String(ui.default_tab) : "dashboard";
      return menuDefinition(tab) && $(tab) ? tab : "dashboard";
    }

    function tabButton(tabID) {
      return Array.from(document.querySelectorAll("nav button")).find((button) => button.dataset.tab === tabID) || null;
    }

    function tabVisible(tabID) {
      const button = tabButton(tabID);
      return !!button && !button.hidden && !!$(tabID);
    }

    function firstVisibleTab() {
      const button = Array.from(document.querySelectorAll("nav button")).find((item) => !item.hidden && $(item.dataset.tab));
      return button ? button.dataset.tab : "menuSettings";
    }

    function effectiveDefaultTab() {
      const tab = configuredDefaultTab();
      return tabVisible(tab) ? tab : firstVisibleTab();
    }

    function activeTabID() {
      const active = document.querySelector('nav button[aria-selected="true"]');
      return active ? active.dataset.tab : "";
    }

    function syncActiveTabAfterMenuSettings() {
      const hash = location.hash ? location.hash.slice(1) : "";
      const active = activeTabID();
      if (!hash && !menuSettingsSynced) {
        menuSettingsSynced = true;
        activateTab(effectiveDefaultTab(), false);
        return;
      }
      menuSettingsSynced = true;
      if (!active || !tabVisible(active)) {
        activateTab(effectiveDefaultTab(), false);
      }
    }

    function applyMenuSettings() {
      const nav = document.querySelector("nav");
      if (!nav) return;
      const known = {};
      currentMenuItems().forEach((item) => {
        const button = tabButton(item.tab);
        const def = menuDefinition(item.tab);
        if (!button || !def) return;
        button.textContent = menuLabel(item, def);
        button.hidden = !!item.hidden;
        nav.appendChild(button);
        known[item.tab] = true;
      });
      document.querySelectorAll("nav button").forEach((button) => {
        if (!known[button.dataset.tab]) button.hidden = true;
      });
      const activeButton = document.querySelector('nav button[aria-selected="true"]');
      if (!activeButton || activeButton.hidden) {
        activateTab(effectiveDefaultTab(), false);
      } else {
        syncOrderInfoVisibility(activeButton.dataset.tab || activeTabID());
      }
    }

    function renderMenuSettings() {
      const items = currentMenuItems();
      const defaultTab = configuredDefaultTab();
      $("menu-settings-rows").innerHTML = items.map((item, index) => {
        const def = menuDefinition(item.tab) || { label: item.tab };
        const hiddenCell = def.locked
          ? '<span class="muted">固定显示</span>'
          : '<label class="menu-hidden-check"><input type="checkbox" data-menu-hidden="' + escapeHTML(item.tab) + '"' + (item.hidden ? " checked" : "") + '>隐藏</label>';
        return "<tr>" +
          '<td><input type="radio" name="menu-default-tab" data-menu-home="' + escapeHTML(item.tab) + '"' + (item.tab === defaultTab ? " checked" : "") + ' aria-label="设为首页"></td>' +
          "<td>" + escapeHTML(def.label) + "</td>" +
          '<td><input class="menu-label-input" data-menu-label="' + escapeHTML(item.tab) + '" value="' + escapeHTML(menuLabel(item, def)) + '" maxlength="24" autocomplete="off" spellcheck="false"></td>' +
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

    function positionsTabActive() {
      const section = $("positions");
      return !!(section && section.classList.contains("active"));
    }

    function positionEntryTimesMissing(row) {
      const entryTime = String(row && row.entry_fill_time ? row.entry_fill_time : "").trim();
      const holdingSeconds = Number(row && row.holding_seconds);
      return !entryTime || !Number.isFinite(holdingSeconds) || holdingSeconds < 0;
    }

    function missingPositionEntryTimesDetected() {
      const rows = state.positions && Array.isArray(state.positions.positions) ? state.positions.positions : [];
      return rows.some((row) => positionEntryTimesMissing(row));
    }

    function startPositionViewPolling() {
      if (!positionViewPollTimer) {
        positionViewPollTimer = window.setInterval(async () => {
          if (positionViewPollBusy) return;
          positionViewPollBusy = true;
          try {
            await loadPositionView();
          } catch (err) {
            toast(err.message);
          } finally {
            positionViewPollBusy = false;
          }
        }, positionViewPollIntervalMs);
      }
      startMissingPositionEntryTimeSync();
    }

    function stopPositionViewPolling() {
      if (positionViewPollTimer) {
        window.clearInterval(positionViewPollTimer);
        positionViewPollTimer = null;
      }
      stopMissingPositionEntryTimeSync();
      positionViewPollBusy = false;
    }

    function startMissingPositionEntryTimeSync() {
      if (positionEntryTimeSyncTimer) return;
      positionEntryTimeSyncTimer = window.setInterval(() => {
        syncMissingPositionEntryTimes().catch((err) => toast(err.message));
      }, missingPositionEntrySyncIntervalMs);
    }

    function stopMissingPositionEntryTimeSync() {
      if (!positionEntryTimeSyncTimer) return;
      window.clearInterval(positionEntryTimeSyncTimer);
      positionEntryTimeSyncTimer = null;
      positionEntryTimeSyncBusy = false;
    }

    async function syncMissingPositionEntryTimes() {
      if (positionEntryTimeSyncBusy || positionViewPollBusy || !positionsTabActive() || !missingPositionEntryTimesDetected()) return;
      positionEntryTimeSyncBusy = true;
      positionViewPollBusy = true;
      try {
        await loadPositions(true);
      } finally {
        positionEntryTimeSyncBusy = false;
        positionViewPollBusy = false;
      }
    }

    function analysisTabActive() {
      const section = $("analysis");
      return !!(section && section.classList.contains("active"));
    }

    function analysisBalanceAutoRefreshAllowed() {
      return analysisTabActive() && !(document.hidden);
    }

    function startAnalysisBalanceAutoRefresh() {
      if (analysisBalanceRefreshTimer) return;
      analysisBalanceRefreshTimer = window.setInterval(() => {
        refreshAnalysisBalanceOverviewAuto();
      }, analysisBalanceRefreshIntervalMs);
    }

    function stopAnalysisBalanceAutoRefresh() {
      if (!analysisBalanceRefreshTimer) return;
      window.clearInterval(analysisBalanceRefreshTimer);
      analysisBalanceRefreshTimer = null;
      analysisBalanceRefreshBusy = false;
    }

    async function refreshAnalysisBalanceOverviewAuto() {
      if (analysisBalanceRefreshBusy || !analysisBalanceAutoRefreshAllowed()) return;
      analysisBalanceRefreshBusy = true;
      try {
        await loadBalanceOverview(true);
      } finally {
        analysisBalanceRefreshBusy = false;
      }
    }

    function syncOrderInfoVisibility(tabID) {
      const box = $("order-info-status");
      if (!box) return;
      box.hidden = tabID !== "orderSettings";
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
      syncOrderInfoVisibility(target);
      if (persist) {
        if (window.history && location.hash !== "#" + target) {
          history.replaceState(null, "", "#" + target);
        }
      }
      if (target === "positions") {
        startPositionViewPolling();
      } else {
        stopPositionViewPolling();
      }
      if (target === "analysis") {
        startAnalysisBalanceAutoRefresh();
      } else {
        stopAnalysisBalanceAutoRefresh();
      }
      if (target === "analysis" && !state.analysis) {
        loadAnalysis(false).catch((err) => toast(err.message));
      }
      if (target === "positions" && (!state.positions || !state.pendingOrders)) {
        loadPositionView().catch((err) => toast(err.message));
      }
      if (target === "symbols" && !state.symbols) {
        loadSymbols(true).catch((err) => toast(err.message));
      }
    }

    function initialTab() {
      const fromHash = location.hash ? location.hash.slice(1) : "";
      const hashButton = tabButton(fromHash);
      if (fromHash && hashButton && !hashButton.hidden && $(fromHash)) return fromHash;
      return effectiveDefaultTab();
    }

    async function loadAll() {
      await Promise.allSettled([loadConfig(), loadAPIKeys(), loadOrders(), loadUpgrade(), loadBalanceOverview()]);
      await loadAnalysis(false);
      renderDashboard();
    }

    async function loadConfig() {
      state.config = await api("/tvbot/config");
      applyMenuSettings();
      syncActiveTabAfterMenuSettings();
      renderConfig();
      renderOrderSettings();
      renderMenuSettings();
      renderPositions();
      renderPendingOrders();
      updateMetrics();
    }

    async function loadAPIKeys() {
      const [okxResult, binanceResult] = await Promise.allSettled([
        api("/tvbot/api-keys?exchange=okx"),
        api("/tvbot/api-keys?exchange=binance")
      ]);
      state.apiKeysByExchange.okx = okxResult.status === "fulfilled" ? okxResult.value : { configured: false, credentials: [], error: okxResult.reason ? okxResult.reason.message : "OKX API Key 读取失败" };
      state.apiKeysByExchange.binance = binanceResult.status === "fulfilled" ? binanceResult.value : { configured: false, credentials: [], error: binanceResult.reason ? binanceResult.reason.message : "Binance API Key 读取失败" };
      state.apiKeys = apiKeyStatus(state.apiKeyExchange);
      renderAPIKeys();
      renderTemplateAPIs();
      renderAnalysisAPIs();
      renderPositionAPIs();
      renderOrders();
      updateMetrics();
    }

    async function loadBalanceOverview(forceRefresh) {
      try {
        state.balanceOverview = await api(balanceOverviewPath(!!forceRefresh));
        state.balanceOverviewError = "";
      } catch (err) {
        state.balanceOverview = null;
        state.balanceOverviewError = err.message;
      }
      renderBalanceOverview();
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
      const qs = new URLSearchParams({ price_days: "3", pnl_minutes: String(analysisPNLWindowMinutes()) });
      const selected = $("analysis-api-id") ? $("analysis-api-id").value : "";
      if (selected) qs.set("api_id", selected);
      if (refresh) qs.set("refresh", "true");
      if (refresh) await loadBalanceOverview(true);
      try {
        state.analysis = await api("/tvbot/analysis?" + qs.toString());
        state.analysisError = "";
        state.analysisTradePage = 1;
      } catch (err) {
        state.analysis = null;
        state.analysisError = err.message;
        state.analysisTradePage = 1;
      }
      renderAnalysis();
    }

    async function loadPositions(forceRefresh) {
      const results = await Promise.all(positionExchanges.map((exchange) => loadPositionExchange(exchange, !!forceRefresh)));
      const rows = [];
      results.forEach((result) => {
        const exchange = normalizeExchange(result.exchange);
        const apiID = result.api_id || "";
        (Array.isArray(result.positions) ? result.positions : []).forEach((row) => {
          const view = Object.assign({}, row, { _exchange: exchange, _api_id: apiID });
          rememberSymbolPrecision(exchange, view);
          rows.push(view);
        });
      });
      state.positions = {
        ok: results.some((result) => result.ok),
        count: rows.length,
        refreshed_at: combinedRefreshedAt(results),
        exchanges: results,
        positions: rows
      };
      state.positionsError = combinedErrors(results);
      renderPositions();
    }

    async function loadPendingOrders() {
      const results = await Promise.all(positionExchanges.map((exchange) => loadPendingOrdersExchange(exchange)));
      const rows = [];
      let normalCount = 0;
      let algoCount = 0;
      results.forEach((result) => {
        const exchange = normalizeExchange(result.exchange);
        const apiID = result.api_id || "";
        (Array.isArray(result.orders) ? result.orders : []).forEach((row) => {
          const view = Object.assign({}, row, { _exchange: exchange, _api_id: apiID });
          rememberSymbolPrecision(exchange, view);
          rows.push(view);
        });
        normalCount += pendingOrderNormalCount(result);
        algoCount += pendingOrderAlgoCount(result);
      });
      state.pendingOrders = {
        ok: results.some((result) => result.ok),
        count: normalCount,
        normal_count: normalCount,
        algo_count: algoCount,
        total_count: normalCount + algoCount,
        refreshed_at: combinedRefreshedAt(results),
        exchanges: results,
        orders: rows
      };
      state.pendingOrdersError = combinedErrors(results);
      renderPendingOrders();
    }

    async function loadPositionExchange(exchange, forceRefresh) {
      const qs = new URLSearchParams({ inst_type: "SWAP", exchange });
      const apiID = activeAPIID(exchange);
      if (apiID) qs.set("api_id", apiID);
      if (forceRefresh) qs.set("refresh", "true");
      try {
        const result = await api("/tvbot/positions?" + qs.toString());
        result.exchange = normalizeExchange(result.exchange || exchange);
        return result;
      } catch (err) {
        return positionExchangeError(exchange, err);
      }
    }

    async function loadPendingOrdersExchange(exchange) {
      const qs = new URLSearchParams({ inst_type: "SWAP", exchange });
      const apiID = activeAPIID(exchange);
      if (apiID) qs.set("api_id", apiID);
      try {
        const result = await api("/tvbot/pending-orders?" + qs.toString());
        result.exchange = normalizeExchange(result.exchange || exchange);
        return result;
      } catch (err) {
        const result = positionExchangeError(exchange, err);
        result.orders = [];
        result.normal_count = 0;
        result.algo_count = 0;
        result.total_count = 0;
        return result;
      }
    }

    function positionExchangeError(exchange, err) {
      return {
        ok: false,
        exchange: normalizeExchange(exchange),
        api_id: "",
        count: 0,
        normal_count: 0,
        algo_count: 0,
        total_count: 0,
        refreshed_at: "",
        positions: [],
        error: err && err.message ? err.message : String(err || "读取失败")
      };
    }

    function combinedErrors(results) {
      return results
        .filter((result) => result && result.error)
        .map((result) => exchangeLabel(result.exchange) + ": " + result.error)
        .join(" / ");
    }

    function combinedRefreshedAt(results) {
      return results.reduce((latest, result) => {
        const value = result && result.refreshed_at ? new Date(result.refreshed_at).getTime() : 0;
        if (!Number.isFinite(value) || value <= 0) return latest;
        return value > latest ? value : latest;
      }, 0);
    }

    function combinedStatusText(response) {
      const exchanges = response && Array.isArray(response.exchanges) ? response.exchanges : [];
      if (exchanges.length === 0) return "-";
      return exchanges.map((result) => {
        const label = exchangeLabel(result.exchange);
        if (result && result.refreshed_at) return label + " " + shanghaiTime(result.refreshed_at);
        if (result && result.error) return label + " 失败";
        return label + " -";
      }).join(" / ");
    }

    function pendingOrderCountValue(result, key, fallback) {
      const value = Number(result && result[key]);
      if (Number.isFinite(value)) return Math.max(0, Math.trunc(value));
      return Math.max(0, Math.trunc(Number(fallback || 0)));
    }

    function pendingOrderNormalCount(result) {
      const orderCount = Array.isArray(result && result.orders) ? result.orders.length : 0;
      return pendingOrderCountValue(result, "normal_count", pendingOrderCountValue(result, "count", orderCount));
    }

    function pendingOrderAlgoCount(result) {
      return pendingOrderCountValue(result, "algo_count", 0);
    }

    function pendingOrdersSummaryText(response) {
      const exchanges = response && Array.isArray(response.exchanges) ? response.exchanges : [];
      if (exchanges.length === 0) return "-";
      return positionExchanges.map((exchange) => {
        const result = exchanges.find((item) => normalizeExchange(item && item.exchange) === exchange);
        const label = exchangeLabel(exchange);
        if (!result) return label + " -";
        if (!result.ok || result.error) return label + " 失败";
        if (exchange === "okx") return "OKX 普通单 " + pendingOrderNormalCount(result) + " / 算法订单 " + pendingOrderAlgoCount(result);
        if (exchange === "binance") return "Binance 普通单 " + pendingOrderNormalCount(result) + " / 算法单 " + pendingOrderAlgoCount(result);
        return label + " 普通单 " + pendingOrderNormalCount(result) + " / 算法单 " + pendingOrderAlgoCount(result);
      }).join(" . ");
    }

    async function loadPositionView(forceRefresh) {
      await Promise.all([loadPositions(!!forceRefresh), loadPendingOrders()]);
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
      $("metric-api-keys").textContent = "OKX " + apiMetricText("okx") + " / Binance " + apiMetricText("binance");
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
        ["OKX Base URL", t.base_url],
        ["Binance Base URL", t.binance_base_url],
        ["Binance Demo Base URL", t.binance_demo_base_url],
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
      renderBalanceOverview();
    }

    function renderBalanceOverview() {
      ["okx", "binance"].forEach((exchange) => {
        const item = balanceOverviewExchange(exchange);
        const prefix = "overview-" + exchange;
        const label = exchangeLabel(exchange);
        if (!item) {
          $(prefix + "-status").textContent = state.balanceOverviewError || "-";
          $(prefix + "-eq").textContent = "-";
          $(prefix + "-avail").textContent = "-";
          $(prefix + "-api").textContent = "-";
          $(prefix + "-updated").textContent = "-";
          drawUSDTChart([], prefix + "-usdt-chart", "暂无 " + label + " USDT 余额数据", exchange === "binance" ? "#138a55" : "#1f6feb");
          return;
        }
        const balance = item.balance || {};
        const usdt = usdtBalanceDetail(balance);
        const configured = !!item.configured;
        const statusText = configured ? (item.status === "ok" ? "已更新" : (item.error || item.status || "错误")) : "未配置";
        $(prefix + "-status").textContent = statusText;
        $(prefix + "-eq").textContent = usdt ? formatUSD(usdt.eq_usd || usdt.eq) : "-";
        $(prefix + "-avail").textContent = usdt ? formatUSDTBalance(usdt.avail_bal || usdt.avail_eq) + " USDT" : "-";
        $(prefix + "-api").textContent = item.api_id ? apiDisplayName(item.api_id, exchange) : "-";
        $(prefix + "-updated").textContent = balance.updated_at ? shanghaiTime(balance.updated_at) : (item.refreshed_at ? shanghaiTime(item.refreshed_at) : "-");
        const points = usdtBalancePoints(item.balance_points || [], balance);
        drawUSDTChart(points, prefix + "-usdt-chart", configured ? "暂无 " + label + " USDT 余额数据" : label + " 未配置", exchange === "binance" ? "#138a55" : "#1f6feb");
      });
      renderAnalysisExchangeBalances();
    }

    function balanceOverviewExchange(exchange) {
      const rows = state.balanceOverview && Array.isArray(state.balanceOverview.exchanges) ? state.balanceOverview.exchanges : [];
      return rows.find((row) => normalizeExchange(row.exchange) === normalizeExchange(exchange)) || null;
    }

    function renderConfig() {
      const cfg = state.config || {};
      const trading = cfg.trading || {};
      $("cfg-addr").value = cfg.server && cfg.server.addr ? cfg.server.addr : "";
      $("cfg-data-file").value = cfg.data_file || "";
      $("cfg-database-file").value = cfg.database_file || "";
      $("cfg-base-url").value = trading.base_url || "";
      $("cfg-binance-base-url").value = trading.binance_base_url || "";
      $("cfg-binance-demo-base-url").value = trading.binance_demo_base_url || "";
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
      const exchange = normalizeExchange(state.apiKeyExchange);
      const status = apiKeyStatus(exchange);
      state.apiKeys = status;
      const accounts = apiAccounts(exchange);
      const select = $("key-selected");
      const previous = state.selectedAPIIDs[exchange] || select.value || status.active_id || "";
      document.querySelectorAll("[data-api-key-exchange]").forEach((button) => {
        button.setAttribute("aria-selected", normalizeExchange(button.dataset.apiKeyExchange) === exchange ? "true" : "false");
      });
      $("key-api-label").childNodes[0].nodeValue = exchangeLabel(exchange) + " API Key";
      $("key-secret-label").childNodes[0].nodeValue = exchangeLabel(exchange) + " Secret Key";
      $("key-passphrase-label").style.display = exchange === "okx" ? "" : "none";
      select.innerHTML = accounts.map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>').join("");
      if (!accounts.length) {
        select.innerHTML = '<option value="default">default - 新 API</option>';
      }
      const selected = accounts.some((account) => account.id === previous) ? previous : (status.active_id || (accounts[0] && accounts[0].id) || "default");
      select.value = selected;
      state.selectedAPIID = selected;
      state.selectedAPIIDs[exchange] = selected;
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
      const exchange = normalizeExchange(state.apiKeyExchange);
      const status = apiKeyStatus(exchange);
      const rows = [
        ["交易所", exchangeLabel(exchange)],
        ["配置状态", status.configured ? "已配置" : "未配置"],
        ["交易 API", status.active_id || "-"],
        ["API Key", status.api_key_masked || "-"],
        ["Secret Key", status.secret_key_set ? "已保存" : "未保存"]
      ];
      if (exchange === "okx") rows.push(["Passphrase", status.passphrase_set ? "已保存" : "未保存"]);
      rows.push(["来源", status.source || "-"]);
      rows.push(["更新时间", status.updated_at || "-"]);
      if (status.error) rows.push(["读取错误", status.error]);
      const test = state.apiKeyTest;
      const testID = state.apiKeyTestID || (test && test.api_id) || "";
      if (test && state.apiKeyTestExchange === exchange && (!selected || !testID || selected === testID || test.api_id === "input")) {
        const balance = test.usdt_balance || {};
        rows.push(["测试 API", test.api_id || testID || "-"]);
        if (test.usdt_balance_found && balance) {
          rows.push(["USDT 总权益", balanceAmount(balance.eq)]);
          rows.push(["USDT 可用", balanceAmount(balance.avail_eq || balance.avail_bal)]);
          rows.push(["USDT 冻结", balanceAmount(balance.frozen_bal)]);
          rows.push(["余额更新时间", shanghaiTimeFromOKX(balance.u_time)]);
        } else {
          rows.push(["USDT 余额", exchangeLabel(exchange) + " 未返回 USDT 明细"]);
        }
      }
      $("api-key-status").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(row[1]) + "</td></tr>").join("");
    }

    function apiKeyStatus(exchange) {
      exchange = normalizeExchange(exchange);
      return state.apiKeysByExchange[exchange] || { configured: false, credentials: [] };
    }

    function apiMetricText(exchange) {
      const status = apiKeyStatus(exchange);
      return status && status.configured ? (status.active_id || "已配置") : "未配置";
    }

    function activeAPIID(exchange) {
      const status = apiKeyStatus(exchange);
      return status && status.configured && status.active_id ? status.active_id : "";
    }

    function apiAccounts(exchange) {
      const status = apiKeyStatus(exchange || state.apiKeyExchange);
      return status && Array.isArray(status.credentials) ? status.credentials : [];
    }

    function selectedAPIAccount(id, exchange) {
      return apiAccounts(exchange).find((account) => account.id === id) || null;
    }

    function apiDisplayName(id, exchange) {
      const apiID = String(id || "").trim();
      if (!apiID) return "-";
      const account = selectedAPIAccount(apiID, exchange) || selectedAPIAccount(apiID, "okx") || selectedAPIAccount(apiID, "binance");
      return account && account.name ? account.name : apiID;
    }

    function fillAPIForm(id) {
      const account = selectedAPIAccount(id, state.apiKeyExchange);
      $("key-id").value = account ? account.id : (id || "default");
      $("key-name").value = account ? (account.name || "") : "";
      $("key-active").checked = account ? !!account.active : true;
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
    }

    function renderTemplateAPIs() {
      const exchange = $("tpl-target-exchange") ? normalizeExchange($("tpl-target-exchange").value) : "okx";
      const status = apiKeyStatus(exchange);
      const options = apiAccounts(exchange).map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>');
      $("tpl-api-id").innerHTML = '<option value="">默认交易 API</option>' + options.join("");
      if (status && status.active_id) {
        $("tpl-api-id").value = status.active_id;
      }
    }

    function templateWebhookURL() {
      return new URL("/tvorder", window.location.origin).toString();
    }

    function renderTemplateWebhookURL() {
      if ($("template-webhook-url")) {
        $("template-webhook-url").value = templateWebhookURL();
      }
    }

    function renderAnalysisAPIs() {
      const select = $("analysis-api-id");
      const current = select.value;
      const status = apiKeyStatus("okx");
      const options = apiAccounts("okx").map((account) => '<option value="' + escapeHTML(account.id) + '">' + escapeHTML(account.id + (account.name ? " - " + account.name : "") + (account.active ? " (交易)" : "")) + '</option>');
      select.innerHTML = '<option value="">默认交易 API</option>' + options.join("");
      select.value = current || (status && status.active_id ? status.active_id : "");
    }

    function renderPositionAPIs() {
      const summary = $("position-exchange-summary");
      if (!summary) return;
      summary.textContent = positionExchanges.map((exchange) => {
        const status = apiKeyStatus(exchange);
        if (!status || !status.configured) return exchangeLabel(exchange) + " 未配置";
        const apiID = status && status.active_id ? status.active_id : "default";
        return exchangeLabel(exchange) + " " + apiDisplayName(apiID, exchange);
      }).join(" / ");
    }

    function positionSideText(posSide, pos) {
      const kind = positionSideKind(posSide, pos);
      if (kind === "long") return "多单";
      if (kind === "short") return "空单";
      if (kind === "net") return "持仓";
      return asText(posSide);
    }

    function positionSideKind(posSide, pos) {
      const side = String(posSide || "").toLowerCase();
      if (side === "long") return "long";
      if (side === "short") return "short";
      if (side === "net") {
        const value = Number(pos);
        if (Number.isFinite(value) && value < 0) return "short";
        if (Number.isFinite(value) && value > 0) return "long";
        return "net";
      }
      return "";
    }

    function tradeSideText(side) {
      const value = String(side || "").toLowerCase();
      if (value === "buy") return "买入";
      if (value === "sell") return "卖出";
      return asText(side);
    }

    function pendingOrderStateText(value) {
      const stateText = String(value || "").toLowerCase();
      if (stateText === "live") return "等待成交";
      if (stateText === "partially_filled") return "部分成交";
      return asText(value);
    }

    function pendingOrderRowKey(row) {
      const apiID = row._api_id || "";
      const exchange = row._exchange || "okx";
      const orderID = row.ordId || ("cl:" + (row.clOrdId || ""));
      return [normalizeExchange(exchange), apiID, String(row.instId || "").toUpperCase(), orderID].join("|");
    }

    function pendingOrderActionCell(row) {
      const exchange = normalizeExchange(row._exchange || "okx");
      const key = pendingOrderRowKey(row);
      const busy = !!state.pendingOrderActions[key];
      const chasing = !!row.chasing;
      const unavailable = !chasing && !!row.price_error;
      const disabled = busy;
      const label = busy ? "处理中" : (chasing ? "停止追单" : "追单");
      const mode = chasing ? "stop" : "start";
      const chaseButton = '<button class="btn small' + (unavailable ? " is-disabled" : "") + '" type="button" data-pending-chase="' + mode + '"' +
        ' data-exchange="' + exchange + '"' +
        ' data-api-id="' + escapeHTML(row._api_id || "") + '"' +
        ' data-inst-id="' + escapeHTML(asText(row.instId)) + '"' +
        ' data-ord-id="' + escapeHTML(row.ordId || "") + '"' +
        ' data-cl-ord-id="' + escapeHTML(row.clOrdId || "") + '"' +
        ' data-price-error="' + escapeHTML(row.price_error || "") + '"' +
        (unavailable ? ' aria-disabled="true"' : "") +
        ' title="' + escapeHTML(row.price_error || label) + '"' +
        (disabled ? " disabled" : "") + ">" + label + "</button>";
      const cancelButton = exchange === "binance" ? '<button class="btn small danger" type="button" data-pending-cancel="true"' +
        ' data-exchange="' + exchange + '"' +
        ' data-api-id="' + escapeHTML(row._api_id || "") + '"' +
        ' data-inst-id="' + escapeHTML(asText(row.instId)) + '"' +
        ' data-ord-id="' + escapeHTML(row.ordId || "") + '"' +
        ' data-cl-ord-id="' + escapeHTML(row.clOrdId || "") + '"' +
        ' title="取消 Binance 挂单"' +
        (disabled ? " disabled" : "") + ">取消</button>" : "";
      return '<td><div class="position-actions">' + chaseButton + cancelButton + "</div></td>";
    }

    function positionPercent(v) {
      if (v === null || v === undefined || v === "") return "-";
      const formatted = formatPct(v);
      return formatted === "-" ? asText(v) : formatted;
    }

    function positionNumber(value) {
      if (value === null || value === undefined) return null;
      const text = String(value).trim();
      if (!text || text === "-") return null;
      const number = Number(text.replace(/,/g, ""));
      return Number.isFinite(number) ? number : null;
    }

    function positionReturnRatio(row) {
      const upl = positionNumber(row ? row.upl : null);
      const margin = positionNumber(row ? row.margin : null);
      if (upl !== null && margin !== null && margin !== 0) return upl / Math.abs(margin);
      return positionNumber(row ? row.uplRatio : null);
    }

    function positionReturnPercent(row) {
      const ratio = positionReturnRatio(row);
      return ratio === null ? positionPercent(row ? row.uplRatio : null) : formatPct(ratio);
    }

    function positionSum(rows, field) {
      return rows.reduce((sum, row) => {
        const value = Number(row[field]);
        return Number.isFinite(value) ? sum + value : sum;
      }, 0);
    }

    function positionAmount(row) {
      const notionalRaw = row ? row.notionalUsd : null;
      if (notionalRaw !== null && notionalRaw !== undefined) {
        const notionalText = String(notionalRaw).trim();
        if (notionalText && notionalText !== "-") {
          const notional = positionNumber(notionalText);
          if (Number.isFinite(notional) && notional !== 0) return formatNumber(Math.abs(notional));
        }
      }
      const marginRaw = row ? row.margin : null;
      const leverRaw = row ? row.lever : null;
      if (marginRaw === null || marginRaw === undefined || leverRaw === null || leverRaw === undefined) return "-";
      const marginText = String(marginRaw).trim();
      const leverText = String(leverRaw).trim();
      if (!marginText || marginText === "-" || !leverText || leverText === "-") return "-";
      const margin = positionNumber(marginText);
      const lever = positionNumber(leverText);
      if (!Number.isFinite(margin) || !Number.isFinite(lever)) return "-";
      return formatNumber(margin * lever);
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

    function positionSideCell(row) {
      const kind = positionSideKind(row ? row.posSide : "", row ? row.pos : "");
      const tone = kind === "long" ? "signed-profit" : (kind === "short" ? "signed-loss" : "");
      return '<td' + (tone ? ' class="' + tone + '"' : "") + ">" + escapeHTML(positionSideText(row ? row.posSide : "", row ? row.pos : "")) + "</td>";
    }

    function positionCloseRowKey(row) {
      return [normalizeExchange(row._exchange || "okx"), row._api_id || "", String(row.instId || "").toUpperCase(), String(row.posSide || "net").toLowerCase()].join("|");
    }

    function positionActionCell(row) {
      const exchange = normalizeExchange(row._exchange || "okx");
      const key = positionCloseRowKey(row);
      const closing = !!state.positionClosing[key];
      const instID = escapeHTML(asText(row.instId));
      const posSide = escapeHTML(String(row.posSide || ""));
      const apiID = escapeHTML(row._api_id || "");
      const baseAttrs = ' data-exchange="' + exchange + '" data-api-id="' + apiID + '" data-inst-id="' + instID + '" data-pos-side="' + posSide + '"';
      const protectionButtons = [
        { label: "止盈", kind: "tp" },
        { label: "止损", kind: "sl" },
        { label: "移动", kind: "trailing" }
      ].map((item) => {
        const protecting = !!state.positionProtecting[key + "|" + item.kind];
        return '<button class="btn small position-protection-btn" type="button" data-position-protection="' + item.kind + '"' + baseAttrs + (protecting ? " disabled" : "") + '>' + item.label + '</button>';
      }).join("");
      const percentButtons = [
        { label: "10%", ratio: "0.1" },
        { label: "25%", ratio: "0.25" },
        { label: "50%", ratio: "0.5" },
        { label: "75%", ratio: "0.75" }
      ].map((item) => '<button class="btn small position-percent-close-btn" type="button" data-position-close="limit" data-position-ratio="' + item.ratio + '"' + baseAttrs + ' title="限价平仓 ' + item.label + '，60秒后市价兜底"' + (closing ? " disabled" : "") + '>' + item.label + '</button>').join("");
      return '<td><div class="position-actions">' +
        protectionButtons +
        percentButtons +
        '<button class="btn small danger" type="button" data-position-close="market"' + baseAttrs + (closing ? " disabled" : "") + '>市价平仓</button>' +
        '<button class="btn small" type="button" data-position-close="limit"' + baseAttrs + (closing ? " disabled" : "") + '>限价平仓</button>' +
        '</div></td>';
    }

    function positionEntryTimeTitle(row) {
      const parts = [];
      if (row && row.entry_time_source) parts.push(entryTimeSourceText(row.entry_time_source));
      if (row && row.entry_time_error) parts.push(row.entry_time_error);
      return parts.join(" / ");
    }

    function positionEntryTimeCell(row) {
      const title = positionEntryTimeTitle(row);
      const attr = title ? ' title="' + escapeHTML(title) + '"' : "";
      const text = row && row.entry_fill_time ? shanghaiTime(row.entry_fill_time) : "-";
      return '<td class="time"' + attr + '>' + escapeHTML(text) + '</td>';
    }

    function positionHoldingTimeCell(row) {
      const title = positionEntryTimeTitle(row);
      const attr = title ? ' title="' + escapeHTML(title) + '"' : "";
      const text = row && row.entry_fill_time ? formatHoldingSeconds(row.holding_seconds) : "-";
      return '<td class="time"' + attr + '>' + escapeHTML(text) + '</td>';
    }

    function renderPositions() {
      renderTableStructure("positions");
      const columnDefs = currentTableColumnDefs("positions");
      const colspan = tableColumnCount("positions");
      const rows = state.positions && Array.isArray(state.positions.positions) ? state.positions.positions : [];
      const totalUpl = positionSum(rows, "upl");
      const positionsReady = state.positions && state.positions.ok;
      $("positions-count").textContent = positionsReady ? asText(state.positions.count || rows.length) : "-";
      $("positions-upl").textContent = positionsReady ? formatNumber(totalUpl) + " USDT" : "-";
      $("positions-upl").className = ["value", positionsReady ? signedToneClass(totalUpl) : ""].filter(Boolean).join(" ");
      $("positions-notional").textContent = positionsReady ? formatUSD(positionSum(rows, "notionalUsd")) : "-";
      $("positions-updated").textContent = state.positions ? combinedStatusText(state.positions) : "-";
      if (!state.positions) {
        $("position-rows").innerHTML = '<tr><td colspan="' + colspan + '" class="muted">' + escapeHTML(state.positionsError || "-") + '</td></tr>';
        return;
      }
      const positionRows = rows.map((row) => {
        return "<tr>" + columnDefs.map((col) => col.cell(row)).join("") + "</tr>";
      }).join("");
      const warningRow = state.positionsError ? '<tr><td colspan="' + colspan + '" class="muted">' + escapeHTML(state.positionsError) + '</td></tr>' : "";
      $("position-rows").innerHTML = positionRows + warningRow || '<tr><td colspan="' + colspan + '" class="muted">暂无当前持仓</td></tr>';
    }

    function renderPendingOrders() {
      renderTableStructure("pending_orders");
      const columnDefs = currentTableColumnDefs("pending_orders");
      const colspan = tableColumnCount("pending_orders");
      const rows = state.pendingOrders && Array.isArray(state.pendingOrders.orders) ? state.pendingOrders.orders : [];
      if (!state.pendingOrders) {
        $("pending-orders-updated").textContent = state.pendingOrdersError || "-";
        $("pending-order-rows").innerHTML = '<tr><td colspan="' + colspan + '" class="muted">' + escapeHTML(state.pendingOrdersError || "-") + '</td></tr>';
        return;
      }
      const pendingReady = state.pendingOrders && state.pendingOrders.ok;
      $("pending-orders-updated").textContent = pendingReady ? pendingOrdersSummaryText(state.pendingOrders) : state.pendingOrdersError || "-";
      const orderRows = rows.map((row) => {
        return "<tr>" + columnDefs.map((col) => col.cell(row)).join("") + "</tr>";
      }).join("");
      const warningRow = state.pendingOrdersError ? '<tr><td colspan="' + colspan + '" class="muted">' + escapeHTML(state.pendingOrdersError) + '</td></tr>' : "";
      $("pending-order-rows").innerHTML = orderRows + warningRow || '<tr><td colspan="' + colspan + '" class="muted">暂无当前挂单</td></tr>';
    }

    function renderAnalysis() {
      if (!state.analysis) {
        $("analysis-updated").textContent = state.analysisError || "-";
        renderAnalysisExchangeBalances();
        renderAnalysisExchangeStats("okx", state.analysisError || "-");
        renderAnalysisExchangeStats("binance", state.analysisError || "-");
        renderAnalysisTradeHistory(state.analysisError || "-");
        return;
      }
      renderAnalysisExchangeBalances();
      const okxAPI = asText(state.analysis.api_id);
      const binanceAPI = asText(state.analysis.binance_api_id);
      const apiText = binanceAPI === "-" ? "OKX " + okxAPI : "OKX " + okxAPI + " / Binance " + binanceAPI;
      $("analysis-updated").textContent = "订单统计 " + shanghaiTime(state.analysis.refreshed_at) + " / API " + apiText;
      renderAnalysisExchangeStats("okx", "");
      renderAnalysisExchangeStats("binance", "");
      renderAnalysisTradeHistory("");
    }

    function renderAnalysisExchangeStats(exchange, errorText) {
      const normalized = normalizeExchange(exchange);
      const prefix = normalized === "binance" ? "analysis-binance" : "analysis-okx";
      if (errorText) {
        $(prefix + "-net-pnl").textContent = "-";
        $(prefix + "-win-rate").textContent = "-";
        $(prefix + "-profit-factor").textContent = "-";
        $(prefix + "-payoff-ratio").textContent = "-";
        $(prefix + "-trades").textContent = "-";
        $(prefix + "-symbol-status").textContent = errorText;
        $(prefix + "-rows").innerHTML = '<tr><td colspan="9" class="muted">' + escapeHTML(errorText) + '</td></tr>';
        return;
      }
      const symbols = (state.analysis && Array.isArray(state.analysis.symbols) ? state.analysis.symbols : []).filter((row) => normalizeExchange(row.exchange) === normalized);
      const summary = analysisExchangeSummary(normalized);
      $(prefix + "-net-pnl").textContent = formatNumber(summary.net_pnl) + " USDT";
      $(prefix + "-win-rate").textContent = formatPct(summary.win_rate);
      $(prefix + "-profit-factor").textContent = formatFactor(summary);
      $(prefix + "-payoff-ratio").textContent = formatNumber(summary.payoff_ratio);
      $(prefix + "-trades").textContent = asText(summary.trade_count || 0);
      $(prefix + "-symbol-status").textContent = "成交数 " + asText(summary.trade_count || 0) + " / 币对 " + symbols.length;
      const rows = symbols.map((row) => {
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
      $(prefix + "-rows").innerHTML = rows.join("") || '<tr><td colspan="9" class="muted">暂无 ' + escapeHTML(exchangeLabel(normalized)) + ' 成交统计</td></tr>';
    }

    function analysisExchangeSummary(exchange) {
      const summaries = state.analysis && Array.isArray(state.analysis.exchange_summaries) ? state.analysis.exchange_summaries : [];
      const found = summaries.find((row) => normalizeExchange(row.exchange) === normalizeExchange(exchange));
      return found || { exchange: normalizeExchange(exchange), trade_count: 0, wins: 0, losses: 0, net_pnl: 0, fees: 0, win_rate: 0, profit_factor: 0, payoff_ratio: 0 };
    }

    function renderAnalysisTradeHistory(errorText) {
      if (errorText) {
        $("analysis-trade-page-info").textContent = "-";
        $("analysis-trade-prev").disabled = true;
        $("analysis-trade-next").disabled = true;
        $("analysis-trade-rows").innerHTML = '<tr><td colspan="10" class="muted">' + escapeHTML(errorText) + '</td></tr>';
        return;
      }
      const trades = state.analysis && Array.isArray(state.analysis.trades) ? state.analysis.trades : [];
      const total = trades.length;
      const totalPages = Math.max(1, Math.ceil(total / analysisTradePageSize));
      const currentPage = Math.min(totalPages, Math.max(1, Math.floor(Number(state.analysisTradePage || 1))));
      state.analysisTradePage = currentPage;
      const start = total ? (currentPage - 1) * analysisTradePageSize : 0;
      const end = total ? Math.min(total, start + analysisTradePageSize) : 0;
      $("analysis-trade-page-info").textContent = total ? ((start + 1) + "-" + end + " / " + total) : "0 / 0";
      $("analysis-trade-prev").disabled = currentPage <= 1 || total === 0;
      $("analysis-trade-next").disabled = currentPage >= totalPages || total === 0;
      const tradeRows = trades.slice(start, end).map((row) => {
        const feeText = formatNumber(row.fee) + (row.fee_ccy ? " " + row.fee_ccy : "");
        return "<tr>" +
          "<td>" + escapeHTML(exchangeLabel(row.exchange)) + "</td>" +
          '<td class="time">' + escapeHTML(shanghaiTime(row.fill_time)) + "</td>" +
          "<td>" + escapeHTML(asText(row.inst_id)) + "</td>" +
          "<td>" + escapeHTML(tradeSideText(row.side)) + "</td>" +
          "<td>" + escapeHTML(formatNumber(row.fill_px)) + "</td>" +
          "<td>" + escapeHTML(formatAssetAmount(row.fill_sz)) + "</td>" +
          signedCell(row.fill_pnl, formatNumber(row.fill_pnl)) +
          signedCell(row.fee, feeText) +
          "<td>" + escapeHTML(asText(row.ord_id)) + "</td>" +
          "<td>" + escapeHTML(analysisTradeIDText(row)) + "</td>" +
          "</tr>";
      });
      $("analysis-trade-rows").innerHTML = tradeRows.join("") || '<tr><td colspan="10" class="muted">暂无成交历史</td></tr>';
    }

    function analysisTradeIDText(row) {
      const text = asText(row ? row.trade_id : "");
      const fillCount = Number(row && row.fill_count ? row.fill_count : 0);
      if (fillCount > 1) return text + " +" + (fillCount - 1);
      return text;
    }

    function renderAnalysisExchangeBalances() {
      const okxItem = balanceOverviewExchange("okx");
      const okxBalance = okxItem && okxItem.balance ? okxItem.balance : (state.analysis ? state.analysis.balance : null);
      renderAnalysisOKXBalance(okxItem, okxBalance);
      renderAnalysisBinanceBalance(balanceOverviewExchange("binance"));
    }

    function renderAnalysisOKXBalance(item, balance) {
      const details = balance && Array.isArray(balance.details) ? balance.details : [];
      const usdt = usdtBalanceDetail(balance);
      $("analysis-usdt-eq").textContent = usdt ? formatUSD(usdt.eq_usd || usdt.eq) : "-";
      $("analysis-balance-updated").textContent = balance && balance.updated_at ? shanghaiTime(balance.updated_at) : "-";
      $("analysis-okx-balance-status").textContent = exchangeBalanceStatusText(item, balance, "OKX");
      $("analysis-okx-usdt-title").textContent = "USDT余额 " + balanceWindowLabel(state.balanceWindowMinutes);
      $("analysis-balance-rows").innerHTML = balanceRowsHTML(details, "暂无 USDT 资产余额");
      const overviewPoints = item ? (item.balance_points || []) : [];
      const fallbackPoints = state.analysis ? (state.analysis.balance_points || []) : [];
      const points = usdtBalancePoints(overviewPoints.length ? overviewPoints : fallbackPoints, balance);
      drawUSDTChart(points, "usdt-chart", "暂无 OKX USDT 余额数据", "#1f6feb");
    }

    function renderAnalysisBinanceBalance(item) {
      const balance = item && item.balance ? item.balance : null;
      const details = balance && Array.isArray(balance.details) ? balance.details : [];
      const usdt = usdtBalanceDetail(balance);
      $("analysis-binance-usdt-eq").textContent = usdt ? formatUSD(usdt.eq_usd || usdt.eq) : "-";
      $("analysis-binance-balance-updated").textContent = balance && balance.updated_at ? shanghaiTime(balance.updated_at) : (item && item.refreshed_at ? shanghaiTime(item.refreshed_at) : "-");
      $("analysis-binance-balance-status").textContent = exchangeBalanceStatusText(item, balance, "Binance");
      $("analysis-binance-usdt-title").textContent = "USDT余额 " + balanceWindowLabel(state.balanceWindowMinutes);
      $("analysis-binance-balance-rows").innerHTML = balanceRowsHTML(details, "暂无 USDT 资产余额");
      const points = usdtBalancePoints(item ? (item.balance_points || []) : [], balance);
      drawUSDTChart(points, "analysis-binance-usdt-chart", item && item.configured ? "暂无 Binance USDT 余额数据" : "Binance 未配置", "#138a55");
    }

    function exchangeBalanceStatusText(item, balance, label) {
      if (!item) {
        if (state.balanceOverviewError) return state.balanceOverviewError;
        return balance ? "已更新" : "-";
      }
      if (!item.configured) return label + " 未配置";
      if (item.status === "ok") return "已更新";
      return item.error || item.status || "读取失败";
    }

    function balanceRowsHTML(details, emptyText) {
      const rows = (Array.isArray(details) ? details : []).filter((row) => String(row.ccy || "").toUpperCase() === "USDT").map((row) => {
        return "<tr>" +
          "<td>" + escapeHTML(asText(row.ccy)) + "</td>" +
          "<td>" + escapeHTML(formatUSDTBalance(row.eq)) + "</td>" +
          "<td>" + escapeHTML(formatUSDTBalance(row.avail_bal || row.avail_eq)) + "</td>" +
          "<td>" + escapeHTML(formatUSDTBalance(row.cash_bal)) + "</td>" +
          "<td>" + escapeHTML(formatUSDTBalance(row.frozen_bal)) + "</td>" +
          "</tr>";
      });
      return rows.join("") || '<tr><td colspan="5" class="muted">' + escapeHTML(emptyText) + '</td></tr>';
    }

    function drawUSDTChart(points, svgID, emptyText, strokeColor) {
      const svg = $(svgID || "usdt-chart");
      if (!svg) return;
      const lineColor = strokeColor || "#1f6feb";
      const rect = svg.getBoundingClientRect();
      const parentWidth = svg.parentElement ? svg.parentElement.clientWidth : 0;
      const isMini = svg.classList.contains("mini-usdt-chart");
      const isAnalysisChart = !!(svg.closest && svg.closest("#analysis"));
      const miniMinHeight = isAnalysisChart ? 360 : 240;
      const miniFallbackHeight = isAnalysisChart ? 360 : 250;
      const width = Math.max(isMini ? 520 : 900, Math.floor(rect.width || parentWidth || (window.innerWidth - 72) || 900));
      const height = Math.max(isMini ? miniMinHeight : 320, Math.floor(rect.height || svg.clientHeight || (isMini ? miniFallbackHeight : 420)));
      const pad = { left: 64, right: 24, top: 18, bottom: 58 };
      const plotWidth = width - pad.left - pad.right;
      const plotHeight = height - pad.top - pad.bottom;
      const plotBottom = height - pad.bottom;
      svg.setAttribute("viewBox", "0 0 " + width + " " + height);
      svg.innerHTML = "";
      if (!points.length) {
        svg.innerHTML = '<text x="' + (width / 2) + '" y="' + (height / 2) + '" text-anchor="middle" fill="#647089">' + escapeHTML(emptyText || "暂无 USDT余额数据") + '</text>';
        return;
      }
      const chartPoints = points.map((point, index) => {
        return { point: point, index: index, value: Number(point.value), date: chartPointDate(point) };
      }).filter((point) => Number.isFinite(point.value));
      if (!chartPoints.length) {
        svg.innerHTML = '<text x="' + (width / 2) + '" y="' + (height / 2) + '" text-anchor="middle" fill="#647089">' + escapeHTML(emptyText || "暂无 USDT余额数据") + '</text>';
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
        '<path d="' + path + '" fill="none" stroke="' + lineColor + '" stroke-width="2.4"/>' +
        '<circle cx="' + x(last).toFixed(2) + '" cy="' + y(last.value).toFixed(2) + '" r="4" fill="' + lineColor + '"/>';
    }

    function renderOrders() {
      const rows = (state.orders || []).map((order, index) => {
        const targetExchange = normalizeExchange(order.target_exchange || (order.result && order.result.target_exchange));
        const precisionInstID = order.result && order.result.inst_id ? order.result.inst_id : order.coinpair;
        const okxResult = targetExchange === "okx" && order.result && (order.result.ord_id || order.result.okx_code) ? [order.result.ord_id, order.result.okx_code].filter(Boolean).join(" / ") : "";
        const binanceResult = targetExchange === "binance" && order.result && (order.result.ord_id || order.result.binance_code || order.result.binance_msg) ? [order.result.ord_id, order.result.binance_code, order.result.binance_msg].filter(Boolean).join(" / ") : "";
        const errorText = [order.error_code, order.error].filter(Boolean).join(": ");
        const exchangeResult = okxResult || binanceResult || errorText || "-";
        const apiID = order.api_id || (order.result && order.result.api_id);
        const sourceExchange = order.source_exchange || "-";
        const targetText = exchangeLabel(targetExchange) + " / " + apiDisplayName(apiID, targetExchange);
        const tone = order.status === "submitted" ? "ok" : (order.status === "failed" || order.status === "rejected" ? "bad" : "warn");
        const canRetry = order.status === "failed" && order.signal_id;
        const retrying = canRetry && state.retrying[order.signal_id];
        const retryButton = canRetry ? '<button class="btn small" type="button" data-retry-id="' + escapeHTML(order.signal_id) + '"' + (retrying ? " disabled" : "") + ">" + (retrying ? "重试中" : "重试") + "</button>" : "";
        const rawJSONButton = orderRawJSONText(order) ? '<button class="btn small order-json-button" type="button" data-order-json-index="' + index + '">JSON</button>' : "";
        const statusCell = '<div class="status-cell">' + pill(order.status, tone) + rawJSONButton + "</div>";
        return "<tr>" +
          '<td class="time">' + escapeHTML(shanghaiTime(order.accepted_at)) + "</td>" +
          "<td>" + statusCell + "</td>" +
          "<td>" + escapeHTML(sourceExchange) + "</td>" +
          "<td>" + escapeHTML(targetText) + "</td>" +
          "<td>" + escapeHTML(asText(order.action)) + "</td>" +
          "<td>" + escapeHTML(asText(order.coinpair)) + "</td>" +
          "<td>" + escapeHTML(formatCachedSymbolPrice(targetExchange, precisionInstID, order.price)) + "</td>" +
          "<td>" + escapeHTML(asText(order.amount)) + "</td>" +
          '<td class="order-okx"><div class="okx-cell"><span class="okx-text">' + escapeHTML(exchangeResult) + "</span>" + retryButton + "</div></td>" +
          "</tr>";
      });
      $("order-rows").innerHTML = rows.join("") || '<tr><td colspan="9" class="muted">-</td></tr>';
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
          binance_base_url: $("cfg-binance-base-url").value.trim(),
          binance_demo_base_url: $("cfg-binance-demo-base-url").value.trim(),
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
      const patch = { ui: { default_tab: configuredDefaultTab(), menu_items: currentMenuItems() } };
      state.config = await api("/tvbot/config", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
      applyMenuSettings();
      syncActiveTabAfterMenuSettings();
      renderMenuSettings();
      toast("菜单设置已保存");
    }

    async function saveAPIKeys() {
      const exchange = normalizeExchange(state.apiKeyExchange);
      const body = {
        exchange: exchange,
        id: $("key-id").value.trim(),
        name: $("key-name").value.trim(),
        api_key: $("key-api").value.trim(),
        secret_key: $("key-secret").value.trim(),
        passphrase: exchange === "okx" ? $("key-passphrase").value.trim() : "",
        active: $("key-active").checked
      };
      state.apiKeysByExchange[exchange] = await api("/tvbot/api-keys?exchange=" + encodeURIComponent(exchange), { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      state.apiKeys = apiKeyStatus(exchange);
      state.selectedAPIID = body.id || state.apiKeys.active_id || "default";
      state.selectedAPIIDs[exchange] = state.selectedAPIID;
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      state.apiKeyTestExchange = exchange;
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
      const exchange = normalizeExchange(state.apiKeyExchange);
      const body = {
        exchange: exchange,
        id: $("key-id").value.trim(),
        api_key: $("key-api").value.trim(),
        secret_key: $("key-secret").value.trim(),
        passphrase: exchange === "okx" ? $("key-passphrase").value.trim() : ""
      };
      $("okx-output").textContent = "checking...";
      const result = await api("/tvbot/api-keys/test", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      state.apiKeyTest = result;
      state.apiKeyTestID = result.api_id || body.id || $("key-selected").value || "";
      state.apiKeyTestExchange = exchange;
      $("okx-output").textContent = JSON.stringify(result, null, 2);
      renderAPIKeyStatus(body.id || state.selectedAPIID || result.api_id || "");
      toast(exchangeLabel(exchange) + " API 可用");
    }

    async function deleteAPIKey() {
      const exchange = normalizeExchange(state.apiKeyExchange);
      const id = $("key-id").value.trim() || $("key-selected").value;
      if (!id) return;
      state.apiKeysByExchange[exchange] = await api("/tvbot/api-keys?exchange=" + encodeURIComponent(exchange) + "&id=" + encodeURIComponent(id), { method: "DELETE" });
      state.apiKeys = apiKeyStatus(exchange);
      state.selectedAPIID = state.apiKeys.active_id || "";
      state.selectedAPIIDs[exchange] = state.selectedAPIID;
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      state.apiKeyTestExchange = exchange;
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
        target_exchange: normalizeExchange($("tpl-target-exchange").value),
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
      const exchange = normalizeExchange(button.dataset.exchange || "okx");
      const mode = button.dataset.positionClose;
      const instID = button.dataset.instId || "";
      const posSide = button.dataset.posSide || "";
      const apiID = button.dataset.apiId || "";
      const ratio = Number(button.dataset.positionRatio || "0");
      const key = [exchange, apiID, instID.toUpperCase(), (posSide || "net").toLowerCase()].join("|");
      if (!instID || !mode || state.positionClosing[key]) return;
      state.positionClosing[key] = true;
      renderPositions();
      try {
        const body = {
          exchange,
          api_id: apiID,
          inst_id: instID,
          pos_side: posSide,
          mode
        };
        if (Number.isFinite(ratio) && ratio > 0 && ratio < 1) body.ratio = ratio;
        const result = await api("/tvbot/positions/close", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body)
        });
        const limitCloseText = "限价平仓已启动 ";
        const ratioText = body.ratio ? " " + Math.round(body.ratio * 100) + "%" : "";
        toast(mode === "market" ? "市价平仓已提交" + ratioText : limitCloseText + asText(result.px) + ratioText);
        await loadPositionView();
        window.setTimeout(() => loadPositionView().catch((err) => toast(err.message)), mode === "market" ? 1600 : 5200);
      } finally {
        delete state.positionClosing[key];
        renderPositions();
      }
    }

    async function protectPosition(button) {
      const exchange = normalizeExchange(button.dataset.exchange || "okx");
      const kind = button.dataset.positionProtection;
      const instID = button.dataset.instId || "";
      const posSide = button.dataset.posSide || "";
      const apiID = button.dataset.apiId || "";
      const key = [exchange, apiID, instID.toUpperCase(), (posSide || "net").toLowerCase(), kind].join("|");
      if (!instID || !kind || state.positionProtecting[key]) return;
      state.positionProtecting[key] = true;
      renderPositions();
      try {
        const result = await api("/tvbot/positions/protection", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            exchange,
            api_id: apiID,
            inst_id: instID,
            pos_side: posSide,
            kind
          })
        });
        const labels = { tp: "止盈单已提交", sl: "止损单已提交", trailing: "移动止损已提交" };
        const detail = result.trigger_px ? " " + asText(result.trigger_px) : (result.callback_ratio ? " " + asText(result.callback_ratio) : "");
        toast((labels[kind] || "保护单已提交") + detail);
        await loadPositionView();
        window.setTimeout(() => loadPositionView().catch((err) => toast(err.message)), 1600);
      } finally {
        delete state.positionProtecting[key];
        renderPositions();
      }
    }

    async function chasePendingOrder(button) {
      const exchange = normalizeExchange(button.dataset.exchange || "okx");
      const mode = button.dataset.pendingChase;
      const priceError = button.dataset.priceError || "";
      if (mode === "start" && priceError) {
        toast(priceError);
        return;
      }
      const apiID = button.dataset.apiId || "";
      const body = {
        exchange: exchange,
        api_id: apiID,
        inst_id: button.dataset.instId || "",
        ord_id: button.dataset.ordId || "",
        cl_ord_id: button.dataset.clOrdId || ""
      };
      const key = [exchange, body.api_id, String(body.inst_id || "").toUpperCase(), body.ord_id || ("cl:" + body.cl_ord_id)].join("|");
      if (!body.inst_id || (!body.ord_id && !body.cl_ord_id) || state.pendingOrderActions[key]) return;
      state.pendingOrderActions[key] = true;
      renderPendingOrders();
      try {
        const path = mode === "stop" ? "/tvbot/pending-orders/chase/stop" : "/tvbot/pending-orders/chase";
        const result = await api(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        toast(mode === "stop" ? "追单已停止" : ("追单已启动，60秒未成交将转市价 " + asText(result.px)));
        await loadPendingOrders();
      } finally {
        delete state.pendingOrderActions[key];
        renderPendingOrders();
      }
    }

    async function cancelPendingOrder(button) {
      const exchange = normalizeExchange(button.dataset.exchange || "okx");
      if (exchange !== "binance") {
        toast("仅支持取消 Binance 挂单");
        return;
      }
      const body = {
        exchange: exchange,
        api_id: button.dataset.apiId || "",
        inst_id: button.dataset.instId || "",
        ord_id: button.dataset.ordId || "",
        cl_ord_id: button.dataset.clOrdId || ""
      };
      const key = [exchange, body.api_id, String(body.inst_id || "").toUpperCase(), body.ord_id || ("cl:" + body.cl_ord_id)].join("|");
      if (!body.inst_id || (!body.ord_id && !body.cl_ord_id) || state.pendingOrderActions[key]) return;
      state.pendingOrderActions[key] = true;
      renderPendingOrders();
      try {
        const result = await api("/tvbot/pending-orders/cancel", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        toast(result.status === "finished" ? "挂单已不存在" : "Binance 挂单已取消");
        await loadPendingOrders();
      } finally {
        delete state.pendingOrderActions[key];
        renderPendingOrders();
      }
    }

    async function checkOKX() {
      $("okx-output").textContent = "checking...";
      try {
        const exchange = normalizeExchange(state.apiKeyExchange);
        const status = apiKeyStatus(exchange);
        const body = status && status.active_id ? { exchange: exchange, api_id: status.active_id } : { exchange: exchange };
        const result = await api("/tvbot/check-okx", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        $("okx-output").textContent = JSON.stringify(result, null, 2);
      } catch (err) {
        $("okx-output").textContent = err.message;
      }
    }

    async function setBalanceWindow(minutes) {
      const parsed = Math.max(0, Number(minutes || 0));
      if (!Number.isFinite(parsed)) return;
      state.balanceWindowMinutes = parsed;
      updateBalanceWindowButtons();
      await loadBalanceOverview(true);
      await loadAnalysis(false);
      toast("周期已切换为 " + balanceWindowLabel(parsed));
    }

    async function resetBalanceBaseline(button) {
      const original = button ? button.textContent : "";
      if (button) {
        button.disabled = true;
        button.textContent = "重置中";
      }
      try {
        state.balanceWindowMinutes = 0;
        updateBalanceWindowButtons();
        await loadBalanceOverview(true);
        await loadAnalysis(false);
        toast("基准已重置为当前余额");
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = original || "重置基准";
        }
      }
    }

    async function syncBalanceHistory(button) {
      const original = button ? button.textContent : "";
      if (button) {
        button.disabled = true;
        button.textContent = "同步中";
      }
      try {
        await loadBalanceOverview(true);
        await loadAnalysis(false);
        toast("余额历史已同步");
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = original || "同步历史";
        }
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
    $("menu-settings-rows").addEventListener("input", (event) => {
      const input = event.target.closest("input[data-menu-label]");
      if (!input) return;
      const items = currentMenuItems();
      const item = items.find((entry) => entry.tab === input.dataset.menuLabel);
      if (!item) return;
      item.label = input.value;
      setCurrentMenuItems(items);
      applyMenuSettings();
    });
    $("menu-settings-rows").addEventListener("change", (event) => {
      const homeInput = event.target.closest("input[data-menu-home]");
      if (homeInput) {
        if (!state.config) state.config = {};
        if (!state.config.ui) state.config.ui = {};
        if (menuDefinition(homeInput.dataset.menuHome)) state.config.ui.default_tab = homeInput.dataset.menuHome;
        renderMenuSettings();
        return;
      }
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
    document.querySelectorAll("[data-api-key-exchange]").forEach((button) => {
      button.addEventListener("click", () => {
        state.apiKeyExchange = normalizeExchange(button.dataset.apiKeyExchange);
        state.apiKeys = apiKeyStatus(state.apiKeyExchange);
        state.apiKeyTest = null;
        state.apiKeyTestID = "";
        state.apiKeyTestExchange = state.apiKeyExchange;
        renderAPIKeys();
        renderTemplateAPIs();
        renderPositionAPIs();
        updateMetrics();
      });
    });
    $("add-api-key").addEventListener("click", () => {
      const exchange = normalizeExchange(state.apiKeyExchange);
      state.selectedAPIID = "";
      state.selectedAPIIDs[exchange] = "";
      state.apiKeyTest = null;
      state.apiKeyTestID = "";
      state.apiKeyTestExchange = exchange;
      $("key-selected").value = "";
      $("key-id").value = "";
      $("key-name").value = "";
      const status = apiKeyStatus(exchange);
      $("key-active").checked = !status || !status.configured;
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
      renderAPIKeyStatus("");
      $("key-id").focus();
    });
    $("key-selected").addEventListener("change", () => {
      state.selectedAPIID = $("key-selected").value;
      state.selectedAPIIDs[normalizeExchange(state.apiKeyExchange)] = state.selectedAPIID;
      fillAPIForm(state.selectedAPIID);
      renderAPIKeyStatus(state.selectedAPIID);
    });
    document.querySelectorAll("[data-balance-minutes]").forEach((button) => {
      button.addEventListener("click", () => setBalanceWindow(button.dataset.balanceMinutes).catch((err) => toast(err.message)));
    });
    $("reset-balance-baseline").addEventListener("click", () => resetBalanceBaseline($("reset-balance-baseline")).catch((err) => toast(err.message)));
    $("sync-balance-history").addEventListener("click", () => syncBalanceHistory($("sync-balance-history")).catch((err) => toast(err.message)));
    $("analysis-api-id").addEventListener("change", () => loadAnalysis(false).catch((err) => toast(err.message)));
    $("refresh-analysis").addEventListener("click", () => loadAnalysis(true).then(() => toast("分析已刷新")).catch((err) => toast(err.message)));
    $("analysis-trade-prev").addEventListener("click", () => {
      state.analysisTradePage = Math.max(1, Number(state.analysisTradePage || 1) - 1);
      renderAnalysisTradeHistory("");
    });
    $("analysis-trade-next").addEventListener("click", () => {
      state.analysisTradePage = Number(state.analysisTradePage || 1) + 1;
      renderAnalysisTradeHistory("");
    });
    $("refresh-positions").addEventListener("click", () => loadPositionView(true).then(() => toast("持仓和挂单已刷新")).catch((err) => toast(err.message)));
    $("tpl-target-exchange").addEventListener("change", () => renderTemplateAPIs());
    $("make-template").addEventListener("click", () => makeTemplate().catch((err) => toast(err.message)));
    $("copy-webhook-url").addEventListener("click", async () => {
      renderTemplateWebhookURL();
      await navigator.clipboard.writeText($("template-webhook-url").value);
      toast("Webhook URL 已复制");
    });
    $("copy-template").addEventListener("click", async () => {
      await navigator.clipboard.writeText($("template-output").value);
      toast("已复制");
    });
    $("close-raw-json").addEventListener("click", () => closeRawJSONDialog());
    $("copy-raw-json").addEventListener("click", async () => {
      await navigator.clipboard.writeText($("raw-json-output").textContent || "");
      toast("原始 JSON 已复制");
    });
    $("order-rows").addEventListener("click", (event) => {
      const jsonButton = event.target.closest("button[data-order-json-index]");
      if (jsonButton) {
        const order = state.orders[Number(jsonButton.dataset.orderJsonIndex)];
        showOrderRawJSON(order);
        return;
      }
      const button = event.target.closest("button[data-retry-id]");
      if (!button) return;
      retryOrder(button.dataset.retryId).catch((err) => toast(err.message));
    });
    $("position-rows").addEventListener("click", (event) => {
      const protectionButton = event.target.closest("button[data-position-protection]");
      if (protectionButton) {
        protectPosition(protectionButton).catch((err) => toast(err.message));
        return;
      }
      const button = event.target.closest("button[data-position-close]");
      if (!button) return;
      closePosition(button).catch((err) => toast(err.message));
    });
    $("pending-order-rows").addEventListener("click", (event) => {
      const button = event.target.closest("button[data-pending-chase]");
      if (button) {
        chasePendingOrder(button).catch((err) => toast(err.message));
        return;
      }
      const cancelButton = event.target.closest("button[data-pending-cancel]");
      if (!cancelButton) return;
      cancelPendingOrder(cancelButton).catch((err) => toast(err.message));
    });
    $("refresh-orders").addEventListener("click", () => loadOrders().then(() => toast("订单已刷新")).catch((err) => toast(err.message)));
    $("refresh-upgrade").addEventListener("click", () => loadUpgrade().then(() => toast("升级状态已刷新")).catch((err) => toast(err.message)));
    $("start-upgrade").addEventListener("click", () => startUpgrade().catch((err) => toast(err.message)));
    document.addEventListener("visibilitychange", () => {
      if (!document.hidden && analysisTabActive()) {
        refreshAnalysisBalanceOverviewAuto();
      }
    });

    renderTemplateWebhookURL();
    updateBalanceWindowButtons();
    initTableColumnDrag();
    renderPositions();
    renderPendingOrders();
    activateTab(initialTab(), false);
    loadAll().catch((err) => toast(err.message));
  </script>
</body>
</html>`

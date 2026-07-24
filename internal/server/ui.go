package server

import "net/http"

func writeHTML(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}

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
      max-width: 1240px;
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
      max-width: 1240px;
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
    @media (max-width: 880px) {
      .bar { align-items: flex-start; flex-direction: column; }
      nav { justify-content: flex-start; }
      .status, .grid, .grid.two, .split, .api-key-layout { grid-template-columns: 1fr; }
      main { padding: 12px; }
      section { padding: 12px; }
    }
  </style>
</head>
<body>
  <header>
    <div class="bar">
      <div class="brand"><span class="mark">TV</span><span>OKX Bot</span></div>
      <nav aria-label="sections">
        <button type="button" data-tab="dashboard" aria-selected="true">总览</button>
        <button type="button" data-tab="config">配置</button>
        <button type="button" data-tab="apiKeys">API Key</button>
        <button type="button" data-tab="symbols">币对</button>
        <button type="button" data-tab="template">告警模板</button>
        <button type="button" data-tab="orders">订单</button>
        <button type="button" data-tab="upgrade">升级</button>
      </nav>
    </div>
  </header>

  <main>
    <div class="status">
      <div class="metric"><div class="label">交易环境</div><div class="value" id="metric-env">-</div></div>
      <div class="metric"><div class="label">OKX API</div><div class="value" id="metric-api-keys">-</div></div>
      <div class="metric"><div class="label">币对映射</div><div class="value" id="metric-symbols">-</div></div>
      <div class="metric"><div class="label">最近订单</div><div class="value" id="metric-orders">-</div></div>
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

    <section id="config">
      <div class="section-head">
        <h2>配置</h2>
        <button class="btn primary" type="button" id="save-config">保存配置</button>
      </div>
      <div class="grid">
        <label>监听地址<input id="cfg-addr" autocomplete="off"></label>
        <label>数据文件<input id="cfg-data-file" autocomplete="off"></label>
        <label>OKX Base URL<input id="cfg-base-url" autocomplete="off"></label>
        <label>交易环境<select id="cfg-env"><option value="demo">demo</option><option value="live">live</option></select></label>
        <label>保证金模式<select id="cfg-margin"><option value="isolated">isolated</option><option value="cross">cross</option></select></label>
        <label>持仓模式<select id="cfg-position"><option value="net">net</option><option value="long_short">long_short</option></select></label>
        <label>信号有效秒数<input id="cfg-ttl" type="number" min="1" step="1"></label>
        <label class="check"><input id="cfg-live" type="checkbox">允许实盘交易</label>
      </div>
    </section>

    <section id="symbols">
      <div class="section-head">
        <h2>币对</h2>
        <button class="btn primary" type="button" id="save-symbols">保存币对</button>
      </div>
      <table>
        <thead>
          <tr><th>Coinpair</th><th>OKX InstID</th><th>CtVal</th><th>LotSz</th><th>MinSz</th><th>操作</th></tr>
        </thead>
        <tbody id="symbol-rows"></tbody>
      </table>
      <div class="actions"><button class="btn" type="button" id="add-symbol">新增币对</button></div>
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
          <label>杠杆<input id="tpl-leverage" type="number" min="1" step="1" value="5"></label>
          <label>USDT 名义金额<input id="tpl-amount" type="number" min="0" step="0.01" value="100"></label>
        </div>
        <div>
          <textarea id="template-output" readonly></textarea>
          <div class="actions"><button class="btn" type="button" id="copy-template">复制 JSON</button></div>
        </div>
      </div>
    </section>

    <section id="orders">
      <div class="section-head">
        <h2>订单</h2>
        <button class="btn" type="button" id="refresh-orders">刷新订单</button>
      </div>
      <table>
        <thead>
          <tr><th>时间</th><th>状态</th><th>API</th><th>方向</th><th>币对</th><th>价格</th><th>金额</th><th>OKX</th></tr>
        </thead>
        <tbody id="order-rows"></tbody>
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
  <div class="toast" id="toast"></div>

  <script>
    const state = { config: null, apiKeys: null, selectedAPIID: "", symbols: {}, orders: [], upgrade: null };
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

    function pill(text, tone) {
      return '<span class="pill ' + (tone || "") + '">' + escapeHTML(asText(text)) + '</span>';
    }

    function escapeHTML(v) {
      return String(v).replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    }

    async function loadAll() {
      await Promise.allSettled([loadConfig(), loadAPIKeys(), loadSymbols(), loadOrders(), loadUpgrade()]);
      renderDashboard();
    }

    async function loadConfig() {
      state.config = await api("/tvbot/config");
      renderConfig();
      updateMetrics();
    }

    async function loadSymbols() {
      const data = await api("/tvbot/symbols");
      state.symbols = data.symbols || {};
      renderSymbols();
      updateMetrics();
    }

    async function loadAPIKeys() {
      state.apiKeys = await api("/tvbot/api-keys");
      renderAPIKeys();
      renderTemplateAPIs();
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

    function updateMetrics() {
      $("metric-env").textContent = state.config && state.config.trading ? state.config.trading.env : "-";
      $("metric-api-keys").textContent = state.apiKeys && state.apiKeys.configured ? (state.apiKeys.active_id || "已配置") : "未配置";
      $("metric-symbols").textContent = Object.keys(state.symbols || {}).length;
      $("metric-orders").textContent = state.orders ? state.orders.length : "-";
    }

    function renderDashboard() {
      if (!state.config) return;
      const t = state.config.trading || {};
      const rows = [
        ["服务地址", state.config.server ? state.config.server.addr : "-"],
        ["数据文件", state.config.data_file],
        ["交易环境", t.env],
        ["实盘开关", t.allow_live_trading ? "enabled" : "disabled"],
        ["保证金模式", t.default_margin_mode],
        ["持仓模式", t.position_mode],
        ["信号有效秒数", t.signal_ttl_seconds]
      ];
      $("dashboard-config").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(asText(row[1])) + "</td></tr>").join("");
    }

    function renderConfig() {
      const cfg = state.config || {};
      const trading = cfg.trading || {};
      $("cfg-addr").value = cfg.server && cfg.server.addr ? cfg.server.addr : "";
      $("cfg-data-file").value = cfg.data_file || "";
      $("cfg-base-url").value = trading.base_url || "";
      $("cfg-env").value = trading.env || "demo";
      $("cfg-margin").value = trading.default_margin_mode || "isolated";
      $("cfg-position").value = trading.position_mode || "net";
      $("cfg-ttl").value = trading.signal_ttl_seconds || 120;
      $("cfg-live").checked = !!trading.allow_live_trading;
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
      const rows = [
        ["配置状态", status.configured ? "已配置" : "未配置"],
        ["交易 API", status.active_id || "-"],
        ["API Key", status.api_key_masked || "-"],
        ["Secret Key", status.secret_key_set ? "已保存" : "未保存"],
        ["Passphrase", status.passphrase_set ? "已保存" : "未保存"],
        ["来源", status.source || "-"],
        ["更新时间", status.updated_at || "-"]
      ];
      $("api-key-status").innerHTML = rows.map((row) => "<tr><th>" + escapeHTML(row[0]) + "</th><td>" + escapeHTML(row[1]) + "</td></tr>").join("");
      $("api-key-accounts").innerHTML = accounts.map((account) => {
        return "<tr>" +
          "<td>" + escapeHTML(account.id) + "</td>" +
          "<td>" + escapeHTML(account.name || "-") + "</td>" +
          "<td>" + pill(account.active ? "交易 API" : (account.configured ? "已配置" : "未配置"), account.active ? "ok" : (account.configured ? "warn" : "bad")) + "</td>" +
          "<td>" + escapeHTML(account.api_key_masked || "-") + "</td>" +
          "</tr>";
      }).join("") || '<tr><td colspan="4" class="muted">-</td></tr>';
    }

    function apiAccounts() {
      return state.apiKeys && Array.isArray(state.apiKeys.credentials) ? state.apiKeys.credentials : [];
    }

    function selectedAPIAccount(id) {
      return apiAccounts().find((account) => account.id === id) || null;
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

    function symbolRow(key, sym) {
      const coin = sym.coinpair || key || "";
      return '<tr data-symbol-row>' +
        '<td><input data-field="coinpair" value="' + escapeHTML(coin) + '"></td>' +
        '<td><input data-field="inst_id" value="' + escapeHTML(sym.inst_id || "") + '"></td>' +
        '<td><input data-field="ct_val" type="number" step="0.00000001" value="' + escapeHTML(sym.ct_val || "") + '"></td>' +
        '<td><input data-field="lot_sz" type="number" step="0.00000001" value="' + escapeHTML(sym.lot_sz || "") + '"></td>' +
        '<td><input data-field="min_sz" type="number" step="0.00000001" value="' + escapeHTML(sym.min_sz || "") + '"></td>' +
        '<td><button class="btn danger" type="button" data-remove-symbol>删除</button></td>' +
        '</tr>';
    }

    function renderSymbols() {
      const rows = Object.keys(state.symbols || {}).sort().map((key) => symbolRow(key, state.symbols[key]));
      $("symbol-rows").innerHTML = rows.join("") || '<tr><td colspan="6" class="muted">-</td></tr>';
    }

    function collectSymbols() {
      const symbols = {};
      document.querySelectorAll("[data-symbol-row]").forEach((row) => {
        const get = (field) => row.querySelector('[data-field="' + field + '"]').value.trim();
        const coinpair = get("coinpair").toUpperCase();
        if (!coinpair) return;
        symbols[coinpair] = {
          coinpair: coinpair,
          inst_id: get("inst_id").toUpperCase(),
          ct_val: Number(get("ct_val")),
          lot_sz: Number(get("lot_sz")),
          min_sz: Number(get("min_sz"))
        };
      });
      return symbols;
    }

    function renderOrders() {
      const rows = (state.orders || []).map((order) => {
        const okx = order.result && (order.result.ord_id || order.result.okx_code) ? [order.result.ord_id, order.result.okx_code].filter(Boolean).join(" / ") : order.error || "-";
        const tone = order.status === "submitted" ? "ok" : (order.status === "failed" ? "bad" : "warn");
        return "<tr>" +
          "<td>" + escapeHTML(asText(order.accepted_at)) + "</td>" +
          "<td>" + pill(order.status, tone) + "</td>" +
          "<td>" + escapeHTML(asText(order.api_id || (order.result && order.result.api_id))) + "</td>" +
          "<td>" + escapeHTML(asText(order.action)) + "</td>" +
          "<td>" + escapeHTML(asText(order.coinpair)) + "</td>" +
          "<td>" + escapeHTML(asText(order.price)) + "</td>" +
          "<td>" + escapeHTML(asText(order.amount)) + "</td>" +
          "<td>" + escapeHTML(okx) + "</td>" +
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
      renderConfig();
      renderDashboard();
      updateMetrics();
      toast("配置已保存");
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
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
      renderAPIKeys();
      renderTemplateAPIs();
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
      $("okx-output").textContent = JSON.stringify(result, null, 2);
      toast("API 可用");
    }

    async function deleteAPIKey() {
      const id = $("key-id").value.trim() || $("key-selected").value;
      if (!id) return;
      state.apiKeys = await api("/tvbot/api-keys?id=" + encodeURIComponent(id), { method: "DELETE" });
      state.selectedAPIID = state.apiKeys.active_id || "";
      renderAPIKeys();
      renderTemplateAPIs();
      updateMetrics();
      toast("API Key 已删除");
    }

    async function saveSymbols() {
      state.symbols = (await api("/tvbot/symbols", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ symbols: collectSymbols() }) })).symbols || {};
      renderSymbols();
      updateMetrics();
      toast("币对已保存");
    }

    async function makeTemplate() {
      const req = {
        api_id: $("tpl-api-id").value,
        price_source: $("tpl-price-source").value,
        leverage: Number($("tpl-leverage").value),
        amount: Number($("tpl-amount").value)
      };
      const resp = await api("/tvbot/templates", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(req) });
      $("template-output").value = resp.json || "";
      toast("模板已生成");
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
        document.querySelectorAll("nav button").forEach((b) => b.setAttribute("aria-selected", "false"));
        document.querySelectorAll("main section").forEach((s) => s.classList.remove("active"));
        button.setAttribute("aria-selected", "true");
        $(button.dataset.tab).classList.add("active");
      });
    });

    $("refresh-all").addEventListener("click", () => loadAll().then(() => toast("已刷新")).catch((err) => toast(err.message)));
    $("check-okx").addEventListener("click", () => checkOKX());
    $("save-config").addEventListener("click", () => saveConfig().catch((err) => toast(err.message)));
    $("save-api-keys").addEventListener("click", () => saveAPIKeys().catch((err) => toast(err.message)));
    $("test-api-keys").addEventListener("click", () => testAPIKeys().catch((err) => {
      $("okx-output").textContent = err.message;
      toast(err.message);
    }));
    $("delete-api-key").addEventListener("click", () => deleteAPIKey().catch((err) => toast(err.message)));
    $("add-api-key").addEventListener("click", () => {
      state.selectedAPIID = "";
      $("key-selected").value = "";
      $("key-id").value = "";
      $("key-name").value = "";
      $("key-active").checked = !state.apiKeys || !state.apiKeys.configured;
      $("key-api").value = "";
      $("key-secret").value = "";
      $("key-passphrase").value = "";
      $("key-id").focus();
    });
    $("key-selected").addEventListener("change", () => {
      state.selectedAPIID = $("key-selected").value;
      fillAPIForm(state.selectedAPIID);
    });
    $("save-symbols").addEventListener("click", () => saveSymbols().catch((err) => toast(err.message)));
    $("add-symbol").addEventListener("click", () => {
      $("symbol-rows").insertAdjacentHTML("beforeend", symbolRow("", { coinpair: "", inst_id: "", ct_val: "", lot_sz: "", min_sz: "" }));
    });
    $("symbol-rows").addEventListener("click", (event) => {
      if (event.target.matches("[data-remove-symbol]")) event.target.closest("tr").remove();
    });
    $("make-template").addEventListener("click", () => makeTemplate().catch((err) => toast(err.message)));
    $("copy-template").addEventListener("click", async () => {
      await navigator.clipboard.writeText($("template-output").value);
      toast("已复制");
    });
    $("refresh-orders").addEventListener("click", () => loadOrders().then(() => toast("订单已刷新")).catch((err) => toast(err.message)));
    $("refresh-upgrade").addEventListener("click", () => loadUpgrade().then(() => toast("升级状态已刷新")).catch((err) => toast(err.message)));
    $("start-upgrade").addEventListener("click", () => startUpgrade().catch((err) => toast(err.message)));

    loadAll().catch((err) => toast(err.message));
  </script>
</body>
</html>`

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	tradeChartBar        = "1m"
	tradeChartDays       = 7
	tradeChartMaxCandles = tradeChartDays * 24 * 60
)

type tradeChartCandle struct {
	TS     int64  `json:"ts"`
	Time   string `json:"time"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume,omitempty"`
}

type tradeChartSeries struct {
	Env     string             `json:"env"`
	Candles []tradeChartCandle `json:"candles"`
	Error   string             `json:"error,omitempty"`
}

type tradeChartMarker struct {
	Kind  string `json:"kind"`
	Side  string `json:"side,omitempty"`
	TS    int64  `json:"ts"`
	Time  string `json:"time"`
	Price string `json:"price"`
}

type tradeChartResponse struct {
	OK       bool               `json:"ok"`
	Exchange string             `json:"exchange"`
	InstID   string             `json:"inst_id"`
	Bar      string             `json:"bar"`
	From     string             `json:"from"`
	To       string             `json:"to"`
	Demo     tradeChartSeries   `json:"demo"`
	Live     tradeChartSeries   `json:"live"`
	Markers  []tradeChartMarker `json:"markers"`
}

func (s *Server) handleTradeChartPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	writeHTML(w, http.StatusOK, tradeChartHTML)
}

func (s *Server) handleTradeChartData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	exchange := trading.NormalizeExchange(r.URL.Query().Get("exchange"))
	if exchange == "" {
		exchange = trading.ExchangeOKX
	}
	if !trading.ValidTargetExchange(exchange) {
		writeError(w, http.StatusBadRequest, "invalid_exchange", "exchange must be okx or binance")
		return
	}
	instID, err := normalizeTradeChartInstrument(exchange, r.URL.Query().Get("inst_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_instrument", err.Error())
		return
	}

	now := s.now()
	windowStart, windowEnd := tradeChartWindow(now, parseChartTimestamp(r.URL.Query().Get("entry_ts")), parseChartTimestamp(r.URL.Query().Get("exit_ts")))
	markers := tradeChartMarkers(r, exchange)
	cfg := config.Default()
	if s.ConfigStore != nil {
		cfg = s.ConfigStore.Get()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	demo, live := s.fetchTradeChartSeries(ctx, cfg, exchange, instID, windowStart, windowEnd)
	if demo.Error != "" && live.Error != "" {
		writeError(w, http.StatusBadGateway, "chart_data_failed", fmt.Sprintf("demo: %s; live: %s", demo.Error, live.Error))
		return
	}
	writeJSON(w, http.StatusOK, tradeChartResponse{
		OK:       true,
		Exchange: exchange,
		InstID:   instID,
		Bar:      tradeChartBar,
		From:     windowStart.UTC().Format(time.RFC3339Nano),
		To:       windowEnd.UTC().Format(time.RFC3339Nano),
		Demo:     demo,
		Live:     live,
		Markers:  markers,
	})
}

func tradeChartWindow(now time.Time, entryTS, exitTS int64) (time.Time, time.Time) {
	now = now.UTC()
	end := now
	if exitTS > 0 && exitTS < now.Add(-tradeChartDays*24*time.Hour).UnixMilli() {
		end = time.UnixMilli(exitTS).UTC().Add(time.Minute)
	}
	if entryTS > 0 && entryTS < end.Add(-tradeChartDays*24*time.Hour).UnixMilli() && entryTS > exitTS {
		end = time.UnixMilli(entryTS).UTC().Add(time.Minute)
	}
	return end.Add(-tradeChartDays * 24 * time.Hour), end
}

func parseChartTimestamp(raw string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	if parsed < 100000000000 {
		return parsed * 1000
	}
	return parsed
}

func tradeChartMarkers(r *http.Request, exchange string) []tradeChartMarker {
	markers := make([]tradeChartMarker, 0, 2)
	for _, item := range []struct {
		kind  string
		field string
	}{
		{kind: "entry", field: "entry_ts"},
		{kind: "exit", field: "exit_ts"},
	} {
		ts := parseChartTimestamp(r.URL.Query().Get(item.field))
		price := strings.TrimSpace(r.URL.Query().Get(item.kind + "_px"))
		if ts <= 0 || price == "" {
			continue
		}
		markers = append(markers, tradeChartMarker{
			Kind:  item.kind,
			Side:  strings.ToLower(strings.TrimSpace(r.URL.Query().Get("side"))),
			TS:    ts,
			Time:  time.UnixMilli(ts).UTC().Format(time.RFC3339Nano),
			Price: price,
		})
	}
	return markers
}

func normalizeTradeChartInstrument(exchange, raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, strings.ToUpper(exchange)+":")
	value = strings.TrimSuffix(value, ".P")
	value = strings.TrimSuffix(value, ".PERP")
	value = strings.TrimSuffix(value, "PERP")
	if value == "" {
		return "", errors.New("inst_id is required")
	}
	if exchange == trading.ExchangeBinance {
		value = strings.ReplaceAll(value, "-SWAP", "")
		value = strings.ReplaceAll(value, "-", "")
		if !strings.HasSuffix(value, "USDT") {
			value += "USDT"
		}
		if !chartInstrumentCharacters(value, false) {
			return "", errors.New("invalid Binance symbol")
		}
		return value, nil
	}

	parts := strings.Split(value, "-")
	if len(parts) == 3 && parts[2] == "SWAP" {
		if chartInstrumentCharacters(parts[0], true) && chartInstrumentCharacters(parts[1], true) {
			return strings.Join(parts, "-"), nil
		}
		return "", errors.New("invalid OKX instrument")
	}
	if len(parts) == 2 && (parts[1] == "USDT" || parts[1] == "USDC" || parts[1] == "USD") {
		if chartInstrumentCharacters(parts[0], true) {
			return value + "-SWAP", nil
		}
		return "", errors.New("invalid OKX instrument")
	}
	for _, quote := range []string{"USDT", "USDC", "USD"} {
		if strings.HasSuffix(value, quote) {
			base := strings.TrimSuffix(value, quote)
			if chartInstrumentCharacters(base, true) {
				return base + "-" + quote + "-SWAP", nil
			}
		}
	}
	if chartInstrumentCharacters(value, true) {
		return value + "-USDT-SWAP", nil
	}
	return "", errors.New("invalid OKX instrument")
}

func chartInstrumentCharacters(value string, allowDash bool) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || (allowDash && char == '-') {
			continue
		}
		return false
	}
	return true
}

func (s *Server) fetchTradeChartSeries(ctx context.Context, cfg config.Config, exchange, instID string, since, until time.Time) (tradeChartSeries, tradeChartSeries) {
	demo := tradeChartSeries{Env: trading.TradeEnvDemo, Candles: []tradeChartCandle{}}
	live := tradeChartSeries{Env: trading.TradeEnvLive, Candles: []tradeChartCandle{}}
	for _, item := range []struct {
		env    string
		series *tradeChartSeries
	}{
		{env: trading.TradeEnvDemo, series: &demo},
		{env: trading.TradeEnvLive, series: &live},
	} {
		var candles []tradeChartCandle
		var err error
		if exchange == trading.ExchangeBinance {
			candles, err = s.fetchBinanceTradeChartCandles(ctx, configForTradeEnv(cfg, item.env), instID, since, until)
		} else {
			candles, err = s.fetchOKXTradeChartCandles(ctx, configForTradeEnv(cfg, item.env), instID, since, until)
		}
		if err != nil {
			item.series.Error = err.Error()
			continue
		}
		item.series.Candles = candles
	}
	return demo, live
}

func (s *Server) fetchOKXTradeChartCandles(ctx context.Context, cfg config.Config, instID string, since, until time.Time) ([]tradeChartCandle, error) {
	client := s.analysisOKXClient(cfg, okx.Credentials{})
	candles, err := client.MarketCandlesRange(ctx, instID, tradeChartBar, since, until, tradeChartMaxCandles)
	if err != nil {
		return nil, err
	}
	out := make([]tradeChartCandle, 0, len(candles))
	for _, candle := range candles {
		ts, err := strconv.ParseInt(strings.TrimSpace(candle.TS), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, tradeChartCandle{
			TS:     ts,
			Time:   time.UnixMilli(ts).UTC().Format(time.RFC3339Nano),
			Open:   candle.Open,
			High:   candle.High,
			Low:    candle.Low,
			Close:  candle.Close,
			Volume: candle.Volume,
		})
	}
	return out, nil
}

func (s *Server) fetchBinanceTradeChartCandles(ctx context.Context, cfg config.Config, symbol string, since, until time.Time) ([]tradeChartCandle, error) {
	client := s.analysisBinanceClient(cfg, binance.Credentials{})
	candles, err := client.MarketKlinesRange(ctx, symbol, tradeChartBar, since, until, tradeChartMaxCandles)
	if err != nil {
		return nil, err
	}
	out := make([]tradeChartCandle, 0, len(candles))
	for _, candle := range candles {
		out = append(out, tradeChartCandle{
			TS:     candle.TS,
			Time:   time.UnixMilli(candle.TS).UTC().Format(time.RFC3339Nano),
			Open:   candle.Open,
			High:   candle.High,
			Low:    candle.Low,
			Close:  candle.Close,
			Volume: candle.Volume,
		})
	}
	return out, nil
}

const tradeChartHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>币对 K 线回看</title>
  <style>
    :root { color-scheme: light; font-family: -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",sans-serif; color:#172033; background:#f4f7fb; }
    * { box-sizing:border-box; }
    body { margin:0; min-width:960px; background:#f4f7fb; }
    .page { max-width:1640px; margin:0 auto; padding:22px 24px 32px; }
    header { display:flex; justify-content:space-between; align-items:flex-start; gap:20px; margin-bottom:16px; }
    h1 { margin:0; font-size:22px; letter-spacing:.2px; }
    .subtitle { color:#68758d; font-size:13px; margin-top:6px; }
    .status { color:#667085; font-size:13px; text-align:right; max-width:620px; }
    .toolbar, .legend, .trade-info { display:flex; align-items:center; flex-wrap:wrap; gap:8px 16px; }
    .toolbar { background:#fff; border:1px solid #dce3ee; border-radius:10px; padding:10px 12px; box-shadow:0 3px 12px rgba(33,55,85,.04); }
    button { border:1px solid #c9d3e2; border-radius:6px; background:#fff; color:#344054; padding:6px 12px; cursor:pointer; font-size:13px; }
    button:hover { border-color:#1f6feb; color:#1f6feb; }
    button.active { color:#fff; background:#1f6feb; border-color:#1f6feb; }
    .legend { margin:14px 2px 10px; color:#475467; font-size:13px; }
    .legend-item { display:inline-flex; align-items:center; gap:6px; }
    .swatch { width:12px; height:12px; border-radius:3px; display:inline-block; }
    .swatch.demo { background:#2f81f7; }
    .swatch.live { background:#f59e0b; }
    .marker-dot { width:10px; height:10px; border-radius:50%; display:inline-block; }
    .marker-dot.entry { background:#159957; }
    .marker-dot.exit { background:#d92d20; }
    .chart-card { position:relative; background:#fff; border:1px solid #dce3ee; border-radius:10px; padding:8px; box-shadow:0 5px 18px rgba(33,55,85,.06); }
    canvas { width:100%; height:620px; display:block; cursor:crosshair; }
    .tooltip { position:absolute; display:none; pointer-events:none; background:rgba(23,32,51,.94); color:#fff; border-radius:6px; padding:7px 9px; font-size:12px; line-height:1.5; white-space:nowrap; z-index:2; }
    .trade-info { margin-top:14px; background:#fff; border:1px solid #dce3ee; border-radius:10px; padding:12px 14px; font-size:13px; color:#475467; }
    .trade-info strong { color:#172033; }
    .error { color:#b42318; }
    @media (max-width:1100px) { body { min-width:0; } .page { padding:16px 12px 24px; } canvas { height:520px; } header { display:block; } .status { margin-top:8px; text-align:left; } }
  </style>
</head>
<body>
  <main class="page">
    <header>
      <div><h1 id="title">币对 K 线回看</h1><div class="subtitle">1 分钟周期 · 最近七天 · 模拟盘 / 实盘价格对比</div></div>
      <div class="status" id="status">正在加载 K 线数据…</div>
    </header>
    <div class="toolbar" role="group" aria-label="回看周期">
      <span>回看范围：</span>
      <button type="button" data-days="1">1 天</button>
      <button type="button" data-days="3">3 天</button>
      <button type="button" data-days="7" class="active">7 天</button>
      <span id="window-text"></span>
    </div>
    <div class="legend">
      <span class="legend-item"><i class="swatch demo"></i>模拟盘 K 线</span>
      <span class="legend-item"><i class="swatch live"></i>实盘 K 线</span>
      <span class="legend-item"><i class="marker-dot entry"></i>开仓 / 买入</span>
      <span class="legend-item"><i class="marker-dot exit"></i>平仓 / 卖出</span>
    </div>
    <div class="chart-card"><canvas id="chart" aria-label="模拟盘与实盘一分钟 K 线图"></canvas><div class="tooltip" id="tooltip"></div></div>
    <div class="trade-info" id="trade-info">-</div>
  </main>
  <script>
    const params = new URLSearchParams(window.location.search);
    const state = { data: null, days: 7, hover: null };
    const canvas = document.getElementById("chart");
    const ctx = canvas.getContext("2d");
    const tooltip = document.getElementById("tooltip");
    const statusEl = document.getElementById("status");
    const titleEl = document.getElementById("title");
    const infoEl = document.getElementById("trade-info");
    const windowEl = document.getElementById("window-text");
    const colors = { demo: "#2f81f7", live: "#f59e0b", entry: "#159957", exit: "#d92d20" };

    function escapeHTML(value) {
      return String(value == null ? "" : value).replace(/[&<>\"']/g, function (char) { return ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"})[char]; });
    }
    function formatTime(value) {
      if (!value) return "-";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return String(value);
      return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", month:"2-digit", day:"2-digit", hour:"2-digit", minute:"2-digit", second:"2-digit", hour12:false }).format(date);
    }
    function formatAxisTime(ts, withDate) {
      const date = new Date(ts);
      return new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", month: withDate ? "2-digit" : undefined, day: withDate ? "2-digit" : undefined, hour:"2-digit", minute:"2-digit", hour12:false }).format(date).replace("/", "-");
    }
    function number(value) { const parsed = Number(value); return Number.isFinite(parsed) ? parsed : null; }
    function priceText(value) {
      const parsed = number(value);
      if (parsed == null) return "-";
      if (parsed >= 1000) return parsed.toLocaleString("en-US", { maximumFractionDigits: 2 });
      if (parsed >= 1) return parsed.toFixed(4).replace(/0+$/, "").replace(/\.$/, "");
      return parsed.toFixed(8).replace(/0+$/, "").replace(/\.$/, "");
    }
    function seriesPoints(series, from, to) {
      return (series && series.candles || []).filter(function (item) { return item.ts >= from && item.ts <= to; });
    }
    function currentWindow() {
      const end = new Date(state.data.to).getTime();
      return { from: end - state.days * 24 * 60 * 60 * 1000, to: end };
    }
    function markerAction(marker) {
      const side = String(marker.side || "").toLowerCase();
      if (marker.kind === "entry") return side === "short" ? "卖出开空" : "买入开多";
      return side === "short" ? "买入平空" : "卖出平多";
    }
    function draw() {
      if (!state.data) return;
      const rect = canvas.getBoundingClientRect();
      const width = Math.max(640, Math.floor(rect.width));
      const height = Math.max(360, Math.floor(rect.height));
      const ratio = window.devicePixelRatio || 1;
      canvas.width = Math.floor(width * ratio);
      canvas.height = Math.floor(height * ratio);
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      ctx.clearRect(0, 0, width, height);
      const windowRange = currentWindow();
      const demo = seriesPoints(state.data.demo, windowRange.from, windowRange.to);
      const live = seriesPoints(state.data.live, windowRange.from, windowRange.to);
      const points = demo.concat(live);
      const markers = (state.data.markers || []).filter(function (item) { return item.ts >= windowRange.from && item.ts <= windowRange.to; });
      const left = 68, right = width - 76, top = 22, bottom = height - 48;
      const plotWidth = Math.max(1, right - left), plotHeight = Math.max(1, bottom - top);
      let min = Infinity, max = -Infinity;
      points.forEach(function (item) { [item.low, item.high].forEach(function (value) { const parsed = number(value); if (parsed != null) { min = Math.min(min, parsed); max = Math.max(max, parsed); } }); });
      markers.forEach(function (item) { const parsed = number(item.price); if (parsed != null) { min = Math.min(min, parsed); max = Math.max(max, parsed); } });
      if (!Number.isFinite(min) || !Number.isFinite(max)) { min = 0; max = 1; }
      const padding = Math.max((max - min) * 0.08, max * 0.0005, 0.000001);
      min -= padding; max += padding;
      const x = function (ts) { return left + ((ts - windowRange.from) / (windowRange.to - windowRange.from)) * plotWidth; };
      const y = function (price) { return top + ((max - price) / (max - min)) * plotHeight; };
      ctx.font = "12px -apple-system,BlinkMacSystemFont,Segoe UI,Microsoft YaHei,sans-serif";
      ctx.lineWidth = 1;
      ctx.strokeStyle = "#e7edf5";
      ctx.fillStyle = "#667085";
      for (let i = 0; i <= 5; i += 1) {
        const yy = top + plotHeight * i / 5;
        ctx.beginPath(); ctx.moveTo(left, yy); ctx.lineTo(right, yy); ctx.stroke();
        ctx.fillText(priceText(max - (max - min) * i / 5), right + 8, yy + 4);
      }
      ctx.strokeStyle = "#aab6c7";
      ctx.beginPath(); ctx.moveTo(left, top); ctx.lineTo(left, bottom); ctx.lineTo(right, bottom); ctx.stroke();
      for (let i = 0; i <= 6; i += 1) {
        const ts = windowRange.from + (windowRange.to - windowRange.from) * i / 6;
        const xx = x(ts);
        ctx.strokeStyle = "#edf1f6"; ctx.beginPath(); ctx.moveTo(xx, top); ctx.lineTo(xx, bottom); ctx.stroke();
        ctx.fillStyle = "#667085"; ctx.fillText(formatAxisTime(ts, state.days >= 3), Math.max(left, xx - 25), bottom + 23);
      }
      function drawSeries(items, color, offset) {
        const candleWidth = Math.max(1, Math.min(5, plotWidth / Math.max(items.length, 1) * 0.72));
        items.forEach(function (item) {
          const open = number(item.open), high = number(item.high), low = number(item.low), close = number(item.close);
          if ([open, high, low, close].some(function (value) { return value == null; })) return;
          const xx = x(item.ts) + offset;
          const up = close >= open;
          ctx.strokeStyle = color; ctx.fillStyle = up ? color : "#fff"; ctx.lineWidth = 1;
          ctx.beginPath(); ctx.moveTo(xx, y(high)); ctx.lineTo(xx, y(low)); ctx.stroke();
          const topBody = Math.min(y(open), y(close));
          const bodyHeight = Math.max(1, Math.abs(y(close) - y(open)));
          ctx.fillRect(xx - candleWidth / 2, topBody, candleWidth, bodyHeight);
          if (!up) { ctx.strokeRect(xx - candleWidth / 2, topBody, candleWidth, bodyHeight); }
        });
      }
      drawSeries(demo, colors.demo, -1.5);
      drawSeries(live, colors.live, 1.5);
      markers.forEach(function (marker) {
        const price = number(marker.price); if (price == null) return;
        const xx = x(marker.ts), yy = y(price), color = colors[marker.kind] || "#344054";
        ctx.save(); ctx.strokeStyle = color; ctx.fillStyle = color; ctx.setLineDash([4, 4]); ctx.beginPath(); ctx.moveTo(xx, top); ctx.lineTo(xx, bottom); ctx.stroke(); ctx.restore();
        ctx.beginPath();
        if (marker.kind === "entry") { ctx.moveTo(xx, yy - 9); ctx.lineTo(xx - 6, yy + 2); ctx.lineTo(xx + 6, yy + 2); }
        else { ctx.moveTo(xx, yy + 9); ctx.lineTo(xx - 6, yy - 2); ctx.lineTo(xx + 6, yy - 2); }
        ctx.closePath(); ctx.fillStyle = color; ctx.fill();
        ctx.fillStyle = color; ctx.font = "bold 11px sans-serif"; ctx.fillText(markerAction(marker), xx + 7, marker.kind === "entry" ? yy - 6 : yy + 16);
      });
      if (state.hover) {
        const hoverX = x(state.hover.ts);
        ctx.save(); ctx.strokeStyle = "#98a2b3"; ctx.setLineDash([3, 3]); ctx.beginPath(); ctx.moveTo(hoverX, top); ctx.lineTo(hoverX, bottom); ctx.stroke(); ctx.restore();
      }
      windowEl.textContent = formatTime(new Date(windowRange.from).toISOString()) + " - " + formatTime(new Date(windowRange.to).toISOString()) + "（上海时间）";
    }
    function renderInfo() {
      const markers = state.data.markers || [];
      const entry = markers.find(function (item) { return item.kind === "entry"; });
      const exit = markers.find(function (item) { return item.kind === "exit"; });
      const side = params.get("side") || "-";
      infoEl.innerHTML = "方向：<strong>" + escapeHTML(side === "short" ? "空单" : side === "long" ? "多单" : side) + "</strong>　开仓：<strong>" + escapeHTML(entry ? formatTime(entry.time) + " / " + priceText(entry.price) : "-") + "</strong>　平仓：<strong>" + escapeHTML(exit ? formatTime(exit.time) + " / " + priceText(exit.price) : "-") + "</strong>";
    }
    function setStatus() {
      const demoCount = state.data.demo && state.data.demo.candles ? state.data.demo.candles.length : 0;
      const liveCount = state.data.live && state.data.live.candles ? state.data.live.candles.length : 0;
      const errors = [];
      if (state.data.demo && state.data.demo.error) errors.push("模拟盘：" + state.data.demo.error);
      if (state.data.live && state.data.live.error) errors.push("实盘：" + state.data.live.error);
      statusEl.innerHTML = errors.length ? '<span class="error">' + escapeHTML(errors.join("；")) + '</span>' : "已加载模拟盘 " + demoCount + " 根 / 实盘 " + liveCount + " 根 1 分钟 K 线";
    }
    async function load() {
      try {
        const response = await fetch("/tvbot/chart/data?" + params.toString(), { headers: { "Accept": "application/json" } });
        const body = await response.json();
        if (!response.ok) throw new Error(body.message || body.error || "K 线数据加载失败");
        state.data = body;
        titleEl.textContent = (body.exchange === "binance" ? "Binance " : "OKX ") + body.inst_id + " K 线回看";
        renderInfo(); setStatus(); draw();
      } catch (error) {
        statusEl.innerHTML = '<span class="error">' + escapeHTML(error.message || error) + '</span>';
      }
    }
    document.querySelectorAll("button[data-days]").forEach(function (button) { button.addEventListener("click", function () { state.days = Number(button.dataset.days) || 7; document.querySelectorAll("button[data-days]").forEach(function (item) { item.classList.toggle("active", item === button); }); draw(); }); });
    canvas.addEventListener("mousemove", function (event) {
      if (!state.data) return;
      const rect = canvas.getBoundingClientRect(); const range = currentWindow(); const left = 68, right = rect.width - 76;
      const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left - left) / Math.max(1, right - left)));
      const ts = range.from + (range.to - range.from) * ratio;
      const all = (state.data.demo.candles || []).concat(state.data.live.candles || []);
      let nearest = null;
      all.forEach(function (item) { if (!nearest || Math.abs(item.ts - ts) < Math.abs(nearest.ts - ts)) nearest = item; });
      state.hover = nearest;
      if (nearest) { tooltip.style.display = "block"; tooltip.style.left = Math.min(rect.width - 190, Math.max(8, event.clientX - rect.left + 12)) + "px"; tooltip.style.top = Math.max(8, event.clientY - rect.top - 44) + "px"; tooltip.textContent = formatTime(nearest.time) + "　开 " + priceText(nearest.open) + "　高 " + priceText(nearest.high) + "　低 " + priceText(nearest.low) + "　收 " + priceText(nearest.close); }
      draw();
    });
    canvas.addEventListener("mouseleave", function () { state.hover = null; tooltip.style.display = "none"; draw(); });
    window.addEventListener("resize", draw);
    load();
  </script>
</body>
</html>`

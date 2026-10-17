package server

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const (
	tradeChartDefaultBar = "1m"
	tradeChartRawBar     = "1m"
	tradeChartHours      = 8
	tradeChartMaxHours   = 24
	tradeChartMaxCandles = tradeChartMaxHours * 60
)

type tradeChartInterval struct {
	Bar      string
	Duration time.Duration
}

var tradeChartIntervals = []tradeChartInterval{
	{Bar: "1m", Duration: time.Minute},
	{Bar: "2m", Duration: 2 * time.Minute},
	{Bar: "3m", Duration: 3 * time.Minute},
	{Bar: "5m", Duration: 5 * time.Minute},
	{Bar: "10m", Duration: 10 * time.Minute},
	{Bar: "15m", Duration: 15 * time.Minute},
	{Bar: "30m", Duration: 30 * time.Minute},
	{Bar: "1h", Duration: time.Hour},
	{Bar: "4h", Duration: 4 * time.Hour},
	{Bar: "8h", Duration: 8 * time.Hour},
	{Bar: "12h", Duration: 12 * time.Hour},
}

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
	Hours    int                `json:"hours"`
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

	interval, err := normalizeTradeChartBar(r.URL.Query().Get("bar"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_bar", err.Error())
		return
	}
	hours, err := normalizeTradeChartHours(r.URL.Query().Get("hours"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_hours", err.Error())
		return
	}

	now := s.now()
	windowStart, windowEnd := tradeChartWindowForHours(now, parseChartTimestamp(r.URL.Query().Get("entry_ts")), parseChartTimestamp(r.URL.Query().Get("exit_ts")), hours)
	markers := tradeChartMarkers(r, exchange)
	cfg := config.Default()
	if s.ConfigStore != nil {
		cfg = s.ConfigStore.Get()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	demo, live := s.fetchTradeChartSeries(ctx, cfg, exchange, instID, windowStart, windowEnd, interval)
	if demo.Error != "" && live.Error != "" {
		writeError(w, http.StatusBadGateway, "chart_data_failed", fmt.Sprintf("demo: %s; live: %s", demo.Error, live.Error))
		return
	}
	writeJSON(w, http.StatusOK, tradeChartResponse{
		OK:       true,
		Exchange: exchange,
		InstID:   instID,
		Bar:      interval.Bar,
		Hours:    hours,
		From:     windowStart.UTC().Format(time.RFC3339Nano),
		To:       windowEnd.UTC().Format(time.RFC3339Nano),
		Demo:     demo,
		Live:     live,
		Markers:  markers,
	})
}

func normalizeTradeChartHours(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return tradeChartHours, nil
	}
	hours, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("hours must be one of 1, 4, 8, 12, 24")
	}
	switch hours {
	case 1, 4, 8, 12, 24:
		return hours, nil
	default:
		return 0, errors.New("hours must be one of 1, 4, 8, 12, 24")
	}
}

func normalizeTradeChartBar(raw string) (tradeChartInterval, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		value = tradeChartDefaultBar
	}
	for _, interval := range tradeChartIntervals {
		if interval.Bar == value {
			return interval, nil
		}
	}
	return tradeChartInterval{}, errors.New("bar must be one of 1m, 2m, 3m, 5m, 10m, 15m, 30m, 1h, 4h, 8h, 12h")
}

func tradeChartWindow(now time.Time, entryTS, exitTS int64) (time.Time, time.Time) {
	return tradeChartWindowForHours(now, entryTS, exitTS, tradeChartHours)
}

func tradeChartWindowForHours(now time.Time, entryTS, exitTS int64, hours int) (time.Time, time.Time) {
	now = now.UTC()
	end := now
	if exitTS > 0 && exitTS < now.Add(-time.Duration(hours)*time.Hour).UnixMilli() {
		end = time.UnixMilli(exitTS).UTC().Add(time.Minute)
	}
	if entryTS > 0 && entryTS < end.Add(-time.Duration(hours)*time.Hour).UnixMilli() && entryTS > exitTS {
		end = time.UnixMilli(entryTS).UTC().Add(time.Minute)
	}
	return end.Add(-time.Duration(hours) * time.Hour), end
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

func (s *Server) fetchTradeChartSeries(ctx context.Context, cfg config.Config, exchange, instID string, since, until time.Time, interval tradeChartInterval) (tradeChartSeries, tradeChartSeries) {
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
		item.series.Candles = aggregateTradeChartCandles(candles, interval)
	}
	return demo, live
}

func (s *Server) fetchOKXTradeChartCandles(ctx context.Context, cfg config.Config, instID string, since, until time.Time) ([]tradeChartCandle, error) {
	client := s.analysisOKXClient(cfg, okx.Credentials{})
	candles, err := client.MarketCandlesRange(ctx, instID, tradeChartRawBar, since, until, tradeChartMaxCandles)
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
	candles, err := client.MarketKlinesRange(ctx, symbol, tradeChartRawBar, since, until, tradeChartMaxCandles)
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

func aggregateTradeChartCandles(candles []tradeChartCandle, interval tradeChartInterval) []tradeChartCandle {
	if interval.Bar == tradeChartRawBar || len(candles) == 0 {
		return candles
	}
	bucketSize := interval.Duration.Milliseconds()
	grouped := make([]tradeChartCandle, 0, len(candles))
	indexes := make(map[int64]int, len(candles))
	for _, candle := range candles {
		if candle.TS <= 0 {
			continue
		}
		bucket := candle.TS / bucketSize * bucketSize
		index, found := indexes[bucket]
		if !found {
			indexes[bucket] = len(grouped)
			grouped = append(grouped, tradeChartCandle{
				TS:     bucket,
				Time:   time.UnixMilli(bucket).UTC().Format(time.RFC3339Nano),
				Open:   candle.Open,
				High:   candle.High,
				Low:    candle.Low,
				Close:  candle.Close,
				Volume: candle.Volume,
			})
			continue
		}
		current := &grouped[index]
		if chartDecimalGreater(candle.High, current.High) {
			current.High = candle.High
		}
		if chartDecimalLess(candle.Low, current.Low) {
			current.Low = candle.Low
		}
		current.Close = candle.Close
		if candle.Volume != "" {
			if current.Volume == "" {
				current.Volume = candle.Volume
			} else {
				current.Volume = addChartDecimals(current.Volume, candle.Volume)
			}
		}
	}
	sort.Slice(grouped, func(i, j int) bool { return grouped[i].TS < grouped[j].TS })
	return grouped
}

func chartDecimalGreater(left, right string) bool {
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	return leftOK && rightOK && leftValue.Cmp(rightValue) > 0
}

func chartDecimalLess(left, right string) bool {
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	return leftOK && rightOK && leftValue.Cmp(rightValue) < 0
}

func addChartDecimals(left, right string) string {
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	if !leftOK || !rightOK {
		return left
	}
	sum := new(big.Rat).Add(leftValue, rightValue).FloatString(18)
	sum = strings.TrimRight(strings.TrimRight(sum, "0"), ".")
	if sum == "" || sum == "-0" {
		return "0"
	}
	return sum
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
    .swatch.volume { background:linear-gradient(to top, rgba(47,129,247,.38) 0 50%, rgba(245,158,11,.38) 50% 100%); }
    canvas { width:100%; height:700px; display:block; cursor:grab; outline:none; touch-action:none; }
    canvas.dragging { cursor:grabbing; }
    canvas:focus { box-shadow:inset 0 0 0 2px rgba(31,111,235,.28); }
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
      <div><h1 id="title">币对 K 线回看</h1><div class="subtitle" id="subtitle">1 分钟周期 · 最近8小时 · 模拟盘 / 实盘价格对比</div></div>
      <div class="status" id="status">正在加载 K 线数据…</div>
    </header>
    <div class="toolbar" role="group" aria-label="K线周期">
      <span>K线周期：</span>
      <button type="button" data-bar="1m" class="active">1分钟</button>
      <button type="button" data-bar="2m">2分钟</button>
      <button type="button" data-bar="3m">3分钟</button>
      <button type="button" data-bar="5m">5分钟</button>
      <button type="button" data-bar="10m">10分钟</button>
      <button type="button" data-bar="15m">15分钟</button>
      <button type="button" data-bar="30m">30分钟</button>
      <button type="button" data-bar="1h">1小时</button>
      <button type="button" data-bar="4h">4小时</button>
      <button type="button" data-bar="8h">8小时</button>
      <button type="button" data-bar="12h">12小时</button>
    </div>
    <div class="toolbar" role="group" aria-label="回看范围">
      <span>回看范围：</span>
      <button type="button" data-hours="1">1 小时</button>
      <button type="button" data-hours="4">4 小时</button>
      <button type="button" data-hours="8" class="active">8 小时</button>
      <button type="button" data-hours="12">12 小时</button>
      <button type="button" data-hours="24">24 小时</button>
      <span id="window-text"></span>
    </div>
    <div class="legend">
      <span class="legend-item"><i class="swatch demo"></i>模拟盘 K 线</span>
      <span class="legend-item"><i class="swatch live"></i>实盘 K 线</span>
      <span class="legend-item"><i class="swatch volume"></i>成交量</span>
      <span class="legend-item"><i class="marker-dot entry"></i>开仓 / 买入</span>
      <span class="legend-item"><i class="marker-dot exit"></i>平仓 / 卖出</span>
    </div>
    <div class="chart-card"><canvas id="chart" tabindex="0" aria-label="模拟盘与实盘K线及成交量图"></canvas><div class="tooltip" id="tooltip"></div></div>
    <div class="trade-info" id="trade-info">-</div>
  </main>
  <script>
    const params = new URLSearchParams(window.location.search);
    const state = { data: null, bar: "1m", hours: 8, hover: null, viewFrom: null, viewTo: null, priceZoom: 1, priceOffset: 0, priceRange: 1, pricePlotHeight: 1, drag: null };
    let loadToken = 0;
    const canvas = document.getElementById("chart");
    const ctx = canvas.getContext("2d");
    const tooltip = document.getElementById("tooltip");
    const statusEl = document.getElementById("status");
    const titleEl = document.getElementById("title");
    const infoEl = document.getElementById("trade-info");
    const windowEl = document.getElementById("window-text");
    const subtitleEl = document.getElementById("subtitle");
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
    function barLabel() {
      return ({ "1m":"1 分钟", "2m":"2 分钟", "3m":"3 分钟", "5m":"5 分钟", "10m":"10 分钟", "15m":"15 分钟", "30m":"30 分钟", "1h":"1 小时", "4h":"4 小时", "8h":"8 小时", "12h":"12 小时" })[state.bar] || state.bar;
    }
    function updateSubtitle() {
      subtitleEl.textContent = barLabel() + "周期 · 最近" + state.hours + "小时 · 模拟盘 / 实盘价格对比";
    }
    function seriesPoints(series, from, to) {
      return (series && series.candles || []).filter(function (item) { return item.ts >= from && item.ts <= to; });
    }
    function baseWindow() {
      const end = new Date(state.data.to).getTime();
      const dataFrom = new Date(state.data.from).getTime();
      const from = end - state.hours * 60 * 60 * 1000;
      return { from: Number.isFinite(dataFrom) ? Math.max(from, dataFrom) : from, to: end };
    }
    function currentWindow() {
      const base = baseWindow();
      if (state.viewFrom == null || state.viewTo == null) return base;
      const baseSpan = Math.max(1, base.to - base.from);
      const span = Math.max(1, Math.min(baseSpan, state.viewTo - state.viewFrom));
      const from = Math.max(base.from, Math.min(state.viewFrom, base.to - span));
      return { from: from, to: from + span };
    }
    function resetView() {
      state.viewFrom = null;
      state.viewTo = null;
      state.priceZoom = 1;
      state.priceOffset = 0;
    }
    function setTimeWindow(from, to) {
      const base = baseWindow();
      const baseSpan = Math.max(1, base.to - base.from);
      const span = Math.max(1, Math.min(baseSpan, to - from));
      const clampedFrom = Math.max(base.from, Math.min(from, base.to - span));
      state.viewFrom = clampedFrom;
      state.viewTo = clampedFrom + span;
    }
    function barMinutes() {
      return ({ "1m":1, "2m":2, "3m":3, "5m":5, "10m":10, "15m":15, "30m":30, "1h":60, "4h":240, "8h":480, "12h":720 })[state.bar] || 1;
    }
    function zoomTime(factor, anchorRatio) {
      const range = currentWindow();
      const base = baseWindow();
      const span = range.to - range.from;
      const minSpan = Math.min(base.to - base.from, Math.max(5 * 60 * 1000, barMinutes() * 60 * 1000 * 12));
      const nextSpan = Math.max(minSpan, Math.min(base.to - base.from, span * factor));
      const ratio = Math.max(0, Math.min(1, anchorRatio == null ? 0.5 : anchorRatio));
      const anchor = range.from + span * ratio;
      setTimeWindow(anchor - nextSpan * ratio, anchor + nextSpan * (1 - ratio));
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
      const volumeHeight = Math.max(76, Math.min(116, height * 0.18));
      const volumeTop = bottom - volumeHeight;
      const priceBottom = volumeTop - 24;
      const plotWidth = Math.max(1, right - left), plotHeight = Math.max(1, priceBottom - top);
      let min = Infinity, max = -Infinity;
      points.forEach(function (item) { [item.low, item.high].forEach(function (value) { const parsed = number(value); if (parsed != null) { min = Math.min(min, parsed); max = Math.max(max, parsed); } }); });
      markers.forEach(function (item) { const parsed = number(item.price); if (parsed != null) { min = Math.min(min, parsed); max = Math.max(max, parsed); } });
      if (!Number.isFinite(min) || !Number.isFinite(max)) { min = 0; max = 1; }
      const padding = Math.max((max - min) * 0.08, max * 0.0005, 0.000001);
      min -= padding; max += padding;
      const priceCenter = (min + max) / 2;
      const priceRange = (max - min) / state.priceZoom;
      min = priceCenter + state.priceOffset - priceRange / 2;
      max = priceCenter + state.priceOffset + priceRange / 2;
      state.priceRange = priceRange;
      state.pricePlotHeight = plotHeight;
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
      ctx.beginPath(); ctx.moveTo(left, top); ctx.lineTo(left, priceBottom); ctx.lineTo(right, priceBottom); ctx.stroke();
      for (let i = 0; i <= 6; i += 1) {
        const ts = windowRange.from + (windowRange.to - windowRange.from) * i / 6;
        const xx = x(ts);
        ctx.strokeStyle = "#edf1f6"; ctx.beginPath(); ctx.moveTo(xx, top); ctx.lineTo(xx, bottom); ctx.stroke();
        ctx.fillStyle = "#667085"; ctx.fillText(formatAxisTime(ts, state.hours >= 24), Math.max(left, xx - 25), bottom + 23);
      }
      const maxVolume = points.reduce(function (value, item) { const volume = number(item.volume); return volume == null ? value : Math.max(value, volume); }, 0);
      ctx.strokeStyle = "#dce3ee";
      ctx.beginPath(); ctx.moveTo(left, volumeTop); ctx.lineTo(right, volumeTop); ctx.stroke();
      ctx.fillStyle = "#667085"; ctx.font = "11px -apple-system,BlinkMacSystemFont,Segoe UI,Microsoft YaHei,sans-serif"; ctx.fillText("成交量", left, volumeTop + 15);
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
      function drawVolume(items, color, offset) {
        if (maxVolume <= 0) return;
        const barWidth = Math.max(1, Math.min(5, plotWidth / Math.max(items.length, 1) * 0.72));
        items.forEach(function (item) {
          const volume = number(item.volume);
          if (volume == null || volume < 0) return;
          const xx = x(item.ts) + offset;
          const barHeight = Math.max(1, volume / maxVolume * (volumeHeight - 22));
          ctx.fillStyle = color; ctx.globalAlpha = 0.38;
          ctx.fillRect(xx - barWidth / 2, bottom - barHeight, barWidth, barHeight);
          ctx.globalAlpha = 1;
        });
      }
      drawSeries(demo, colors.demo, -1.5);
      drawSeries(live, colors.live, 1.5);
      drawVolume(demo, colors.demo, -1.5);
      drawVolume(live, colors.live, 1.5);
      markers.forEach(function (marker) {
        const price = number(marker.price); if (price == null) return;
        const xx = x(marker.ts), yy = y(price), color = colors[marker.kind] || "#344054";
        ctx.save(); ctx.strokeStyle = color; ctx.fillStyle = color; ctx.setLineDash([4, 4]); ctx.beginPath(); ctx.moveTo(xx, top); ctx.lineTo(xx, priceBottom); ctx.stroke(); ctx.restore();
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
      statusEl.innerHTML = errors.length ? '<span class="error">' + escapeHTML(errors.join("；")) + '</span>' : "已加载模拟盘 " + demoCount + " 根 / 实盘 " + liveCount + " 根 " + escapeHTML(state.data.bar || state.bar) + " K 线";
    }
    async function load() {
      const token = ++loadToken;
      try {
        const query = new URLSearchParams(params);
        query.set("bar", state.bar);
        query.set("hours", String(state.hours));
        updateSubtitle();
        const response = await fetch("/tvbot/chart/data?" + query.toString(), { headers: { "Accept": "application/json" } });
        const body = await response.json();
        if (!response.ok) throw new Error(body.message || body.error || "K 线数据加载失败");
        if (token !== loadToken) return;
        state.data = body;
        titleEl.textContent = (body.exchange === "binance" ? "Binance " : "OKX ") + body.inst_id + " K 线回看";
        renderInfo(); setStatus(); draw();
      } catch (error) {
        statusEl.innerHTML = '<span class="error">' + escapeHTML(error.message || error) + '</span>';
      }
    }
    document.querySelectorAll("button[data-bar]").forEach(function (button) { button.addEventListener("click", function () { state.bar = button.dataset.bar || "1m"; document.querySelectorAll("button[data-bar]").forEach(function (item) { item.classList.toggle("active", item === button); }); resetView(); statusEl.textContent = "正在加载 " + state.bar + " K 线数据…"; load(); }); });
    document.querySelectorAll("button[data-hours]").forEach(function (button) { button.addEventListener("click", function () { state.hours = Number(button.dataset.hours) || 8; document.querySelectorAll("button[data-hours]").forEach(function (item) { item.classList.toggle("active", item === button); }); resetView(); statusEl.textContent = "正在加载最近" + state.hours + "小时 K 线数据…"; load(); }); });
    function finishDrag() {
      if (!state.drag) return;
      state.drag = null;
      canvas.classList.remove("dragging");
    }
    canvas.addEventListener("pointerdown", function (event) {
      if (!state.data || event.button !== 0) return;
      event.preventDefault();
      canvas.focus();
      const range = currentWindow();
      state.drag = { x: event.clientX, y: event.clientY, from: range.from, to: range.to, priceOffset: state.priceOffset };
      canvas.classList.add("dragging");
      if (canvas.setPointerCapture) canvas.setPointerCapture(event.pointerId);
      tooltip.style.display = "none";
    });
    canvas.addEventListener("pointermove", function (event) {
      if (!state.data) return;
      if (state.drag) {
        event.preventDefault();
        const rect = canvas.getBoundingClientRect();
        const dx = event.clientX - state.drag.x;
        const dy = event.clientY - state.drag.y;
        const span = state.drag.to - state.drag.from;
        const plotWidth = Math.max(1, rect.width - 144);
        setTimeWindow(state.drag.from - dx / plotWidth * span, state.drag.to - dx / plotWidth * span);
        state.priceOffset = state.drag.priceOffset - dy / Math.max(1, state.pricePlotHeight || rect.height) * (state.priceRange || 1);
        state.hover = null;
        draw();
        return;
      }
      const rect = canvas.getBoundingClientRect(); const range = currentWindow(); const left = 68, right = rect.width - 76;
      const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left - left) / Math.max(1, right - left)));
      const ts = range.from + (range.to - range.from) * ratio;
      const all = (state.data.demo.candles || []).concat(state.data.live.candles || []);
      let nearest = null;
      all.forEach(function (item) { if (!nearest || Math.abs(item.ts - ts) < Math.abs(nearest.ts - ts)) nearest = item; });
      state.hover = nearest;
      if (nearest) { tooltip.style.display = "block"; tooltip.style.left = Math.min(rect.width - 230, Math.max(8, event.clientX - rect.left + 12)) + "px"; tooltip.style.top = Math.max(8, event.clientY - rect.top - 44) + "px"; tooltip.textContent = formatTime(nearest.time) + "　开 " + priceText(nearest.open) + "　高 " + priceText(nearest.high) + "　低 " + priceText(nearest.low) + "　收 " + priceText(nearest.close) + "　量 " + priceText(nearest.volume); }
      draw();
    });
    canvas.addEventListener("wheel", function (event) {
      if (!state.data) return;
      event.preventDefault();
      const rect = canvas.getBoundingClientRect();
      const ratio = (event.clientX - rect.left - 68) / Math.max(1, rect.width - 144);
      zoomTime(event.deltaY < 0 ? 0.8 : 1.25, ratio);
      draw();
    }, { passive: false });
    canvas.addEventListener("pointerup", finishDrag);
    canvas.addEventListener("pointercancel", finishDrag);
    window.addEventListener("pointerup", finishDrag);
    window.addEventListener("keydown", function (event) {
      if (!state.data || !["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(event.key)) return;
      event.preventDefault();
      if (event.key === "ArrowUp") state.priceZoom = Math.min(20, state.priceZoom * 1.25);
      if (event.key === "ArrowDown") state.priceZoom = Math.max(0.5, state.priceZoom / 1.25);
      if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
        const range = currentWindow();
        const shift = (range.to - range.from) * 0.2 * (event.key === "ArrowLeft" ? -1 : 1);
        setTimeWindow(range.from + shift, range.to + shift);
      }
      draw();
    });
    canvas.addEventListener("pointerleave", function () { if (state.drag) return; state.hover = null; tooltip.style.display = "none"; draw(); });
    window.addEventListener("resize", draw);
    load();
  </script>
</body>
</html>`

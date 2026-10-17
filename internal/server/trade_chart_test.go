package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
)

func TestTradeChartDataReturnsDemoLiveCandlesAndMarkers(t *testing.T) {
	srv := newTestServer(t)
	now := srv.now()
	base := now.Add(-time.Hour).UnixMilli()
	var sawDemo, sawLive bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/history-candles" {
			t.Fatalf("unexpected chart upstream path %s", r.URL.Path)
		}
		if r.URL.Query().Get("instId") != "ETH-USDT-SWAP" || r.URL.Query().Get("bar") != "1m" {
			t.Fatalf("unexpected chart query %s", r.URL.RawQuery)
		}
		isDemo := r.Header.Get("x-simulated-trading") == "1"
		if isDemo {
			sawDemo = true
		} else {
			sawLive = true
		}
		open := "2400"
		close := "2401"
		if isDemo {
			open = "2399"
			close = "2400"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[["` +
			strconvFormatInt(base+60000) + `","` + open + `","2405","2395","` + close + `","10","","","1"],["` +
			strconvFormatInt(base) + `","2398","2402","2390","2400","10","","","1"]]}`))
	}))
	defer upstream.Close()
	_, err := srv.ConfigStore.Update(func(cfg *config.Config) error {
		cfg.Trading.BaseURL = upstream.URL
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.OKXHTTPClient = upstream.Client()

	query := "/tvbot/chart/data?exchange=okx&inst_id=ETH-USDT-SWAP&side=long&entry_ts=" + strconvFormatInt(base) + "&exit_ts=" + strconvFormatInt(base+60000) + "&entry_px=2398&exit_px=2401"
	req := httptest.NewRequest(http.MethodGet, query, nil)
	req.Header.Set("X-Admin-Token", "admin")
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chart status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response tradeChartResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Demo.Candles) != 2 || len(response.Live.Candles) != 2 {
		t.Fatalf("unexpected demo/live candle counts: demo=%d live=%d", len(response.Demo.Candles), len(response.Live.Candles))
	}
	if len(response.Markers) != 2 || response.Markers[0].Kind != "entry" || response.Markers[1].Kind != "exit" {
		t.Fatalf("unexpected chart markers: %#v", response.Markers)
	}
	if response.Bar != tradeChartDefaultBar {
		t.Fatalf("unexpected default chart bar: %q", response.Bar)
	}
	if response.AccountEnv != config.EnvDemo {
		t.Fatalf("unexpected chart account environment: %q", response.AccountEnv)
	}
	from, err := time.Parse(time.RFC3339Nano, response.From)
	if err != nil {
		t.Fatalf("invalid chart from time: %v", err)
	}
	to, err := time.Parse(time.RFC3339Nano, response.To)
	if err != nil {
		t.Fatalf("invalid chart to time: %v", err)
	}
	if got := to.Sub(from); got != tradeChartHours*time.Hour {
		t.Fatalf("unexpected chart window: %s", got)
	}
	longReq := httptest.NewRequest(http.MethodGet, query+"&hours=24", nil)
	longReq.Header.Set("X-Admin-Token", "admin")
	longRecorder := httptest.NewRecorder()
	srv.ServeHTTP(longRecorder, longReq)
	if longRecorder.Code != http.StatusOK {
		t.Fatalf("24-hour chart status=%d body=%s", longRecorder.Code, longRecorder.Body.String())
	}
	var longResponse tradeChartResponse
	if err := json.Unmarshal(longRecorder.Body.Bytes(), &longResponse); err != nil {
		t.Fatal(err)
	}
	longFrom, err := time.Parse(time.RFC3339Nano, longResponse.From)
	if err != nil {
		t.Fatalf("invalid 24-hour chart from time: %v", err)
	}
	longTo, err := time.Parse(time.RFC3339Nano, longResponse.To)
	if err != nil {
		t.Fatalf("invalid 24-hour chart to time: %v", err)
	}
	if longResponse.Hours != 24 || longTo.Sub(longFrom) != 24*time.Hour {
		t.Fatalf("unexpected 24-hour chart window: hours=%d duration=%s", longResponse.Hours, longTo.Sub(longFrom))
	}
	if !sawDemo || !sawLive {
		t.Fatalf("expected demo and live upstream requests demo=%v live=%v", sawDemo, sawLive)
	}
}

func TestTradeChartPageAndAnalysisTableExposeIndependentChartWindow(t *testing.T) {
	srv := newTestServer(t)
	pageReq := httptest.NewRequest(http.MethodGet, "/tvbot/chart", nil)
	pageReq.Header.Set("X-Admin-Token", "admin")
	page := httptest.NewRecorder()
	srv.ServeHTTP(page, pageReq)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "模拟盘 K 线") || !strings.Contains(page.Body.String(), "实盘 K 线") || !strings.Contains(page.Body.String(), `data-series-toggle="demo"`) || !strings.Contains(page.Body.String(), `data-series-toggle="live"`) || !strings.Contains(page.Body.String(), "visibilityInitialized") || !strings.Contains(page.Body.String(), "applyAccountVisibility") || !strings.Contains(page.Body.String(), "最近8小时") || !strings.Contains(page.Body.String(), `data-hours="8" class="active"`) || !strings.Contains(page.Body.String(), `data-hours="12"`) || !strings.Contains(page.Body.String(), `data-hours="24"`) || !strings.Contains(page.Body.String(), `data-bar="12h"`) || !strings.Contains(page.Body.String(), "query.set(\"hours\", String(state.hours))") || !strings.Contains(page.Body.String(), "成交量") || !strings.Contains(page.Body.String(), `addEventListener("wheel"`) || !strings.Contains(page.Body.String(), `addEventListener("pointerdown"`) || !strings.Contains(page.Body.String(), "setPointerCapture") || !strings.Contains(page.Body.String(), "state.priceOffset = state.drag.priceOffset + dy /") || !strings.Contains(page.Body.String(), "ArrowUp") || !strings.Contains(page.Body.String(), "ArrowLeft") || !strings.Contains(page.Body.String(), "canvas") {
		t.Fatalf("chart page missing expected markers: status=%d", page.Code)
	}

	mainReq := httptest.NewRequest(http.MethodGet, "/tvbot", nil)
	mainReq.Header.Set("X-Admin-Token", "admin")
	main := httptest.NewRecorder()
	srv.ServeHTTP(main, mainReq)
	if main.Code != http.StatusOK || !strings.Contains(main.Body.String(), `data-trade-chart="true"`) || !strings.Contains(main.Body.String(), `window.open("/tvbot/chart?"`) || strings.Contains(main.Body.String(), "popup=yes") {
		t.Fatalf("main page is missing chart opener: status=%d", main.Code)
	}
}

func TestTradeChartIntervalsAndAggregation(t *testing.T) {
	for _, bar := range []string{"1m", "2m", "3m", "5m", "10m", "15m", "30m", "1h", "4h", "8h", "12h"} {
		interval, err := normalizeTradeChartBar(bar)
		if err != nil || interval.Bar != bar {
			t.Fatalf("normalizeTradeChartBar(%q)=(%#v,%v)", bar, interval, err)
		}
	}
	if _, err := normalizeTradeChartBar("2d"); err == nil {
		t.Fatal("unsupported chart bar should be rejected")
	}
	for _, hours := range []string{"1", "4", "8", "12", "24"} {
		if got, err := normalizeTradeChartHours(hours); err != nil || got != intValue(hours) {
			t.Fatalf("normalizeTradeChartHours(%q)=(%d,%v)", hours, got, err)
		}
	}
	if _, err := normalizeTradeChartHours("48"); err == nil {
		t.Fatal("unsupported chart hours should be rejected")
	}
	if got := tradeChartAccountEnv(config.Config{Trading: config.TradingConfig{Env: config.EnvDemo}}); got != config.EnvDemo {
		t.Fatalf("demo account environment=%q", got)
	}
	if got := tradeChartAccountEnv(config.Config{Trading: config.TradingConfig{Env: config.EnvLive}}); got != config.EnvLive {
		t.Fatalf("live account environment=%q", got)
	}

	base := (time.Unix(0, 0).Add(2 * time.Minute)).UnixMilli()
	interval, _ := normalizeTradeChartBar("2m")
	got := aggregateTradeChartCandles([]tradeChartCandle{
		{TS: base, Open: "100", High: "105", Low: "99", Close: "103", Volume: "1.25"},
		{TS: base + time.Minute.Milliseconds(), Open: "103", High: "108", Low: "101", Close: "106", Volume: "2.75"},
	}, interval)
	if len(got) != 1 || got[0].Open != "100" || got[0].High != "108" || got[0].Low != "99" || got[0].Close != "106" || got[0].Volume != "4" {
		t.Fatalf("unexpected aggregated candle: %#v", got)
	}
}

func intValue(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

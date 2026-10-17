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
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "模拟盘 K 线") || !strings.Contains(page.Body.String(), "最近8小时") || !strings.Contains(page.Body.String(), `data-hours="8" class="active"`) || !strings.Contains(page.Body.String(), "canvas") {
		t.Fatalf("chart page missing expected markers: status=%d", page.Code)
	}

	mainReq := httptest.NewRequest(http.MethodGet, "/tvbot", nil)
	mainReq.Header.Set("X-Admin-Token", "admin")
	main := httptest.NewRecorder()
	srv.ServeHTTP(main, mainReq)
	if main.Code != http.StatusOK || !strings.Contains(main.Body.String(), `data-trade-chart="true"`) || !strings.Contains(main.Body.String(), `window.open("/tvbot/chart?"`) {
		t.Fatalf("main page is missing chart opener: status=%d", main.Code)
	}
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

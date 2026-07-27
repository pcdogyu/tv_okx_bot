package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBalanceWindowQuerySupportsMinutesAndLegacyDays(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantMinutes int
		wantDays    int
	}{
		{name: "current", target: "/tvbot/balances/overview?minutes=0", wantMinutes: 0, wantDays: 0},
		{name: "one hour", target: "/tvbot/balances/overview?minutes=60", wantMinutes: 60, wantDays: 1},
		{name: "ninety days", target: "/tvbot/balances/overview?minutes=129600", wantMinutes: 129600, wantDays: 90},
		{name: "clamped", target: "/tvbot/balances/overview?minutes=999999", wantMinutes: 129600, wantDays: 90},
		{name: "legacy days", target: "/tvbot/balances/overview?days=3", wantMinutes: 4320, wantDays: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			gotMinutes, gotDays := balanceWindowQuery(req)
			if gotMinutes != tc.wantMinutes || gotDays != tc.wantDays {
				t.Fatalf("window=(%d,%d), want (%d,%d)", gotMinutes, gotDays, tc.wantMinutes, tc.wantDays)
			}
		})
	}
}

func TestCompactBalancePointsUsesCurrentAndHourlyLongWindows(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 15, 0, 0, time.UTC)
	points := []analysisBalancePoint{
		{Time: now.Add(-2 * time.Hour), TS: now.Add(-2 * time.Hour).UnixMilli(), Value: 100},
		{Time: now.Add(-90 * time.Minute), TS: now.Add(-90 * time.Minute).UnixMilli(), Value: 110},
		{Time: now.Add(-30 * time.Minute), TS: now.Add(-30 * time.Minute).UnixMilli(), Value: 120},
	}
	current := compactBalancePoints(points, 0)
	if len(current) != 1 || current[0].Value != 120 {
		t.Fatalf("current points=%#v", current)
	}
	longWindow := compactBalancePoints(points, 30*24*60)
	if len(longWindow) != 2 || longWindow[0].Value != 110 || longWindow[1].Value != 120 {
		t.Fatalf("hourly points len=%d points=%#v", len(longWindow), longWindow)
	}
	for _, point := range longWindow {
		if time.UnixMilli(point.TS).Minute() != 0 {
			t.Fatalf("point should be hour-bucketed: %#v", point)
		}
	}
}

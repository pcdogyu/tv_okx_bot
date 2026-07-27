package server

import (
	"strings"
	"testing"
)

func TestTVBotUSDTBalanceLayoutAndWindowButtons(t *testing.T) {
	if got := strings.Count(tvbotHTML, `class="analysis-metrics symbol-metrics exchange-balance-metrics"`); got != 2 {
		t.Fatalf("exchange balance metric blocks=%d, want 2", got)
	}
	if got := strings.Count(tvbotHTML, `class="balance-table-wrap"`); got != 2 {
		t.Fatalf("balance table wrappers=%d, want 2", got)
	}
	for _, marker := range []string{
		`.exchange-balance-metrics`,
		`.balance-table-wrap`,
		`data-balance-minutes="15"`,
		`data-balance-minutes="60"`,
		`data-balance-minutes="240"`,
		`data-balance-minutes="480"`,
		`data-balance-minutes="720"`,
		`data-balance-minutes="1440"`,
		`data-balance-minutes="2880"`,
		`data-balance-minutes="4320"`,
		`data-balance-minutes="10080"`,
	} {
		if !strings.Contains(tvbotHTML, marker) {
			t.Fatalf("tvbot ui missing %s", marker)
		}
	}
}

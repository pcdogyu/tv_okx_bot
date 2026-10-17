package trading

import "testing"

func TestOffsetLimitPrice(t *testing.T) {
	longPrice, err := OffsetLimitPrice(ActionLong, 2400, 0.5)
	if err != nil || longPrice != 2388 {
		t.Fatalf("long offset price=%v err=%v", longPrice, err)
	}
	shortPrice, err := OffsetLimitPrice(ActionShort, 2400, 1)
	if err != nil || shortPrice != 2424 {
		t.Fatalf("short offset price=%v err=%v", shortPrice, err)
	}
	for _, test := range []struct {
		action   Side
		current  float64
		distance float64
	}{
		{ActionLong, 2400, 0},
		{ActionLong, 2400, 100},
		{Side("other"), 2400, 1},
	} {
		if _, err := OffsetLimitPrice(test.action, test.current, test.distance); err == nil {
			t.Fatalf("invalid offset should fail: %#v", test)
		}
	}
}

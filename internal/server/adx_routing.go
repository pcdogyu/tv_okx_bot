package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

const adxAlertScriptVersion = "4.1.0"

type adxRoutingDecision struct {
	Versioned   bool
	IgnoreEntry bool
}

type adxAlertMetadata struct {
	Versioned bool
	ADX       float64
	Timeframe string
}

func applyTVOrderADXRouting(signal *trading.Signal) (adxRoutingDecision, error) {
	if signal == nil {
		return adxRoutingDecision{}, fmt.Errorf("signal is required")
	}
	if signal.SourceAction == "" {
		signal.SourceAction = signal.Action
	}
	metadata, err := parseTVOrderADXMetadata(firstADXMetadataText(signal.OrderIntent, signal.Text))
	if err != nil {
		return adxRoutingDecision{}, err
	}
	if !metadata.Versioned {
		return adxRoutingDecision{}, nil
	}

	signal.ADX = float64Pointer(metadata.ADX)
	decision := adxRoutingDecision{Versioned: true}
	switch {
	case metadata.ADX > 25:
		signal.MarketStrategy = trading.MarketStrategyTrend
	case metadata.ADX < 20:
		signal.MarketStrategy = trading.MarketStrategyScalp
		signal.Action = oppositeTradingSide(signal.Action)
		signal.PositionSide = oppositePositionSide(signal.PositionSide)
	default:
		signal.MarketStrategy = trading.MarketStrategyTransition
		if signal.PositionEffect == trading.PositionEffectClose {
			return decision, fmt.Errorf("ADX %s is in the transition range and cannot determine a close direction", trading.NormalizeFloat(metadata.ADX))
		}
		decision.IgnoreEntry = true
	}
	return decision, nil
}

func firstADXMetadataText(orderIntent, text string) string {
	for _, candidate := range []string{orderIntent, text} {
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, "script=") || strings.Contains(lower, "adx=") {
			return candidate
		}
	}
	return ""
}

func parseTVOrderADXMetadata(raw string) (adxAlertMetadata, error) {
	values := map[string][]string{}
	for _, token := range strings.Split(raw, "|") {
		key, value, ok := strings.Cut(strings.TrimSpace(token), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "script", "adx", "adx_tf":
			values[key] = append(values[key], strings.TrimSpace(value))
		}
	}

	scripts := values["script"]
	if len(scripts) == 0 {
		return adxAlertMetadata{}, nil
	}
	if len(scripts) != 1 {
		return adxAlertMetadata{}, fmt.Errorf("4.1.0 alert must contain exactly one script value")
	}
	if scripts[0] != adxAlertScriptVersion {
		return adxAlertMetadata{}, nil
	}
	if len(values["adx"]) != 1 {
		return adxAlertMetadata{}, fmt.Errorf("4.1.0 alert must contain exactly one ADX value")
	}
	if len(values["adx_tf"]) != 1 || values["adx_tf"][0] == "" {
		return adxAlertMetadata{}, fmt.Errorf("4.1.0 alert must contain exactly one ADX timeframe")
	}
	adx, err := strconv.ParseFloat(values["adx"][0], 64)
	if err != nil || math.IsNaN(adx) || math.IsInf(adx, 0) || adx < 0 || adx > 100 {
		return adxAlertMetadata{}, fmt.Errorf("4.1.0 alert ADX %q must be a number from 0 to 100", values["adx"][0])
	}
	return adxAlertMetadata{
		Versioned: true,
		ADX:       adx,
		Timeframe: values["adx_tf"][0],
	}, nil
}

func oppositeTradingSide(side trading.Side) trading.Side {
	switch side {
	case trading.ActionLong:
		return trading.ActionShort
	case trading.ActionShort:
		return trading.ActionLong
	default:
		return side
	}
}

func oppositePositionSide(side string) string {
	switch side {
	case trading.PositionSideLong:
		return trading.PositionSideShort
	case trading.PositionSideShort:
		return trading.PositionSideLong
	default:
		return side
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}

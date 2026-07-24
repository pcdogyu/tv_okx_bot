package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
)

var (
	errPositionNotOpen        = errors.New("position is not open")
	positionClosePollInterval = 5 * time.Second
	positionCloseLimitTimeout = 300 * time.Second
	positionCloseJobs         = newPositionCloseRegistry()
	positionCloseSeq          uint64
)

type positionsResponse struct {
	OK          bool           `json:"ok"`
	APIID       string         `json:"api_id"`
	InstType    string         `json:"inst_type"`
	Count       int            `json:"count"`
	RefreshedAt time.Time      `json:"refreshed_at"`
	Positions   []okx.Position `json:"positions"`
}

type positionCloseRequest struct {
	APIID   string `json:"api_id"`
	InstID  string `json:"inst_id"`
	PosSide string `json:"pos_side"`
	Mode    string `json:"mode"`
}

type positionCloseResponse struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	APIID   string `json:"api_id"`
	InstID  string `json:"inst_id"`
	PosSide string `json:"pos_side,omitempty"`
	OrdID   string `json:"ord_id,omitempty"`
	ClOrdID string `json:"cl_ord_id,omitempty"`
	Px      string `json:"px,omitempty"`
	Message string `json:"message,omitempty"`
}

type positionCloseOrder struct {
	Position okx.Position
	Ack      okx.OrderAck
	Px       string
}

type positionCloseRegistry struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func newPositionCloseRegistry() *positionCloseRegistry {
	return &positionCloseRegistry{active: map[string]struct{}{}}
}

func (r *positionCloseRegistry) start(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.active[key]; ok {
		return false
	}
	r.active[key] = struct{}{}
	return true
}

func (r *positionCloseRegistry) done(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, key)
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	if s.OKXCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
		return
	}
	instType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("inst_type")))
	if instType == "" {
		instType = "SWAP"
	}
	apiID := strings.TrimSpace(r.URL.Query().Get("api_id"))
	cfg := s.ConfigStore.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	resp, err := s.fetchPositions(ctx, cfg, apiID, instType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "positions_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePositionClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	if s.OKXCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
		return
	}
	var req positionCloseRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	req.APIID = strings.TrimSpace(req.APIID)
	req.InstID = strings.ToUpper(strings.TrimSpace(req.InstID))
	req.PosSide = normalizePosSide(req.PosSide)
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.InstID == "" {
		writeError(w, http.StatusBadRequest, "invalid_position_close", "inst_id is required")
		return
	}
	if req.Mode != "market" && req.Mode != "limit" {
		writeError(w, http.StatusBadRequest, "invalid_position_close", "mode must be market or limit")
		return
	}
	cfg := s.ConfigStore.Get()
	client, apiID, err := s.okxClientForCredentials(cfg, req.APIID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credentials_failed", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	position, err := currentOpenPosition(ctx, client, req.InstID, req.PosSide)
	if err != nil {
		if errors.Is(err, errPositionNotOpen) {
			writeError(w, http.StatusConflict, "position_not_open", err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "positions_failed", err.Error())
		return
	}
	switch req.Mode {
	case "market":
		order, err := placeMarketPositionClose(ctx, cfg, client, position)
		if err != nil {
			writeError(w, http.StatusBadGateway, "position_close_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, positionCloseResponse{
			OK:      true,
			Status:  "submitted",
			Mode:    req.Mode,
			APIID:   apiID,
			InstID:  order.Position.InstID,
			PosSide: normalizePosSide(order.Position.PosSide),
			OrdID:   order.Ack.OrdID,
			ClOrdID: order.Ack.ClOrdID,
			Message: "market close order submitted",
		})
	case "limit":
		key := positionCloseKey(apiID, position.InstID, position.PosSide)
		if !positionCloseJobs.start(key) {
			writeError(w, http.StatusConflict, "position_close_running", "limit close is already running for this position")
			return
		}
		order, err := placeLimitPositionClose(ctx, cfg, client, position)
		if err != nil {
			positionCloseJobs.done(key)
			writeError(w, http.StatusBadGateway, "position_close_failed", err.Error())
			return
		}
		go s.watchLimitPositionClose(apiID, cfg, client, order)
		writeJSON(w, http.StatusAccepted, positionCloseResponse{
			OK:      true,
			Status:  "running",
			Mode:    req.Mode,
			APIID:   apiID,
			InstID:  order.Position.InstID,
			PosSide: normalizePosSide(order.Position.PosSide),
			OrdID:   order.Ack.OrdID,
			ClOrdID: order.Ack.ClOrdID,
			Px:      order.Px,
			Message: "limit close order started",
		})
	}
}

func (s *Server) fetchPositions(ctx context.Context, cfg config.Config, requestedAPIID, instType string) (positionsResponse, error) {
	creds, apiID, err := s.OKXCredentials.OKXCredentials(requestedAPIID)
	if err != nil {
		return positionsResponse{}, err
	}
	client := okx.Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  s.okxHTTPClient(),
	}
	positions, _, err := client.Positions(ctx, instType)
	if err != nil {
		return positionsResponse{}, err
	}
	positions = openPositions(positions)
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].InstID == positions[j].InstID {
			return positions[i].PosSide < positions[j].PosSide
		}
		return positions[i].InstID < positions[j].InstID
	})
	return positionsResponse{
		OK:          true,
		APIID:       apiID,
		InstType:    instType,
		Count:       len(positions),
		RefreshedAt: s.now(),
		Positions:   positions,
	}, nil
}

func (s *Server) okxClientForCredentials(cfg config.Config, requestedAPIID string) (okx.Client, string, error) {
	creds, apiID, err := s.OKXCredentials.OKXCredentials(requestedAPIID)
	if err != nil {
		return okx.Client{}, "", err
	}
	return okx.Client{
		BaseURL:     cfg.OKXBaseURL(),
		Credentials: creds,
		Demo:        cfg.DemoTradingHeaderEnabled(),
		HTTPClient:  s.okxHTTPClient(),
	}, apiID, nil
}

func openPositions(positions []okx.Position) []okx.Position {
	out := make([]okx.Position, 0, len(positions))
	for _, position := range positions {
		if isOpenPosition(position.Pos) {
			out = append(out, position)
		}
	}
	return out
}

func currentOpenPosition(ctx context.Context, client okx.Client, instID, posSide string) (okx.Position, error) {
	instID = strings.ToUpper(strings.TrimSpace(instID))
	posSide = normalizePosSide(posSide)
	positions, _, err := client.Positions(ctx, "SWAP")
	if err != nil {
		return okx.Position{}, err
	}
	for _, position := range positions {
		if !strings.EqualFold(position.InstID, instID) || !isOpenPosition(position.Pos) {
			continue
		}
		if posSide == "" || normalizePosSide(position.PosSide) == posSide {
			return position, nil
		}
	}
	return okx.Position{}, fmt.Errorf("%w: %s %s", errPositionNotOpen, instID, posSide)
}

func placeMarketPositionClose(ctx context.Context, cfg config.Config, client okx.Client, position okx.Position) (positionCloseOrder, error) {
	req, err := positionCloseOrderRequest(cfg, position, "market", "")
	if err != nil {
		return positionCloseOrder{}, err
	}
	ack, _, err := client.PlaceOrder(ctx, req)
	if err != nil {
		return positionCloseOrder{}, err
	}
	return positionCloseOrder{Position: position, Ack: ack}, nil
}

func placeLimitPositionClose(ctx context.Context, cfg config.Config, client okx.Client, position okx.Position) (positionCloseOrder, error) {
	px, err := limitClosePrice(ctx, client, position)
	if err != nil {
		return positionCloseOrder{}, err
	}
	req, err := positionCloseOrderRequest(cfg, position, "limit", px)
	if err != nil {
		return positionCloseOrder{}, err
	}
	ack, _, err := client.PlaceOrder(ctx, req)
	if err != nil {
		return positionCloseOrder{}, err
	}
	return positionCloseOrder{Position: position, Ack: ack, Px: px}, nil
}

func (s *Server) watchLimitPositionClose(apiID string, cfg config.Config, client okx.Client, active positionCloseOrder) {
	key := positionCloseKey(apiID, active.Position.InstID, active.Position.PosSide)
	defer positionCloseJobs.done(key)

	poll := time.NewTicker(positionClosePollInterval)
	defer poll.Stop()
	timeout := time.NewTimer(positionCloseLimitTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-poll.C:
			next, closed, err := refreshLimitPositionClose(cfg, client, active)
			if err != nil {
				s.logPositionCloseError("limit position close refresh failed", err, active.Position)
				continue
			}
			if closed {
				return
			}
			active = next
		case <-timeout.C:
			if err := fallbackMarketPositionClose(cfg, client, active); err != nil {
				s.logPositionCloseError("limit position close fallback failed", err, active.Position)
			}
			return
		}
	}
}

func refreshLimitPositionClose(cfg config.Config, client okx.Client, active positionCloseOrder) (positionCloseOrder, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	position, err := currentOpenPosition(ctx, client, active.Position.InstID, active.Position.PosSide)
	if err != nil {
		if errors.Is(err, errPositionNotOpen) {
			return active, true, nil
		}
		return active, false, err
	}
	nextPx, err := limitClosePrice(ctx, client, position)
	if err != nil {
		return active, false, err
	}
	if nextPx == active.Px {
		active.Position = position
		return active, false, nil
	}
	if err := cancelPositionCloseOrder(ctx, client, active); err != nil {
		if closed, checkErr := positionClosed(ctx, client, active.Position); checkErr == nil && closed {
			return active, true, nil
		}
		return active, false, err
	}
	next, err := placeLimitPositionClose(ctx, cfg, client, position)
	if err != nil {
		return active, false, err
	}
	return next, false, nil
}

func fallbackMarketPositionClose(cfg config.Config, client okx.Client, active positionCloseOrder) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := cancelPositionCloseOrder(ctx, client, active); err != nil {
		closed, checkErr := positionClosed(ctx, client, active.Position)
		if checkErr == nil && closed {
			return nil
		}
	}
	position, err := currentOpenPosition(ctx, client, active.Position.InstID, active.Position.PosSide)
	if err != nil {
		if errors.Is(err, errPositionNotOpen) {
			return nil
		}
		return err
	}
	_, err = placeMarketPositionClose(ctx, cfg, client, position)
	return err
}

func positionClosed(ctx context.Context, client okx.Client, position okx.Position) (bool, error) {
	_, err := currentOpenPosition(ctx, client, position.InstID, position.PosSide)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, errPositionNotOpen) {
		return true, nil
	}
	return false, err
}

func cancelPositionCloseOrder(ctx context.Context, client okx.Client, active positionCloseOrder) error {
	if strings.TrimSpace(active.Ack.OrdID) == "" && strings.TrimSpace(active.Ack.ClOrdID) == "" {
		return nil
	}
	req := okx.CancelOrderRequest{InstID: active.Position.InstID}
	if strings.TrimSpace(active.Ack.OrdID) != "" {
		req.OrdID = active.Ack.OrdID
	} else {
		req.ClOrdID = active.Ack.ClOrdID
	}
	_, _, err := client.CancelOrder(ctx, req)
	return err
}

func positionCloseOrderRequest(cfg config.Config, position okx.Position, ordType, px string) (okx.PlaceOrderRequest, error) {
	side, err := closeOrderSide(position)
	if err != nil {
		return okx.PlaceOrderRequest{}, err
	}
	size := absolutePositionSize(position.Pos)
	if size == "" || size == "0" {
		return okx.PlaceOrderRequest{}, errPositionNotOpen
	}
	tdMode := strings.ToLower(strings.TrimSpace(position.MgnMode))
	if tdMode == "" {
		tdMode = cfg.MarginMode()
	}
	posSide := normalizePosSide(position.PosSide)
	req := okx.PlaceOrderRequest{
		InstID:     strings.ToUpper(strings.TrimSpace(position.InstID)),
		TDMode:     tdMode,
		ClOrdID:    nextPositionCloseClOrdID(),
		Side:       side,
		OrdType:    strings.ToLower(strings.TrimSpace(ordType)),
		Px:         px,
		Sz:         size,
		ReduceOnly: posSide == "" || posSide == "net",
	}
	if posSide != "" && posSide != "net" {
		req.PosSide = posSide
	}
	return req, nil
}

func limitClosePrice(ctx context.Context, client okx.Client, position okx.Position) (string, error) {
	inst, err := client.SwapInstrument(ctx, position.InstID)
	if err != nil {
		return "", err
	}
	ticker, _, err := client.MarketTicker(ctx, position.InstID)
	if err != nil {
		return "", err
	}
	side, err := closeOrderSide(position)
	if err != nil {
		return "", err
	}
	return priceOneTickFromMid(ticker, inst.TickSz, side)
}

func priceOneTickFromMid(ticker okx.Ticker, tickRaw, side string) (string, error) {
	tick, err := strconv.ParseFloat(strings.TrimSpace(tickRaw), 64)
	if err != nil || tick <= 0 {
		return "", fmt.Errorf("invalid tick size %q", tickRaw)
	}
	bid, bidErr := strconv.ParseFloat(strings.TrimSpace(ticker.BidPx), 64)
	ask, askErr := strconv.ParseFloat(strings.TrimSpace(ticker.AskPx), 64)
	if bidErr != nil || askErr != nil || bid <= 0 || ask <= 0 {
		last, lastErr := strconv.ParseFloat(strings.TrimSpace(ticker.Last), 64)
		if lastErr != nil || last <= 0 {
			return "", fmt.Errorf("invalid ticker bid/ask for %s", ticker.InstID)
		}
		bid, ask = last, last
	}
	mid := (bid + ask) / 2
	switch side {
	case "sell":
		return formatPriceToTick(mid-tick, tick, tickRaw, false)
	case "buy":
		return formatPriceToTick(mid+tick, tick, tickRaw, true)
	default:
		return "", fmt.Errorf("unsupported close side %q", side)
	}
}

func formatPriceToTick(value, tick float64, tickRaw string, roundUp bool) (string, error) {
	if value <= 0 || tick <= 0 {
		return "", fmt.Errorf("invalid close price %.12g for tick %.12g", value, tick)
	}
	scaled := value / tick
	if roundUp {
		value = math.Ceil(scaled-1e-9) * tick
	} else {
		value = math.Floor(scaled+1e-9) * tick
	}
	if value <= 0 {
		return "", fmt.Errorf("invalid close price after tick rounding")
	}
	decimals := decimalsFromDecimalString(tickRaw)
	out := strconv.FormatFloat(value, 'f', decimals, 64)
	return trimDecimalZeros(out), nil
}

func closeOrderSide(position okx.Position) (string, error) {
	switch normalizePosSide(position.PosSide) {
	case "long":
		return "sell", nil
	case "short":
		return "buy", nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(position.Pos), 64)
	if err != nil {
		if strings.HasPrefix(strings.TrimSpace(position.Pos), "-") {
			return "buy", nil
		}
		return "sell", nil
	}
	if value < 0 {
		return "buy", nil
	}
	if value > 0 {
		return "sell", nil
	}
	return "", errPositionNotOpen
}

func absolutePositionSize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return strings.TrimPrefix(raw, "-")
	}
	return trimDecimalZeros(strconv.FormatFloat(math.Abs(value), 'f', -1, 64))
}

func normalizePosSide(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "net" {
		return ""
	}
	return raw
}

func positionCloseKey(apiID, instID, posSide string) string {
	side := normalizePosSide(posSide)
	if side == "" {
		side = "net"
	}
	return strings.TrimSpace(apiID) + "|" + strings.ToUpper(strings.TrimSpace(instID)) + "|" + side
}

func nextPositionCloseClOrdID() string {
	seq := atomic.AddUint64(&positionCloseSeq, 1) % 1000000
	return fmt.Sprintf("PC%d%06d", time.Now().UTC().UnixMilli(), seq)
}

func decimalsFromDecimalString(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0
	}
	if i := strings.Index(raw, "e-"); i >= 0 {
		n, err := strconv.Atoi(raw[i+2:])
		if err == nil && n > 0 {
			return n
		}
	}
	if i := strings.Index(raw, "."); i >= 0 {
		return len(strings.TrimRight(raw[i+1:], "0"))
	}
	return 0
}

func trimDecimalZeros(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, ".") {
		raw = strings.TrimRight(strings.TrimRight(raw, "0"), ".")
	}
	if raw == "-0" || raw == "" {
		return "0"
	}
	return raw
}

func (s *Server) logPositionCloseError(message string, err error, position okx.Position) {
	if s.Logger == nil {
		return
	}
	s.Logger.Warn(message,
		"error", err,
		"inst_id", position.InstID,
		"pos_side", position.PosSide,
	)
}

func isOpenPosition(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return true
	}
	return math.Abs(value) > 0
}

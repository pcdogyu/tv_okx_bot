package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/binance"
	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

var (
	errPositionNotOpen             = errors.New("position is not open")
	errPendingOrderNoRemaining     = errors.New("pending order has no remaining size")
	positionClosePollInterval      = 5 * time.Second
	positionCloseLimitTimeout      = 300 * time.Second
	lowMarginPositionCheckInterval = time.Minute
	lowMarginPositionThresholdUSDT = 10.0
	positionCloseJobs              = newPositionCloseRegistry()
	pendingOrderChaseInterval      = 5 * time.Second
	pendingOrderChaseTimeout       = 60 * time.Second
	pendingOrderChaseJobs          = newPendingOrderChaseRegistry()
	positionCloseSeq               uint64
	pendingOrderMarketSeq          uint64
)

type positionsResponse struct {
	OK          bool           `json:"ok"`
	Exchange    string         `json:"exchange"`
	APIID       string         `json:"api_id"`
	InstType    string         `json:"inst_type"`
	Count       int            `json:"count"`
	RefreshedAt time.Time      `json:"refreshed_at"`
	Positions   []okx.Position `json:"positions"`
}

type pendingOrdersResponse struct {
	OK          bool               `json:"ok"`
	Exchange    string             `json:"exchange"`
	APIID       string             `json:"api_id"`
	InstType    string             `json:"inst_type"`
	Count       int                `json:"count"`
	RefreshedAt time.Time          `json:"refreshed_at"`
	Orders      []pendingOrderView `json:"orders"`
}

type pendingOrderView struct {
	okx.PendingOrder
	MidPx      string `json:"mid_px,omitempty"`
	ChasePx    string `json:"chase_px,omitempty"`
	PriceError string `json:"price_error,omitempty"`
	Chasing    bool   `json:"chasing"`
}

type positionCloseRequest struct {
	APIID   string `json:"api_id"`
	InstID  string `json:"inst_id"`
	PosSide string `json:"pos_side"`
	Mode    string `json:"mode"`
}

type pendingOrderChaseRequest struct {
	Exchange string `json:"exchange"`
	APIID    string `json:"api_id"`
	InstID   string `json:"inst_id"`
	OrdID    string `json:"ord_id"`
	ClOrdID  string `json:"cl_ord_id"`
}

type pendingOrderChaseResponse struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	APIID   string `json:"api_id"`
	InstID  string `json:"inst_id"`
	OrdID   string `json:"ord_id,omitempty"`
	ClOrdID string `json:"cl_ord_id,omitempty"`
	MidPx   string `json:"mid_px,omitempty"`
	Px      string `json:"px,omitempty"`
	Message string `json:"message,omitempty"`
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

type pendingOrderChaseRegistry struct {
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func newPositionCloseRegistry() *positionCloseRegistry {
	return &positionCloseRegistry{active: map[string]struct{}{}}
}

func newPendingOrderChaseRegistry() *pendingOrderChaseRegistry {
	return &pendingOrderChaseRegistry{active: map[string]context.CancelFunc{}}
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

func (r *pendingOrderChaseRegistry) start(key string, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.active[key]; ok {
		return false
	}
	r.active[key] = cancel
	return true
}

func (r *pendingOrderChaseRegistry) stop(key string) bool {
	r.mu.Lock()
	cancel, ok := r.active[key]
	if ok {
		delete(r.active, key)
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *pendingOrderChaseRegistry) done(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, key)
}

func (r *pendingOrderChaseRegistry) activeKey(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[key]
	return ok
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	exchange := trading.NormalizeExchange(r.URL.Query().Get("exchange"))
	instType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("inst_type")))
	if instType == "" {
		instType = "SWAP"
	}
	apiID := strings.TrimSpace(r.URL.Query().Get("api_id"))
	cfg := s.ConfigStore.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var (
		resp positionsResponse
		err  error
	)
	if exchange == trading.ExchangeBinance {
		if s.BinanceCredentials == nil {
			writeError(w, http.StatusServiceUnavailable, "not_configured", "Binance credential store is not configured")
			return
		}
		resp, err = s.fetchBinancePositions(ctx, cfg, apiID)
	} else {
		if s.OKXCredentials == nil {
			writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
			return
		}
		resp, err = s.fetchPositions(ctx, cfg, apiID, instType)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "positions_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePendingOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	exchange := trading.NormalizeExchange(r.URL.Query().Get("exchange"))
	instType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("inst_type")))
	if instType == "" {
		instType = "SWAP"
	}
	apiID := strings.TrimSpace(r.URL.Query().Get("api_id"))
	cfg := s.ConfigStore.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var (
		resp pendingOrdersResponse
		err  error
	)
	if exchange == trading.ExchangeBinance {
		if s.BinanceCredentials == nil {
			writeError(w, http.StatusServiceUnavailable, "not_configured", "Binance credential store is not configured")
			return
		}
		resp, err = s.fetchBinancePendingOrders(ctx, cfg, apiID)
	} else {
		if s.OKXCredentials == nil {
			writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
			return
		}
		resp, err = s.fetchPendingOrders(ctx, cfg, apiID, instType)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "pending_orders_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePendingOrderChase(w http.ResponseWriter, r *http.Request) {
	s.handlePendingOrderChaseAction(w, r, true)
}

func (s *Server) handlePendingOrderChaseStop(w http.ResponseWriter, r *http.Request) {
	s.handlePendingOrderChaseAction(w, r, false)
}

func (s *Server) handlePendingOrderCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	var req pendingOrderChaseRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	req.normalize()
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pending_order_cancel", err.Error())
		return
	}
	if req.Exchange != trading.ExchangeBinance {
		writeError(w, http.StatusBadRequest, "unsupported_exchange", "only Binance pending order cancel is supported")
		return
	}
	if s.BinanceCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "Binance credential store is not configured")
		return
	}
	cfg := s.ConfigStore.Get()
	if !cfg.BinanceLiveTradingAllowedByEnvironment() {
		writeError(w, http.StatusForbidden, "live_trading_disabled", "Binance live trading is not allowed by environment")
		return
	}
	client, apiID, err := s.binanceClientForCredentials(cfg, req.APIID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credentials_failed", err.Error())
		return
	}
	req.APIID = apiID
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	order, found, err := currentBinancePendingOrder(ctx, client, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "pending_order_cancel_failed", err.Error())
		return
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   apiID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		pendingOrderChaseJobs.stop(pendingOrderChaseKey(req))
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.OrdID = order.OrdID
	resp.ClOrdID = order.ClOrdID
	if err := cancelBinancePendingOrder(ctx, client, order); err != nil {
		writeError(w, http.StatusBadGateway, "pending_order_cancel_failed", err.Error())
		return
	}
	pendingOrderChaseJobs.stop(pendingOrderChaseKey(req))
	resp.Status = "canceled"
	resp.Message = "pending order canceled"
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePendingOrderChaseAction(w http.ResponseWriter, r *http.Request, start bool) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	var req pendingOrderChaseRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	req.normalize()
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pending_order_chase", err.Error())
		return
	}
	cfg := s.ConfigStore.Get()
	if req.Exchange == trading.ExchangeBinance {
		s.handleBinancePendingOrderChaseAction(w, r, start, cfg, req)
		return
	}
	if s.OKXCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
		return
	}
	client, apiID, err := s.okxClientForCredentials(cfg, req.APIID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credentials_failed", err.Error())
		return
	}
	req.APIID = apiID
	key := pendingOrderChaseKey(req)
	if !start {
		stopped := pendingOrderChaseJobs.stop(key)
		status := "not_running"
		message := "pending order chase was not running"
		if stopped {
			status = "stopped"
			message = "pending order chase stopped"
		}
		writeJSON(w, http.StatusOK, pendingOrderChaseResponse{
			OK:      true,
			Status:  status,
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: message,
		})
		return
	}
	if pendingOrderChaseJobs.activeKey(key) {
		writeJSON(w, http.StatusOK, pendingOrderChaseResponse{
			OK:      true,
			Status:  "running",
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: "pending order chase is already running",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	req, result, closed, err := preparePendingOrderChase(ctx, cfg, client, req)
	cancel()
	if err != nil {
		writeError(w, http.StatusBadGateway, "pending_order_chase_failed", err.Error())
		return
	}
	if closed {
		writeJSON(w, http.StatusConflict, pendingOrderChaseResponse{
			OK:      false,
			Status:  "finished",
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: "pending order is no longer open",
		})
		return
	}
	key = pendingOrderChaseKey(req)
	chaseCtx, chaseCancel := context.WithCancel(context.Background())
	if !pendingOrderChaseJobs.start(key, chaseCancel) {
		chaseCancel()
		result.Status = "running"
		result.Message = "pending order chase is already running"
		writeJSON(w, http.StatusOK, result)
		return
	}
	go s.watchPendingOrderChase(chaseCtx, cfg, client, req)
	result.Status = "running"
	result.Message = "pending order chase started; market fallback after timeout"
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleBinancePendingOrderChaseAction(w http.ResponseWriter, r *http.Request, start bool, cfg config.Config, req pendingOrderChaseRequest) {
	if s.BinanceCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "Binance credential store is not configured")
		return
	}
	if !cfg.BinanceLiveTradingAllowedByEnvironment() {
		writeError(w, http.StatusForbidden, "live_trading_disabled", "Binance live trading is not allowed by environment")
		return
	}
	client, apiID, err := s.binanceClientForCredentials(cfg, req.APIID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "credentials_failed", err.Error())
		return
	}
	req.APIID = apiID
	key := pendingOrderChaseKey(req)
	if !start {
		stopped := pendingOrderChaseJobs.stop(key)
		status := "not_running"
		message := "pending order chase was not running"
		if stopped {
			status = "stopped"
			message = "pending order chase stopped"
		}
		writeJSON(w, http.StatusOK, pendingOrderChaseResponse{
			OK:      true,
			Status:  status,
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: message,
		})
		return
	}
	if pendingOrderChaseJobs.activeKey(key) {
		writeJSON(w, http.StatusOK, pendingOrderChaseResponse{
			OK:      true,
			Status:  "running",
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: "pending order chase is already running",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	result, closed, err := prepareBinancePendingOrderChase(ctx, client, req)
	cancel()
	if err != nil {
		writeError(w, http.StatusBadGateway, "pending_order_chase_failed", err.Error())
		return
	}
	if closed {
		writeJSON(w, http.StatusConflict, pendingOrderChaseResponse{
			OK:      false,
			Status:  "finished",
			APIID:   apiID,
			InstID:  req.InstID,
			OrdID:   req.OrdID,
			ClOrdID: req.ClOrdID,
			Message: "pending order is no longer open",
		})
		return
	}
	chaseCtx, chaseCancel := context.WithCancel(context.Background())
	if !pendingOrderChaseJobs.start(key, chaseCancel) {
		chaseCancel()
		result.Status = "running"
		result.Message = "pending order chase is already running"
		writeJSON(w, http.StatusOK, result)
		return
	}
	go s.watchBinancePendingOrderChase(chaseCtx, client, req)
	result.Status = "running"
	result.Message = "pending order chase started; market fallback after timeout"
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) StartLowMarginPositionMonitor(ctx context.Context) {
	if s.ConfigStore == nil || s.OKXCredentials == nil {
		return
	}
	go s.runLowMarginPositionMonitor(ctx, lowMarginPositionCheckInterval)
}

func (s *Server) runLowMarginPositionMonitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = lowMarginPositionCheckInterval
	}
	s.closeLowMarginPositions(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.closeLowMarginPositions(ctx)
		}
	}
}

func (s *Server) closeLowMarginPositions(ctx context.Context) {
	ids := configuredAPIIDs(s.OKXCredentials.Status())
	if len(ids) == 0 {
		return
	}
	cfg := s.ConfigStore.Get()
	for _, requestedAPIID := range ids {
		scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.closeLowMarginPositionsForAPI(scanCtx, cfg, requestedAPIID)
		cancel()
		if err != nil && s.Logger != nil {
			s.Logger.Warn("failed to scan low margin positions", "api_id", requestedAPIID, "error", err)
		}
	}
}

func (s *Server) closeLowMarginPositionsForAPI(ctx context.Context, cfg config.Config, requestedAPIID string) error {
	client, apiID, err := s.okxClientForCredentials(cfg, requestedAPIID)
	if err != nil {
		return err
	}
	positions, _, err := client.Positions(ctx, "SWAP")
	if err != nil {
		return err
	}
	for _, position := range openPositions(positions) {
		margin, low := lowMarginPosition(position, lowMarginPositionThresholdUSDT)
		if !low {
			continue
		}
		orderCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		order, started, err := s.startLimitPositionClose(orderCtx, apiID, cfg, client, position)
		cancel()
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn("failed to start low margin position close", "api_id", apiID, "inst_id", position.InstID, "pos_side", position.PosSide, "margin", margin, "error", err)
			}
			continue
		}
		if started && s.Logger != nil {
			s.Logger.Info("low margin position close started", "api_id", apiID, "inst_id", position.InstID, "pos_side", position.PosSide, "margin", margin, "px", order.Px, "ord_id", order.Ack.OrdID)
		}
	}
	return nil
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
		order, started, err := s.startLimitPositionClose(ctx, apiID, cfg, client, position)
		if err != nil {
			writeError(w, http.StatusBadGateway, "position_close_failed", err.Error())
			return
		}
		if !started {
			writeError(w, http.StatusConflict, "position_close_running", "limit close is already running for this position")
			return
		}
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

func (s *Server) startLimitPositionClose(ctx context.Context, apiID string, cfg config.Config, client okx.Client, position okx.Position) (positionCloseOrder, bool, error) {
	key := positionCloseKey(apiID, position.InstID, position.PosSide)
	if !positionCloseJobs.start(key) {
		return positionCloseOrder{}, false, nil
	}
	order, err := placeLimitPositionClose(ctx, cfg, client, position)
	if err != nil {
		positionCloseJobs.done(key)
		return positionCloseOrder{}, false, err
	}
	go s.watchLimitPositionClose(apiID, cfg, client, order)
	return order, true, nil
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
		Exchange:    trading.ExchangeOKX,
		APIID:       apiID,
		InstType:    instType,
		Count:       len(positions),
		RefreshedAt: s.now(),
		Positions:   positions,
	}, nil
}

func (s *Server) fetchPendingOrders(ctx context.Context, cfg config.Config, requestedAPIID, instType string) (pendingOrdersResponse, error) {
	client, apiID, err := s.okxClientForCredentials(cfg, requestedAPIID)
	if err != nil {
		return pendingOrdersResponse{}, err
	}
	orders, _, err := client.PendingOrders(ctx, instType)
	if err != nil {
		return pendingOrdersResponse{}, err
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].InstID == orders[j].InstID {
			if orders[i].CTime == orders[j].CTime {
				return orders[i].OrdID < orders[j].OrdID
			}
			return orders[i].CTime > orders[j].CTime
		}
		return orders[i].InstID < orders[j].InstID
	})
	views := s.pendingOrderViews(ctx, client, apiID, orders)
	return pendingOrdersResponse{
		OK:          true,
		Exchange:    trading.ExchangeOKX,
		APIID:       apiID,
		InstType:    instType,
		Count:       len(orders),
		RefreshedAt: s.now(),
		Orders:      views,
	}, nil
}

func (s *Server) fetchBinancePositions(ctx context.Context, cfg config.Config, requestedAPIID string) (positionsResponse, error) {
	client, apiID, err := s.binanceClientForCredentials(cfg, requestedAPIID)
	if err != nil {
		return positionsResponse{}, err
	}
	positions, err := client.Positions(ctx, "")
	if err != nil {
		return positionsResponse{}, err
	}
	out := make([]okx.Position, 0, len(positions))
	for _, position := range positions {
		converted := binancePositionToOKX(position)
		if isOpenPosition(converted.Pos) {
			out = append(out, converted)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].InstID == out[j].InstID {
			return out[i].PosSide < out[j].PosSide
		}
		return out[i].InstID < out[j].InstID
	})
	return positionsResponse{
		OK:          true,
		Exchange:    trading.ExchangeBinance,
		APIID:       apiID,
		InstType:    "USDT-M",
		Count:       len(out),
		RefreshedAt: s.now(),
		Positions:   out,
	}, nil
}

func (s *Server) fetchBinancePendingOrders(ctx context.Context, cfg config.Config, requestedAPIID string) (pendingOrdersResponse, error) {
	client, apiID, err := s.binanceClientForCredentials(cfg, requestedAPIID)
	if err != nil {
		return pendingOrdersResponse{}, err
	}
	orders, err := client.OpenOrders(ctx, "")
	if err != nil {
		return pendingOrdersResponse{}, err
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].Symbol == orders[j].Symbol {
			return orders[i].Time > orders[j].Time
		}
		return orders[i].Symbol < orders[j].Symbol
	})
	views := s.binancePendingOrderViews(ctx, client, apiID, orders)
	return pendingOrdersResponse{
		OK:          true,
		Exchange:    trading.ExchangeBinance,
		APIID:       apiID,
		InstType:    "USDT-M",
		Count:       len(views),
		RefreshedAt: s.now(),
		Orders:      views,
	}, nil
}

func binancePositionToOKX(position binance.Position) okx.Position {
	posSide := strings.ToLower(strings.TrimSpace(position.PositionSide))
	if posSide == "" || posSide == "both" {
		posSide = "net"
	}
	marginMode := strings.ToLower(strings.TrimSpace(position.MarginType))
	if marginMode == "" && strings.TrimSpace(position.IsolatedMargin) != "" {
		marginMode = config.MarginIsolated
	}
	margin := strings.TrimSpace(position.IsolatedMargin)
	if margin == "" || margin == "0" {
		margin = strings.TrimSpace(position.InitialMargin)
	}
	return okx.Position{
		InstType:    "USDT-M",
		InstID:      strings.ToUpper(strings.TrimSpace(position.Symbol)),
		MgnMode:     marginMode,
		PosSide:     posSide,
		Pos:         strings.TrimSpace(position.PositionAmt),
		AvailPos:    strings.TrimSpace(position.PositionAmt),
		AvgPx:       strings.TrimSpace(position.EntryPrice),
		MarkPx:      strings.TrimSpace(position.MarkPrice),
		Upl:         strings.TrimSpace(position.UnRealizedProfit),
		UplRatio:    binanceUPLRatio(position),
		Lever:       strings.TrimSpace(position.Leverage),
		LiqPx:       strings.TrimSpace(position.LiquidationPrice),
		NotionalUsd: strings.TrimLeft(strings.TrimSpace(position.Notional), "-"),
		Margin:      margin,
		Adl:         strconv.Itoa(position.Adl),
		UTime:       strconv.FormatInt(position.UpdateTime, 10),
		Ccy:         strings.TrimSpace(position.MarginAsset),
	}
}

func binanceOpenOrderToOKX(order binance.OpenOrder) okx.PendingOrder {
	reduceOnly := json.RawMessage("false")
	if order.ReduceOnly {
		reduceOnly = json.RawMessage("true")
	}
	return okx.PendingOrder{
		InstType:   "USDT-M",
		InstID:     strings.ToUpper(strings.TrimSpace(order.Symbol)),
		OrdID:      strconv.FormatInt(order.OrderID, 10),
		ClOrdID:    strings.TrimSpace(order.ClientOrderID),
		Side:       strings.ToLower(strings.TrimSpace(order.Side)),
		PosSide:    strings.ToLower(strings.TrimSpace(order.PositionSide)),
		OrdType:    strings.ToLower(strings.TrimSpace(order.Type)),
		Px:         strings.TrimSpace(order.Price),
		Sz:         strings.TrimSpace(order.OrigQty),
		AccFillSz:  strings.TrimSpace(order.ExecutedQty),
		AvgPx:      strings.TrimSpace(order.AvgPrice),
		State:      strings.ToLower(strings.TrimSpace(order.Status)),
		ReduceOnly: reduceOnly,
		CTime:      strconv.FormatInt(order.Time, 10),
		UTime:      strconv.FormatInt(order.UpdateTime, 10),
	}
}

func binanceUPLRatio(position binance.Position) string {
	upl, err1 := strconv.ParseFloat(strings.TrimSpace(position.UnRealizedProfit), 64)
	notional, err2 := strconv.ParseFloat(strings.TrimLeft(strings.TrimSpace(position.Notional), "-"), 64)
	if err1 != nil || err2 != nil || notional <= 0 {
		return ""
	}
	return strconv.FormatFloat(upl/notional, 'f', 8, 64)
}

func (s *Server) pendingOrderViews(ctx context.Context, client okx.Client, apiID string, orders []okx.PendingOrder) []pendingOrderView {
	views := make([]pendingOrderView, 0, len(orders))
	tickers := map[string]okx.Ticker{}
	instruments := map[string]okx.Instrument{}
	for _, order := range orders {
		view := pendingOrderView{
			PendingOrder: order,
			Chasing: pendingOrderChaseJobs.activeKey(pendingOrderChaseKey(pendingOrderChaseRequest{
				APIID:   apiID,
				InstID:  order.InstID,
				OrdID:   order.OrdID,
				ClOrdID: order.ClOrdID,
			})),
		}
		midPx, chasePx, err := pendingOrderMidAndChasePrice(ctx, client, order, tickers, instruments)
		if err != nil {
			view.PriceError = err.Error()
		} else {
			view.MidPx = midPx
			view.ChasePx = chasePx
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) binancePendingOrderViews(ctx context.Context, client binance.Client, apiID string, orders []binance.OpenOrder) []pendingOrderView {
	views := make([]pendingOrderView, 0, len(orders))
	tickers := map[string]binance.BookTicker{}
	instruments := map[string]binance.SymbolInfo{}
	for _, rawOrder := range orders {
		order := binanceOpenOrderToOKX(rawOrder)
		view := pendingOrderView{
			PendingOrder: order,
			Chasing: pendingOrderChaseJobs.activeKey(pendingOrderChaseKey(pendingOrderChaseRequest{
				Exchange: trading.ExchangeBinance,
				APIID:    apiID,
				InstID:   order.InstID,
				OrdID:    order.OrdID,
				ClOrdID:  order.ClOrdID,
			})),
		}
		midPx, chasePx, err := binancePendingOrderMidAndChasePrice(ctx, client, order, tickers, instruments)
		if err != nil {
			view.PriceError = err.Error()
		} else {
			view.MidPx = midPx
			view.ChasePx = chasePx
		}
		views = append(views, view)
	}
	return views
}

func (s *Server) watchPendingOrderChase(ctx context.Context, cfg config.Config, client okx.Client, req pendingOrderChaseRequest) {
	key := pendingOrderChaseKey(req)
	defer pendingOrderChaseJobs.done(key)
	interval := pendingOrderChaseInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := pendingOrderChaseTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, closed, err := chasePendingOrderOnce(stepCtx, cfg, client, req, false)
			cancel()
			if err != nil {
				if s.Logger != nil {
					s.Logger.Warn("pending order chase failed", "api_id", req.APIID, "inst_id", req.InstID, "ord_id", req.OrdID, "cl_ord_id", req.ClOrdID, "error", err)
				}
				continue
			}
			if closed {
				return
			}
		case <-timeoutTimer.C:
			stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, _, err := fallbackPendingOrderMarket(stepCtx, cfg, client, req)
			cancel()
			if err != nil && s.Logger != nil {
				s.Logger.Warn("pending order chase market fallback failed", "api_id", req.APIID, "inst_id", req.InstID, "ord_id", req.OrdID, "cl_ord_id", req.ClOrdID, "error", err)
			}
			return
		}
	}
}

func preparePendingOrderChase(ctx context.Context, cfg config.Config, client okx.Client, req pendingOrderChaseRequest) (pendingOrderChaseRequest, pendingOrderChaseResponse, bool, error) {
	order, found, err := currentPendingOrder(ctx, client, req)
	if err != nil {
		return req, pendingOrderChaseResponse{}, false, err
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		return req, resp, true, nil
	}
	resp.OrdID = order.OrdID
	resp.ClOrdID = order.ClOrdID
	midPx, chasePx, err := pendingOrderMidAndChasePrice(ctx, client, order, nil, nil)
	if err != nil {
		return req, resp, false, err
	}
	resp.MidPx = midPx
	resp.Px = chasePx
	if shouldRebuildPendingOrderWithProtection(cfg, order) {
		remaining, err := pendingOrderRemainingSize(order)
		if err != nil {
			if errors.Is(err, errPendingOrderNoRemaining) {
				resp.Status = "finished"
				resp.Message = "pending order has no remaining size"
				return req, resp, true, nil
			}
			return req, resp, false, err
		}
		if err := cancelPendingOrder(ctx, client, order); err != nil {
			if _, stillOpen, checkErr := currentPendingOrder(ctx, client, req); checkErr == nil && !stillOpen {
				resp.Status = "finished"
				resp.Message = "pending order is no longer open"
				return req, resp, true, nil
			}
			return req, resp, false, err
		}
		limitReq, err := pendingOrderLimitRequest(cfg, order, remaining, chasePx)
		if err != nil {
			return req, resp, false, err
		}
		ack, _, err := client.PlaceOrder(ctx, limitReq)
		if err != nil {
			return req, resp, false, err
		}
		req.OrdID = strings.TrimSpace(ack.OrdID)
		req.ClOrdID = strings.TrimSpace(ack.ClOrdID)
		if req.ClOrdID == "" {
			req.ClOrdID = limitReq.ClOrdID
		}
		resp.Status = "rebuilt"
		resp.Message = "pending order rebuilt with risk controls"
		resp.OrdID = req.OrdID
		resp.ClOrdID = req.ClOrdID
		return req, resp, false, nil
	}
	result, closed, err := chasePendingOrderFromOrder(ctx, cfg, client, req, order, true)
	return req, result, closed, err
}

func chasePendingOrderOnce(ctx context.Context, cfg config.Config, client okx.Client, req pendingOrderChaseRequest, forceAttach bool) (pendingOrderChaseResponse, bool, error) {
	order, found, err := currentPendingOrder(ctx, client, req)
	if err != nil {
		return pendingOrderChaseResponse{}, false, err
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		return resp, true, nil
	}
	return chasePendingOrderFromOrder(ctx, cfg, client, req, order, forceAttach)
}

func chasePendingOrderFromOrder(ctx context.Context, cfg config.Config, client okx.Client, req pendingOrderChaseRequest, order okx.PendingOrder, forceAttach bool) (pendingOrderChaseResponse, bool, error) {
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	resp.OrdID = order.OrdID
	resp.ClOrdID = order.ClOrdID
	midPx, chasePx, err := pendingOrderMidAndChasePrice(ctx, client, order, nil, nil)
	if err != nil {
		return resp, false, err
	}
	resp.MidPx = midPx
	resp.Px = chasePx
	attachAlgoOrds := pendingOrderAmendAttachAlgoOrders(cfg, order, req)
	if strings.TrimSpace(order.Px) == chasePx && !(forceAttach && len(attachAlgoOrds) > 0) {
		resp.Status = "unchanged"
		resp.Message = "pending order price is already at chase price"
		return resp, false, nil
	}
	amendReq := okx.AmendOrderRequest{
		InstID:         order.InstID,
		NewPx:          chasePx,
		AttachAlgoOrds: attachAlgoOrds,
	}
	if strings.TrimSpace(order.OrdID) != "" {
		amendReq.OrdID = order.OrdID
	} else {
		amendReq.ClOrdID = order.ClOrdID
	}
	if _, _, err := client.AmendOrder(ctx, amendReq); err != nil {
		return resp, false, err
	}
	resp.Status = "amended"
	resp.Message = "pending order price amended"
	return resp, false, nil
}

func fallbackPendingOrderMarket(ctx context.Context, cfg config.Config, client okx.Client, req pendingOrderChaseRequest) (pendingOrderChaseResponse, bool, error) {
	order, found, err := currentPendingOrder(ctx, client, req)
	if err != nil {
		return pendingOrderChaseResponse{}, false, err
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		return resp, true, nil
	}
	resp.OrdID = order.OrdID
	resp.ClOrdID = order.ClOrdID
	remaining, err := pendingOrderRemainingSize(order)
	if err != nil {
		if errors.Is(err, errPendingOrderNoRemaining) {
			resp.Status = "finished"
			resp.Message = "pending order has no remaining size"
			return resp, true, nil
		}
		return resp, false, err
	}
	if err := cancelPendingOrder(ctx, client, order); err != nil {
		if _, stillOpen, checkErr := currentPendingOrder(ctx, client, req); checkErr == nil && !stillOpen {
			resp.Status = "finished"
			resp.Message = "pending order is no longer open"
			return resp, true, nil
		}
		return resp, false, err
	}
	marketReq, err := pendingOrderMarketRequest(cfg, order, remaining)
	if err != nil {
		return resp, false, err
	}
	ack, _, err := client.PlaceOrder(ctx, marketReq)
	if err != nil {
		return resp, false, err
	}
	resp.Status = "market_submitted"
	resp.Message = "pending order canceled and market order submitted"
	resp.Px = ""
	if strings.TrimSpace(ack.OrdID) != "" {
		resp.OrdID = ack.OrdID
	}
	if strings.TrimSpace(ack.ClOrdID) != "" {
		resp.ClOrdID = ack.ClOrdID
	}
	return resp, false, nil
}

func (s *Server) watchBinancePendingOrderChase(ctx context.Context, client binance.Client, req pendingOrderChaseRequest) {
	key := pendingOrderChaseKey(req)
	defer pendingOrderChaseJobs.done(key)
	interval := pendingOrderChaseInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := pendingOrderChaseTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, closed, err := chaseBinancePendingOrderOnce(stepCtx, client, req)
			cancel()
			if err != nil {
				if s.Logger != nil {
					s.Logger.Warn("Binance pending order chase failed", "api_id", req.APIID, "symbol", req.InstID, "ord_id", req.OrdID, "cl_ord_id", req.ClOrdID, "error", err)
				}
				continue
			}
			if closed {
				return
			}
		case <-timeoutTimer.C:
			stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, _, err := fallbackBinancePendingOrderMarket(stepCtx, client, req)
			cancel()
			if err != nil && s.Logger != nil {
				s.Logger.Warn("Binance pending order chase market fallback failed", "api_id", req.APIID, "symbol", req.InstID, "ord_id", req.OrdID, "cl_ord_id", req.ClOrdID, "error", err)
			}
			return
		}
	}
}

func prepareBinancePendingOrderChase(ctx context.Context, client binance.Client, req pendingOrderChaseRequest) (pendingOrderChaseResponse, bool, error) {
	return chaseBinancePendingOrderOnce(ctx, client, req)
}

func chaseBinancePendingOrderOnce(ctx context.Context, client binance.Client, req pendingOrderChaseRequest) (pendingOrderChaseResponse, bool, error) {
	order, found, err := currentBinancePendingOrder(ctx, client, req)
	if err != nil {
		return pendingOrderChaseResponse{}, false, err
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		return resp, true, nil
	}
	return chaseBinancePendingOrderFromOrder(ctx, client, req, order)
}

func chaseBinancePendingOrderFromOrder(ctx context.Context, client binance.Client, req pendingOrderChaseRequest, order okx.PendingOrder) (pendingOrderChaseResponse, bool, error) {
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   order.OrdID,
		ClOrdID: order.ClOrdID,
	}
	midPx, chasePx, err := binancePendingOrderMidAndChasePrice(ctx, client, order, nil, nil)
	if err != nil {
		return resp, false, err
	}
	resp.MidPx = midPx
	resp.Px = chasePx
	if strings.TrimSpace(order.Px) == chasePx {
		resp.Status = "unchanged"
		resp.Message = "pending order price is already at chase price"
		return resp, false, nil
	}
	ack, err := client.ModifyOrder(ctx, binance.ModifyOrderRequest{
		Symbol:            order.InstID,
		Side:              order.Side,
		Quantity:          strings.TrimSpace(order.Sz),
		Price:             chasePx,
		OrderID:           strings.TrimSpace(order.OrdID),
		OrigClientOrderID: strings.TrimSpace(order.ClOrdID),
	})
	if err != nil {
		return resp, false, err
	}
	resp.Status = "amended"
	resp.Message = "pending order price amended"
	if ack.OrderID != 0 {
		resp.OrdID = strconv.FormatInt(ack.OrderID, 10)
	}
	if strings.TrimSpace(ack.ClientOrderID) != "" {
		resp.ClOrdID = strings.TrimSpace(ack.ClientOrderID)
	}
	return resp, false, nil
}

func fallbackBinancePendingOrderMarket(ctx context.Context, client binance.Client, req pendingOrderChaseRequest) (pendingOrderChaseResponse, bool, error) {
	order, found, err := currentBinancePendingOrder(ctx, client, req)
	if err != nil {
		return pendingOrderChaseResponse{}, false, err
	}
	resp := pendingOrderChaseResponse{
		OK:      true,
		APIID:   req.APIID,
		InstID:  req.InstID,
		OrdID:   req.OrdID,
		ClOrdID: req.ClOrdID,
	}
	if !found {
		resp.Status = "finished"
		resp.Message = "pending order is no longer open"
		return resp, true, nil
	}
	resp.OrdID = order.OrdID
	resp.ClOrdID = order.ClOrdID
	remaining, err := pendingOrderRemainingSize(order)
	if err != nil {
		if errors.Is(err, errPendingOrderNoRemaining) {
			resp.Status = "finished"
			resp.Message = "pending order has no remaining size"
			return resp, true, nil
		}
		return resp, false, err
	}
	if err := cancelBinancePendingOrder(ctx, client, order); err != nil {
		if _, stillOpen, checkErr := currentBinancePendingOrder(ctx, client, req); checkErr == nil && !stillOpen {
			resp.Status = "finished"
			resp.Message = "pending order is no longer open"
			return resp, true, nil
		}
		return resp, false, err
	}
	marketReq := binance.PlaceOrderRequest{
		Symbol:           order.InstID,
		Side:             order.Side,
		PositionSide:     binancePendingPositionSide(order.PosSide),
		Type:             "MARKET",
		Quantity:         remaining,
		NewClientOrderID: nextPendingOrderMarketClOrdID(),
	}
	if okxRawBool(order.ReduceOnly) && (marketReq.PositionSide == "" || marketReq.PositionSide == "BOTH") {
		marketReq.ReduceOnly = true
	}
	ack, err := client.PlaceOrder(ctx, marketReq)
	if err != nil {
		return resp, false, err
	}
	resp.Status = "market_submitted"
	resp.Message = "pending order canceled and market order submitted"
	if ack.OrderID != 0 {
		resp.OrdID = strconv.FormatInt(ack.OrderID, 10)
	}
	if strings.TrimSpace(ack.ClientOrderID) != "" {
		resp.ClOrdID = strings.TrimSpace(ack.ClientOrderID)
	}
	return resp, false, nil
}

func currentPendingOrder(ctx context.Context, client okx.Client, req pendingOrderChaseRequest) (okx.PendingOrder, bool, error) {
	orders, _, err := client.PendingOrders(ctx, "SWAP")
	if err != nil {
		return okx.PendingOrder{}, false, err
	}
	for _, order := range orders {
		if !strings.EqualFold(order.InstID, req.InstID) {
			continue
		}
		if req.OrdID != "" && order.OrdID == req.OrdID {
			return order, true, nil
		}
		if req.OrdID == "" && req.ClOrdID != "" && order.ClOrdID == req.ClOrdID {
			return order, true, nil
		}
	}
	return okx.PendingOrder{}, false, nil
}

func currentBinancePendingOrder(ctx context.Context, client binance.Client, req pendingOrderChaseRequest) (okx.PendingOrder, bool, error) {
	orders, err := client.OpenOrders(ctx, req.InstID)
	if err != nil {
		return okx.PendingOrder{}, false, err
	}
	for _, rawOrder := range orders {
		order := binanceOpenOrderToOKX(rawOrder)
		if !strings.EqualFold(order.InstID, req.InstID) {
			continue
		}
		if req.OrdID != "" && order.OrdID == req.OrdID {
			return order, true, nil
		}
		if req.OrdID == "" && req.ClOrdID != "" && order.ClOrdID == req.ClOrdID {
			return order, true, nil
		}
	}
	return okx.PendingOrder{}, false, nil
}

func cancelPendingOrder(ctx context.Context, client okx.Client, order okx.PendingOrder) error {
	if strings.TrimSpace(order.OrdID) == "" && strings.TrimSpace(order.ClOrdID) == "" {
		return errors.New("pending order has no ord_id or cl_ord_id")
	}
	req := okx.CancelOrderRequest{InstID: strings.ToUpper(strings.TrimSpace(order.InstID))}
	if strings.TrimSpace(order.OrdID) != "" {
		req.OrdID = strings.TrimSpace(order.OrdID)
	} else {
		req.ClOrdID = strings.TrimSpace(order.ClOrdID)
	}
	_, _, err := client.CancelOrder(ctx, req)
	return err
}

func cancelBinancePendingOrder(ctx context.Context, client binance.Client, order okx.PendingOrder) error {
	if strings.TrimSpace(order.OrdID) == "" && strings.TrimSpace(order.ClOrdID) == "" {
		return errors.New("pending order has no ord_id or cl_ord_id")
	}
	_, err := client.CancelOrder(ctx, binance.CancelOrderRequest{
		Symbol:            order.InstID,
		OrderID:           strings.TrimSpace(order.OrdID),
		OrigClientOrderID: strings.TrimSpace(order.ClOrdID),
	})
	return err
}

func binancePendingPositionSide(posSide string) string {
	switch strings.ToLower(strings.TrimSpace(posSide)) {
	case "long":
		return "LONG"
	case "short":
		return "SHORT"
	case "both":
		return "BOTH"
	default:
		return ""
	}
}

func pendingOrderLimitRequest(cfg config.Config, order okx.PendingOrder, remaining, px string) (okx.PlaceOrderRequest, error) {
	req, err := pendingOrderOrderRequest(cfg, order, remaining, "limit", px, nextPendingOrderLimitClOrdID())
	if err != nil {
		return okx.PlaceOrderRequest{}, err
	}
	req.AttachAlgoOrds = pendingOrderAttachAlgoOrders(cfg, order, req.ClOrdID)
	return req, nil
}

func pendingOrderMarketRequest(cfg config.Config, order okx.PendingOrder, remaining string) (okx.PlaceOrderRequest, error) {
	req, err := pendingOrderOrderRequest(cfg, order, remaining, "market", "", nextPendingOrderMarketClOrdID())
	if err != nil {
		return okx.PlaceOrderRequest{}, err
	}
	req.AttachAlgoOrds = pendingOrderAttachAlgoOrders(cfg, order, req.ClOrdID)
	return req, nil
}

func pendingOrderOrderRequest(cfg config.Config, order okx.PendingOrder, remaining, ordType, px, clOrdID string) (okx.PlaceOrderRequest, error) {
	instID := strings.ToUpper(strings.TrimSpace(order.InstID))
	if instID == "" {
		return okx.PlaceOrderRequest{}, errors.New("inst_id is required")
	}
	side := strings.ToLower(strings.TrimSpace(order.Side))
	if side != "buy" && side != "sell" {
		return okx.PlaceOrderRequest{}, fmt.Errorf("unsupported order side %q", order.Side)
	}
	tdMode := strings.ToLower(strings.TrimSpace(order.TDMode))
	if tdMode == "" {
		tdMode = cfg.MarginMode()
	}
	req := okx.PlaceOrderRequest{
		InstID:  instID,
		TDMode:  tdMode,
		ClOrdID: clOrdID,
		Side:    side,
		OrdType: ordType,
		Px:      px,
		Sz:      remaining,
	}
	posSide := normalizePosSide(order.PosSide)
	if posSide != "" && posSide != "net" {
		req.PosSide = posSide
	}
	if okxRawBool(order.ReduceOnly) {
		req.ReduceOnly = true
	}
	return req, nil
}

func shouldRebuildPendingOrderWithProtection(cfg config.Config, order okx.PendingOrder) bool {
	return len(order.AttachAlgoOrds) == 0 && len(pendingOrderAttachAlgoOrders(cfg, order, "PENDINGRISK")) > 0
}

func pendingOrderAmendAttachAlgoOrders(cfg config.Config, order okx.PendingOrder, req pendingOrderChaseRequest) []map[string]string {
	if len(order.AttachAlgoOrds) == 0 {
		return nil
	}
	clOrdID := strings.TrimSpace(order.ClOrdID)
	if clOrdID == "" {
		clOrdID = strings.TrimSpace(req.ClOrdID)
	}
	if clOrdID == "" {
		clOrdID = nextPendingOrderLimitClOrdID()
	}
	return pendingOrderAttachAlgoOrders(cfg, order, clOrdID)
}

func pendingOrderAttachAlgoOrders(cfg config.Config, order okx.PendingOrder, clOrdID string) []map[string]string {
	if okxRawBool(order.ReduceOnly) {
		return nil
	}
	settings := cfg.OrderSettings()
	risk := settings.Risk
	risk.Normalize()
	if risk.Type == trading.RiskNone {
		return nil
	}
	action, ok := pendingOrderTradingAction(order.Side)
	if !ok {
		return nil
	}
	return okx.AttachAlgoOrders(action, risk, clOrdID)
}

func pendingOrderTradingAction(side string) (trading.Side, bool) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return trading.ActionLong, true
	case "sell":
		return trading.ActionShort, true
	default:
		return "", false
	}
}

func pendingOrderRemainingSize(order okx.PendingOrder) (string, error) {
	totalRaw := strings.TrimSpace(order.Sz)
	total, ok := new(big.Rat).SetString(totalRaw)
	if !ok || total.Sign() <= 0 {
		return "", fmt.Errorf("invalid pending order size %q", order.Sz)
	}
	filledRaw := strings.TrimSpace(order.AccFillSz)
	if filledRaw == "" {
		filledRaw = "0"
	}
	filled, ok := new(big.Rat).SetString(filledRaw)
	if !ok || filled.Sign() < 0 {
		return "", fmt.Errorf("invalid pending order filled size %q", order.AccFillSz)
	}
	remaining := new(big.Rat).Sub(total, filled)
	if remaining.Sign() <= 0 {
		return "", errPendingOrderNoRemaining
	}
	decimals := decimalsFromDecimalString(totalRaw)
	if filledDecimals := decimalsFromDecimalString(filledRaw); filledDecimals > decimals {
		decimals = filledDecimals
	}
	out := trimDecimalZeros(remaining.FloatString(decimals))
	if out == "" || out == "0" {
		return "", errPendingOrderNoRemaining
	}
	return out, nil
}

func okxRawBool(raw []byte) bool {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return false
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, _ := strconv.ParseBool(strings.TrimSpace(text))
		return parsed
	}
	return false
}

func pendingOrderMidAndChasePrice(ctx context.Context, client okx.Client, order okx.PendingOrder, tickers map[string]okx.Ticker, instruments map[string]okx.Instrument) (string, string, error) {
	instID := strings.ToUpper(strings.TrimSpace(order.InstID))
	if instID == "" {
		return "", "", errors.New("inst_id is required")
	}
	var inst okx.Instrument
	var ok bool
	if instruments != nil {
		inst, ok = instruments[instID]
	}
	if !ok {
		var err error
		inst, err = client.SwapInstrument(ctx, instID)
		if err != nil {
			return "", "", err
		}
		if instruments != nil {
			instruments[instID] = inst
		}
	}
	var ticker okx.Ticker
	if tickers != nil {
		ticker, ok = tickers[instID]
	} else {
		ok = false
	}
	if !ok {
		var err error
		ticker, _, err = client.MarketTicker(ctx, instID)
		if err != nil {
			return "", "", err
		}
		if tickers != nil {
			tickers[instID] = ticker
		}
	}
	mid, err := tickerMidPrice(ticker)
	if err != nil {
		return "", "", err
	}
	chasePx, err := passivePendingOrderPrice(mid, inst.TickSz, order.Side)
	if err != nil {
		return "", "", err
	}
	return formatMidPrice(mid, inst.TickSz), chasePx, nil
}

func binancePendingOrderMidAndChasePrice(ctx context.Context, client binance.Client, order okx.PendingOrder, tickers map[string]binance.BookTicker, instruments map[string]binance.SymbolInfo) (string, string, error) {
	symbol := strings.ToUpper(strings.TrimSpace(order.InstID))
	if symbol == "" {
		return "", "", errors.New("inst_id is required")
	}
	if strings.ToLower(strings.TrimSpace(order.OrdType)) != "limit" {
		return "", "", fmt.Errorf("Binance only supports chasing limit orders")
	}
	var inst binance.SymbolInfo
	var ok bool
	if instruments != nil {
		inst, ok = instruments[symbol]
	}
	if !ok {
		var err error
		inst, err = client.SymbolInfo(ctx, symbol)
		if err != nil {
			return "", "", err
		}
		if instruments != nil {
			instruments[symbol] = inst
		}
	}
	tickRaw, err := binanceTickSizeRaw(inst)
	if err != nil {
		return "", "", err
	}
	var ticker binance.BookTicker
	if tickers != nil {
		ticker, ok = tickers[symbol]
	} else {
		ok = false
	}
	if !ok {
		ticker, err = client.BookTicker(ctx, symbol)
		if err != nil {
			return "", "", err
		}
		if tickers != nil {
			tickers[symbol] = ticker
		}
	}
	mid, err := binanceBookMidPrice(ticker)
	if err != nil {
		return "", "", err
	}
	chasePx, err := passivePendingOrderPrice(mid, tickRaw, order.Side)
	if err != nil {
		return "", "", err
	}
	return formatMidPrice(mid, tickRaw), chasePx, nil
}

func tickerMidPrice(ticker okx.Ticker) (float64, error) {
	bid, bidErr := strconv.ParseFloat(strings.TrimSpace(ticker.BidPx), 64)
	ask, askErr := strconv.ParseFloat(strings.TrimSpace(ticker.AskPx), 64)
	if bidErr == nil && askErr == nil && bid > 0 && ask > 0 {
		return (bid + ask) / 2, nil
	}
	last, lastErr := strconv.ParseFloat(strings.TrimSpace(ticker.Last), 64)
	if lastErr != nil || last <= 0 {
		return 0, fmt.Errorf("invalid ticker bid/ask for %s", ticker.InstID)
	}
	return last, nil
}

func binanceBookMidPrice(ticker binance.BookTicker) (float64, error) {
	bid, bidErr := strconv.ParseFloat(strings.TrimSpace(ticker.BidPrice), 64)
	ask, askErr := strconv.ParseFloat(strings.TrimSpace(ticker.AskPrice), 64)
	if bidErr != nil || askErr != nil || bid <= 0 || ask <= 0 {
		return 0, fmt.Errorf("invalid Binance book ticker bid/ask for %s", ticker.Symbol)
	}
	return (bid + ask) / 2, nil
}

func binanceTickSizeRaw(info binance.SymbolInfo) (string, error) {
	for _, filter := range info.Filters {
		if filter.FilterType == "PRICE_FILTER" && strings.TrimSpace(filter.TickSize) != "" {
			return strings.TrimSpace(filter.TickSize), nil
		}
	}
	filters, err := info.TradingFilters()
	if err != nil {
		return "", err
	}
	return trading.NormalizeFloat(filters.TickSize), nil
}

func passivePendingOrderPrice(mid float64, tickRaw, side string) (string, error) {
	tick, err := strconv.ParseFloat(strings.TrimSpace(tickRaw), 64)
	if err != nil || tick <= 0 {
		return "", fmt.Errorf("invalid tick size %q", tickRaw)
	}
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return formatPriceToTick(mid-tick, tick, tickRaw, false)
	case "sell":
		return formatPriceToTick(mid+tick, tick, tickRaw, true)
	default:
		return "", fmt.Errorf("unsupported order side %q", side)
	}
}

func formatMidPrice(mid float64, tickRaw string) string {
	decimals := decimalsFromDecimalString(tickRaw)
	return trimDecimalZeros(strconv.FormatFloat(mid, 'f', decimals, 64))
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

func (s *Server) binanceClientForCredentials(cfg config.Config, requestedAPIID string) (binance.Client, string, error) {
	creds, apiID, err := s.BinanceCredentials.BinanceCredentials(requestedAPIID)
	if err != nil {
		return binance.Client{}, "", err
	}
	return binance.Client{
		BaseURL:     cfg.BinanceBaseURL(),
		Credentials: creds,
		HTTPClient:  s.binanceHTTPClient(),
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

func lowMarginPosition(position okx.Position, threshold float64) (float64, bool) {
	if threshold <= 0 || !isOpenPosition(position.Pos) {
		return 0, false
	}
	margin, err := strconv.ParseFloat(strings.TrimSpace(position.Margin), 64)
	if err != nil || margin < 0 {
		return 0, false
	}
	return margin, margin < threshold
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

func (r *pendingOrderChaseRequest) normalize() {
	r.Exchange = trading.NormalizeExchange(r.Exchange)
	r.APIID = strings.TrimSpace(r.APIID)
	r.InstID = strings.ToUpper(strings.TrimSpace(r.InstID))
	r.OrdID = strings.TrimSpace(r.OrdID)
	r.ClOrdID = strings.TrimSpace(r.ClOrdID)
}

func (r pendingOrderChaseRequest) validate() error {
	if r.InstID == "" {
		return errors.New("inst_id is required")
	}
	if r.OrdID == "" && r.ClOrdID == "" {
		return errors.New("ord_id or cl_ord_id is required")
	}
	return nil
}

func pendingOrderChaseKey(req pendingOrderChaseRequest) string {
	id := strings.TrimSpace(req.OrdID)
	if id == "" {
		id = "cl:" + strings.TrimSpace(req.ClOrdID)
	}
	return trading.NormalizeExchange(req.Exchange) + "|" + strings.TrimSpace(req.APIID) + "|" + strings.ToUpper(strings.TrimSpace(req.InstID)) + "|" + id
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

func nextPendingOrderLimitClOrdID() string {
	seq := atomic.AddUint64(&pendingOrderMarketSeq, 1) % 1000000
	return fmt.Sprintf("PCL%d%06d", time.Now().UTC().UnixMilli(), seq)
}

func nextPendingOrderMarketClOrdID() string {
	seq := atomic.AddUint64(&pendingOrderMarketSeq, 1) % 1000000
	return fmt.Sprintf("PCM%d%06d", time.Now().UTC().UnixMilli(), seq)
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

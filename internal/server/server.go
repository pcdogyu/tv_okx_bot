package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/security"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	"github.com/pcdogyu/tv_okx_bot/internal/upgrade"
)

type Server struct {
	ConfigStore *config.Store
	Orders      *storage.OrderStore
	Token       security.TokenService
	Executor    trading.Executor
	AdminToken  string
	AdminUser   string
	AdminPass   string
	Logger      *slog.Logger
	Now         func() time.Time
	Upgrade     *upgrade.Manager
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		http.Redirect(w, r, "https://www.mext.go.jp/", http.StatusFound)
	case r.URL.Path == "/tvorder":
		s.handleTVOrder(w, r)
	case r.URL.Path == "/tvbot" || strings.HasPrefix(r.URL.Path, "/tvbot/"):
		s.handleTVBot(w, r)
	case r.URL.Path == "/upgrade" || r.URL.Path == "/upgrade/":
		s.handleUpgrade(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "valid api roots are /tvorder, /tvbot and /upgrade")
	}
}

func (s *Server) handleTVOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "/tvorder only accepts POST")
		return
	}
	var signal trading.Signal
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&signal); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	signal.Normalize()
	signal.Risk = trading.Risk{Type: trading.RiskNone}
	cfg := s.ConfigStore.Get()
	now := s.now()
	if err := signal.Validate(now, time.Duration(cfg.Trading.SignalTTLSeconds)*time.Second, cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_signal", err.Error())
		return
	}
	if !s.Token.Validate(signal.CanonicalTokenPayload(), signal.Token) {
		writeError(w, http.StatusUnauthorized, "invalid_token", "token validation failed")
		return
	}
	dedupeKey := storage.DedupeKey(signal)
	record, duplicate, err := s.Orders.RecordAccepted(signal, dedupeKey, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if duplicate {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":    "duplicate",
			"signal_id": record.SignalID,
		})
		return
	}
	go s.execute(record.SignalID, signal, cfg)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":    "accepted",
		"signal_id": record.SignalID,
	})
}

func (s *Server) execute(signalID string, signal trading.Signal, cfg config.Config) {
	if s.Executor == nil {
		_ = s.Orders.MarkFailed(signalID, errors.New("executor is not configured"), s.now())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := s.Executor.ExecuteSignal(ctx, signal, cfg)
	result.SignalID = signalID
	if err != nil {
		_ = s.Orders.MarkFailed(signalID, err, s.now())
		if s.Logger != nil {
			s.Logger.Error("order failed", "signal_id", signalID, "action", signal.Action, "coinpair", signal.Coinpair, "error", err)
		}
		return
	}
	_ = s.Orders.MarkSubmitted(signalID, result, s.now())
}

func (s *Server) handleTVBot(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/tvbot")
	if path == "" || path == "/" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		writeHTML(w, http.StatusOK, tvbotHTML)
		return
	}
	switch path {
	case "/config":
		s.handleConfig(w, r)
	case "/symbols":
		s.handleSymbols(w, r)
	case "/templates":
		s.handleTemplates(w, r)
	case "/orders":
		s.handleOrders(w, r)
	case "/check-okx":
		s.handleCheckOKX(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown /tvbot endpoint")
	}
}

func (s *Server) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if s.Upgrade == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "upgrade runner is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.Upgrade.Status())
	case http.MethodPost:
		result, started, err := s.Upgrade.Start(context.Background())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upgrade_error", err.Error())
			return
		}
		if started {
			writeJSON(w, http.StatusAccepted, result)
			return
		}
		writeJSON(w, http.StatusConflict, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are allowed")
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.ConfigStore.Get())
	case http.MethodPut:
		var patch configPatch
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		cfg, err := s.ConfigStore.Update(func(c *config.Config) error {
			applyConfigPatch(c, patch)
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and PUT are allowed")
	}
}

func (s *Server) handleSymbols(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"symbols": s.ConfigStore.Get().Symbols})
	case http.MethodPut:
		symbols, err := decodeSymbols(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		cfg, err := s.ConfigStore.Update(func(c *config.Config) error {
			c.Symbols = symbols
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_symbols", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"symbols": cfg.Symbols})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and PUT are allowed")
	}
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	var req trading.TemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	resp, err := trading.BuildTemplate(req, s.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_template", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": s.Orders.List(limit)})
}

func (s *Server) handleCheckOKX(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	if s.Executor == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "executor is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := s.Executor.Check(ctx, s.ConfigStore.Get())
	if err != nil {
		writeError(w, http.StatusBadGateway, "okx_check_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.authorized(r) {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="tv-okx-bot"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", "admin credentials are required")
	return false
}

func (s *Server) authorized(r *http.Request) bool {
	got := r.Header.Get("X-Admin-Token")
	if s.AdminToken != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.AdminToken)) == 1 {
		return true
	}
	user, pass, ok := r.BasicAuth()
	if !ok || s.AdminUser == "" || s.AdminPass == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(user), []byte(s.AdminUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(s.AdminPass)) == 1
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type configPatch struct {
	Server   *serverPatch  `json:"server"`
	DataFile *string       `json:"data_file"`
	Trading  *tradingPatch `json:"trading"`
}

type serverPatch struct {
	Addr *string `json:"addr"`
}

type tradingPatch struct {
	Env               *string `json:"env"`
	AllowLiveTrading  *bool   `json:"allow_live_trading"`
	BaseURL           *string `json:"base_url"`
	DefaultMarginMode *string `json:"default_margin_mode"`
	PositionMode      *string `json:"position_mode"`
	SignalTTLSeconds  *int    `json:"signal_ttl_seconds"`
}

func applyConfigPatch(c *config.Config, patch configPatch) {
	if patch.Server != nil && patch.Server.Addr != nil {
		c.Server.Addr = *patch.Server.Addr
	}
	if patch.DataFile != nil {
		c.DataFile = *patch.DataFile
	}
	if patch.Trading == nil {
		return
	}
	if patch.Trading.Env != nil {
		c.Trading.Env = *patch.Trading.Env
	}
	if patch.Trading.AllowLiveTrading != nil {
		c.Trading.AllowLiveTrading = *patch.Trading.AllowLiveTrading
	}
	if patch.Trading.BaseURL != nil {
		c.Trading.BaseURL = *patch.Trading.BaseURL
	}
	if patch.Trading.DefaultMarginMode != nil {
		c.Trading.DefaultMarginMode = *patch.Trading.DefaultMarginMode
	}
	if patch.Trading.PositionMode != nil {
		c.Trading.PositionMode = *patch.Trading.PositionMode
	}
	if patch.Trading.SignalTTLSeconds != nil {
		c.Trading.SignalTTLSeconds = *patch.Trading.SignalTTLSeconds
	}
}

func decodeSymbols(r *http.Request) (map[string]config.SymbolConfig, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(body)
	var wrapped struct {
		Symbols map[string]config.SymbolConfig `json:"symbols"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Symbols != nil {
		return wrapped.Symbols, nil
	}
	var direct map[string]config.SymbolConfig
	if err := json.Unmarshal(raw, &direct); err != nil {
		return nil, fmt.Errorf("expected symbol map or {\"symbols\":...}: %w", err)
	}
	return direct, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":   code,
		"message": message,
	})
}

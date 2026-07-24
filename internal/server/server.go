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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
	"github.com/pcdogyu/tv_okx_bot/internal/security"
	"github.com/pcdogyu/tv_okx_bot/internal/storage"
	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	"github.com/pcdogyu/tv_okx_bot/internal/upgrade"
)

type Server struct {
	ConfigStore    *config.Store
	Orders         *storage.OrderStore
	Token          security.TokenService
	Executor       trading.Executor
	OKXCredentials *okx.CredentialStore
	OKXHTTPClient  *http.Client
	AdminToken     string
	AdminUser      string
	AdminPass      string
	Logger         *slog.Logger
	Now            func() time.Time
	Upgrade        *upgrade.Manager
	BuildInfo      BuildInfo
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
	now := s.now()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		s.recordTVOrderRejected(r, trading.Signal{}, "bad_json", err, now)
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	var signal trading.Signal
	if err := json.Unmarshal(body, &signal); err != nil {
		s.recordTVOrderRejected(r, signalPreviewFromJSON(body), "bad_json", err, now)
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	signal.Normalize()
	tokenSignal := signal
	cfg := s.ConfigStore.Get()
	cfg.OrderSettings().ApplyToSignal(&signal)
	if err := signal.Validate(now, time.Duration(cfg.Trading.SignalTTLSeconds)*time.Second, cfg); err != nil {
		s.recordTVOrderRejected(r, signal, "invalid_signal", err, now)
		writeError(w, http.StatusBadRequest, "invalid_signal", err.Error())
		return
	}
	if !s.validSignalToken(tokenSignal, signal) {
		s.recordTVOrderRejected(r, signal, "invalid_token", errors.New("token validation failed"), now)
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

func (s *Server) recordTVOrderRejected(r *http.Request, signal trading.Signal, code string, err error, now time.Time) {
	s.logTVOrderRejected(r, signal, code, err)
	if s.Orders == nil {
		return
	}
	if _, storeErr := s.Orders.RecordRejected(signal, code, err, now); storeErr != nil && s.Logger != nil {
		s.Logger.Error("failed to record rejected tvorder", "code", code, "error", storeErr)
	}
}

func (s *Server) validSignalToken(raw, applied trading.Signal) bool {
	payloads := []string{
		applied.CanonicalWebhookTokenPayload(),
		applied.CanonicalTokenPayload(),
		raw.CanonicalTokenPayload(),
	}
	seen := map[string]bool{}
	for _, payload := range payloads {
		if seen[payload] {
			continue
		}
		seen[payload] = true
		if s.Token.Validate(payload, applied.Token) {
			return true
		}
	}
	return false
}

func signalPreviewFromJSON(body []byte) trading.Signal {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return trading.Signal{}
	}
	var signal trading.Signal
	readString := func(name string) string {
		raw, ok := fields[name]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		var n json.Number
		if err := json.Unmarshal(raw, &n); err == nil {
			return n.String()
		}
		return ""
	}
	readInt := func(name string) int {
		raw, ok := fields[name]
		if !ok {
			return 0
		}
		var i int
		if err := json.Unmarshal(raw, &i); err == nil {
			return i
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if parsed, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				return parsed
			}
		}
		return 0
	}
	readFloat := func(name string) trading.FlexibleFloat {
		raw, ok := fields[name]
		if !ok {
			return trading.FlexibleFloat{}
		}
		var f trading.FlexibleFloat
		if err := json.Unmarshal(raw, &f); err == nil {
			return f
		}
		return trading.FlexibleFloat{}
	}
	signal.Action = trading.Side(readString("action"))
	signal.APIID = readString("api_id")
	signal.Coinpair = readString("coinpair")
	signal.Price = readFloat("price")
	signal.SentAt = readString("sent_at")
	signal.Time = readString("time")
	signal.Ticker = readString("ticker")
	signal.Exchange = readString("exchange")
	signal.Interval = readString("interval")
	signal.Condition = readString("condition")
	signal.Text = readString("text")
	signal.Leverage = readInt("leverage")
	signal.Amount = readFloat("amount")
	signal.Token = readString("token")
	signal.Normalize()
	return signal
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
		writeHTML(w, http.StatusOK, renderTVBotHTML(s.BuildInfo))
		return
	}
	switch {
	case path == "/config":
		s.handleConfig(w, r)
	case path == "/symbols":
		s.handleSymbols(w, r)
	case path == "/templates":
		s.handleTemplates(w, r)
	case path == "/orders":
		s.handleOrders(w, r)
	case isOrderRetryPath(path):
		s.handleOrderRetry(w, r, path)
	case path == "/analysis":
		s.handleAnalysis(w, r)
	case path == "/api-keys":
		s.handleAPIKeys(w, r)
	case path == "/api-keys/test":
		s.handleAPIKeyTest(w, r)
	case path == "/check-okx":
		s.handleCheckOKX(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown /tvbot endpoint")
	}
}

func (s *Server) logTVOrderRejected(r *http.Request, signal trading.Signal, code string, err error) {
	if s.Logger == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.Logger.Warn("tvorder rejected",
		"code", code,
		"message", msg,
		"client_ip", clientIP(r),
		"user_agent", r.UserAgent(),
		"action", signal.Action,
		"api_id", signal.APIID,
		"coinpair", signal.Coinpair,
		"ticker", signal.Ticker,
		"sent_at", signal.SentAt,
	)
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if i := strings.Index(forwarded, ","); i >= 0 {
			return strings.TrimSpace(forwarded[:i])
		}
		return forwarded
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	if s.OKXCredentials == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "OKX credential store is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.OKXCredentials.Status())
	case http.MethodPut:
		var req struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			APIKey     string `json:"api_key"`
			SecretKey  string `json:"secret_key"`
			Passphrase string `json:"passphrase"`
			Active     *bool  `json:"active"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		active := false
		if req.Active != nil {
			active = *req.Active
		}
		status, err := s.OKXCredentials.UpdateAccount(okx.CredentialAccountUpdate{
			ID:              req.ID,
			Name:            req.Name,
			Active:          active,
			PreserveMissing: true,
			Credentials: okx.Credentials{
				APIKey:     req.APIKey,
				SecretKey:  req.SecretKey,
				Passphrase: req.Passphrase,
			},
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_api_keys", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, status)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		status, err := s.OKXCredentials.DeleteAccount(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_api_keys", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, status)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET, PUT and DELETE are allowed")
	}
}

func (s *Server) handleAPIKeyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	var req struct {
		ID         string `json:"id"`
		APIKey     string `json:"api_key"`
		SecretKey  string `json:"secret_key"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	creds := okx.Credentials{
		APIKey:     strings.TrimSpace(req.APIKey),
		SecretKey:  strings.TrimSpace(req.SecretKey),
		Passphrase: strings.TrimSpace(req.Passphrase),
	}
	cfg := s.ConfigStore.Get()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if credentialFieldsProvided(creds) {
		if s.OKXCredentials != nil {
			if stored, resolvedID, err := s.OKXCredentials.OKXCredentials(req.ID); err == nil {
				req.ID = resolvedID
				mergeMissingCredentials(&creds, stored)
			}
		}
		result, err := checkOKXCredentials(ctx, cfg, req.ID, creds)
		if err != nil {
			writeError(w, http.StatusBadGateway, "okx_check_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := s.checkStoredOKX(ctx, cfg, req.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "okx_check_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
		cfg := s.ConfigStore.Get()
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		type instrumentResult struct {
			env string
			set okxInstrumentSet
		}
		results := make(chan instrumentResult, 2)
		go func() {
			results <- instrumentResult{env: config.EnvLive, set: s.fetchOKXInstrumentSet(ctx, cfg, false)}
		}()
		go func() {
			results <- instrumentResult{env: config.EnvDemo, set: s.fetchOKXInstrumentSet(ctx, cfg, true)}
		}()
		catalog := okxSymbolsCatalog{}
		for i := 0; i < 2; i++ {
			result := <-results
			if result.env == config.EnvDemo {
				catalog.Demo = result.set
				continue
			}
			catalog.Live = result.set
		}
		writeJSON(w, http.StatusOK, symbolsResponse{
			Symbols: cfg.Symbols,
			OKX:     catalog,
		})
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

type symbolsResponse struct {
	Symbols map[string]config.SymbolConfig `json:"symbols"`
	OKX     okxSymbolsCatalog              `json:"okx"`
}

type okxSymbolsCatalog struct {
	Live okxInstrumentSet `json:"live"`
	Demo okxInstrumentSet `json:"demo"`
}

type okxInstrumentSet struct {
	Env         string           `json:"env"`
	Demo        bool             `json:"demo"`
	Count       int              `json:"count"`
	Instruments []okx.Instrument `json:"instruments"`
	Error       string           `json:"error,omitempty"`
}

func (s *Server) fetchOKXInstrumentSet(ctx context.Context, cfg config.Config, demo bool) okxInstrumentSet {
	env := config.EnvLive
	if demo {
		env = config.EnvDemo
	}
	set := okxInstrumentSet{
		Env:         env,
		Demo:        demo,
		Instruments: []okx.Instrument{},
	}
	client := okx.Client{
		BaseURL:    cfg.OKXBaseURL(),
		Demo:       demo,
		HTTPClient: s.OKXHTTPClient,
	}
	instruments, _, err := client.SwapInstruments(ctx)
	if err != nil {
		set.Error = err.Error()
		return set
	}
	sort.Slice(instruments, func(i, j int) bool {
		return strings.Compare(instruments[i].InstID, instruments[j].InstID) < 0
	})
	set.Instruments = instruments
	set.Count = len(instruments)
	return set
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

func isOrderRetryPath(path string) bool {
	rest := strings.TrimPrefix(path, "/orders/")
	return rest != path && strings.HasSuffix(rest, "/retry") && strings.TrimSuffix(rest, "/retry") != ""
}

func orderRetrySignalID(path string) string {
	rest := strings.TrimPrefix(path, "/orders/")
	return strings.TrimSuffix(rest, "/retry")
}

func (s *Server) handleOrderRetry(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	if s.Orders == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "order store is not configured")
		return
	}
	sourceID := orderRetrySignalID(path)
	source, ok := s.Orders.Get(sourceID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "order record not found")
		return
	}
	if source.Status != storage.StatusFailed {
		writeError(w, http.StatusConflict, "not_retriable", "only failed orders can be retried")
		return
	}
	cfg := s.ConfigStore.Get()
	now := s.now()
	signal, err := retrySignalFromRecord(source, cfg, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_retry_signal", err.Error())
		return
	}
	record, duplicate, err := s.Orders.RecordAccepted(signal, storage.RetryKey(source.SignalID, signal, now), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if duplicate {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":    "duplicate",
			"signal_id": record.SignalID,
			"retry_of":  source.SignalID,
		})
		return
	}
	go s.execute(record.SignalID, signal, cfg)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":    "accepted",
		"signal_id": record.SignalID,
		"retry_of":  source.SignalID,
	})
}

func retrySignalFromRecord(rec storage.OrderRecord, cfg config.Config, now time.Time) (trading.Signal, error) {
	price, err := parseOrderRecordFloat("price", rec.Price)
	if err != nil {
		return trading.Signal{}, err
	}
	signal := trading.Signal{
		Action:   rec.Action,
		APIID:    rec.APIID,
		Coinpair: rec.Coinpair,
		Ticker:   rec.Ticker,
		Price:    trading.NewFlexibleFloat(price),
		SentAt:   now.UTC().Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(rec.Amount) != "" {
		amount, err := parseOrderRecordFloat("amount", rec.Amount)
		if err != nil {
			return trading.Signal{}, err
		}
		signal.Amount = trading.NewFlexibleFloat(amount)
	}
	signal.Normalize()
	cfg.OrderSettings().ApplyToSignal(&signal)
	if err := signal.Validate(now, 0, cfg); err != nil {
		return trading.Signal{}, err
	}
	return signal, nil
}

func parseOrderRecordFloat(name, value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		if err == nil {
			err = fmt.Errorf("must be positive")
		}
		return 0, fmt.Errorf("%s %q is invalid: %w", name, value, err)
	}
	return parsed, nil
}

func (s *Server) handleCheckOKX(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	var req struct {
		APIID string `json:"api_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
	}
	if strings.TrimSpace(req.APIID) != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		result, err := s.checkStoredOKX(ctx, s.ConfigStore.Get(), req.APIID)
		if err != nil {
			writeError(w, http.StatusBadGateway, "okx_check_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
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

func (s *Server) checkStoredOKX(ctx context.Context, cfg trading.RuntimeConfig, apiID string) (map[string]any, error) {
	if s.OKXCredentials == nil {
		return nil, errors.New("OKX credential store is not configured")
	}
	creds, resolvedID, err := s.OKXCredentials.OKXCredentials(apiID)
	if err != nil {
		return nil, err
	}
	return checkOKXCredentials(ctx, cfg, resolvedID, creds)
}

func checkOKXCredentials(ctx context.Context, cfg trading.RuntimeConfig, apiID string, creds okx.Credentials) (map[string]any, error) {
	trader := okx.Trader{
		Credentials: creds,
		HTTPClient:  &http.Client{Timeout: 15 * time.Second},
	}
	return trader.CheckCredentials(ctx, cfg, apiID, creds)
}

func credentialFieldsProvided(creds okx.Credentials) bool {
	return strings.TrimSpace(creds.APIKey) != "" ||
		strings.TrimSpace(creds.SecretKey) != "" ||
		strings.TrimSpace(creds.Passphrase) != ""
}

func mergeMissingCredentials(creds *okx.Credentials, stored okx.Credentials) {
	if strings.TrimSpace(creds.APIKey) == "" {
		creds.APIKey = stored.APIKey
	}
	if strings.TrimSpace(creds.SecretKey) == "" {
		creds.SecretKey = stored.SecretKey
	}
	if strings.TrimSpace(creds.Passphrase) == "" {
		creds.Passphrase = stored.Passphrase
	}
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
	Server       *serverPatch  `json:"server"`
	DataFile     *string       `json:"data_file"`
	DatabaseFile *string       `json:"database_file"`
	Trading      *tradingPatch `json:"trading"`
}

type serverPatch struct {
	Addr *string `json:"addr"`
}

type tradingPatch struct {
	Env                       *string  `json:"env"`
	AllowLiveTrading          *bool    `json:"allow_live_trading"`
	BaseURL                   *string  `json:"base_url"`
	DefaultMarginMode         *string  `json:"default_margin_mode"`
	PositionMode              *string  `json:"position_mode"`
	SignalTTLSeconds          *int     `json:"signal_ttl_seconds"`
	OrderAmountUSDT           *float64 `json:"order_amount_usdt"`
	Leverage                  *int     `json:"leverage"`
	OrderType                 *string  `json:"order_type"`
	RiskType                  *string  `json:"risk_type"`
	TakeProfitPct             *float64 `json:"take_profit_pct"`
	StopLossPct               *float64 `json:"stop_loss_pct"`
	TrailingPct               *float64 `json:"trailing_pct"`
	LongLimitPriceMultiplier  *float64 `json:"long_limit_price_multiplier"`
	ShortLimitPriceMultiplier *float64 `json:"short_limit_price_multiplier"`
}

func applyConfigPatch(c *config.Config, patch configPatch) {
	if patch.Server != nil && patch.Server.Addr != nil {
		c.Server.Addr = *patch.Server.Addr
	}
	if patch.DataFile != nil {
		c.DataFile = *patch.DataFile
	}
	if patch.DatabaseFile != nil {
		c.DatabaseFile = *patch.DatabaseFile
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
	if patch.Trading.OrderAmountUSDT != nil {
		c.Trading.OrderAmountUSDT = *patch.Trading.OrderAmountUSDT
	}
	if patch.Trading.Leverage != nil {
		c.Trading.Leverage = *patch.Trading.Leverage
	}
	if patch.Trading.OrderType != nil {
		c.Trading.OrderType = *patch.Trading.OrderType
	}
	if patch.Trading.RiskType != nil {
		c.Trading.RiskType = *patch.Trading.RiskType
	}
	if patch.Trading.TakeProfitPct != nil {
		c.Trading.TakeProfitPct = *patch.Trading.TakeProfitPct
	}
	if patch.Trading.StopLossPct != nil {
		c.Trading.StopLossPct = *patch.Trading.StopLossPct
	}
	if patch.Trading.TrailingPct != nil {
		c.Trading.TrailingPct = *patch.Trading.TrailingPct
	}
	if patch.Trading.LongLimitPriceMultiplier != nil {
		c.Trading.LongLimitPriceMultiplier = *patch.Trading.LongLimitPriceMultiplier
	}
	if patch.Trading.ShortLimitPriceMultiplier != nil {
		c.Trading.ShortLimitPriceMultiplier = *patch.Trading.ShortLimitPriceMultiplier
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
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":   code,
		"message": message,
	})
}

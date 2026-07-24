package server

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/config"
	"github.com/pcdogyu/tv_okx_bot/internal/okx"
)

type positionsResponse struct {
	OK          bool           `json:"ok"`
	APIID       string         `json:"api_id"`
	InstType    string         `json:"inst_type"`
	Count       int            `json:"count"`
	RefreshedAt time.Time      `json:"refreshed_at"`
	Positions   []okx.Position `json:"positions"`
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

func openPositions(positions []okx.Position) []okx.Position {
	out := make([]okx.Position, 0, len(positions))
	for _, position := range positions {
		if isOpenPosition(position.Pos) {
			out = append(out, position)
		}
	}
	return out
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

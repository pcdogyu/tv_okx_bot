package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/trading"
)

type OrderStatus string

const (
	StatusAccepted  OrderStatus = "accepted"
	StatusDuplicate OrderStatus = "duplicate"
	StatusSubmitted OrderStatus = "submitted"
	StatusFailed    OrderStatus = "failed"
)

type OrderRecord struct {
	SignalID   string              `json:"signal_id"`
	DedupeKey  string              `json:"dedupe_key"`
	Status     OrderStatus         `json:"status"`
	Action     trading.Side        `json:"action"`
	APIID      string              `json:"api_id,omitempty"`
	Coinpair   string              `json:"coinpair"`
	Ticker     string              `json:"ticker"`
	Price      string              `json:"price"`
	Leverage   int                 `json:"leverage"`
	Amount     string              `json:"amount"`
	TokenHash  string              `json:"token_hash"`
	AcceptedAt time.Time           `json:"accepted_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
	Result     trading.OrderResult `json:"result,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type OrderStore struct {
	mu    sync.Mutex
	path  string
	state orderState
}

type orderState struct {
	Dedupe map[string]string `json:"dedupe"`
	Orders []OrderRecord     `json:"orders"`
}

func NewOrderStore(path string) (*OrderStore, error) {
	s := &OrderStore{
		path: path,
		state: orderState{
			Dedupe: map[string]string{},
			Orders: []OrderRecord{},
		},
	}
	if path == "" {
		return s, nil
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *OrderStore) RecordAccepted(signal trading.Signal, dedupeKey string, now time.Time) (OrderRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.state.Dedupe[dedupeKey]; ok {
		if rec, found := s.findLocked(id); found {
			rec.Status = StatusDuplicate
			return rec, true, nil
		}
	}
	rec := OrderRecord{
		SignalID:   NewSignalID(now, dedupeKey),
		DedupeKey:  dedupeKey,
		Status:     StatusAccepted,
		Action:     signal.Action,
		APIID:      signal.APIID,
		Coinpair:   signal.Coinpair,
		Ticker:     signal.Ticker,
		Price:      trading.NormalizeFloat(signal.Price.Value),
		Leverage:   signal.Leverage,
		Amount:     trading.NormalizeFloat(signal.Amount.Value),
		TokenHash:  ShortHash(signal.Token),
		AcceptedAt: now.UTC(),
		UpdatedAt:  now.UTC(),
	}
	s.state.Dedupe[dedupeKey] = rec.SignalID
	s.state.Orders = append(s.state.Orders, rec)
	if err := s.saveLocked(); err != nil {
		return OrderRecord{}, false, err
	}
	return rec, false, nil
}

func (s *OrderStore) MarkSubmitted(signalID string, result trading.OrderResult, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Orders {
		if s.state.Orders[i].SignalID == signalID {
			s.state.Orders[i].Status = StatusSubmitted
			s.state.Orders[i].UpdatedAt = now.UTC()
			s.state.Orders[i].Result = result
			s.state.Orders[i].Error = ""
			return s.saveLocked()
		}
	}
	return errors.New("signal record not found")
}

func (s *OrderStore) MarkFailed(signalID string, err error, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Orders {
		if s.state.Orders[i].SignalID == signalID {
			s.state.Orders[i].Status = StatusFailed
			s.state.Orders[i].UpdatedAt = now.UTC()
			if err != nil {
				s.state.Orders[i].Error = err.Error()
			}
			return s.saveLocked()
		}
	}
	return errors.New("signal record not found")
}

func (s *OrderStore) List(limit int) []OrderRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.state.Orders) {
		limit = len(s.state.Orders)
	}
	out := make([]OrderRecord, 0, limit)
	for i := len(s.state.Orders) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.state.Orders[i])
	}
	return out
}

func (s *OrderStore) findLocked(signalID string) (OrderRecord, bool) {
	for _, rec := range s.state.Orders {
		if rec.SignalID == signalID {
			return rec, true
		}
	}
	return OrderRecord{}, false
}

func (s *OrderStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return err
	}
	if s.state.Dedupe == nil {
		s.state.Dedupe = map[string]string{}
	}
	if s.state.Orders == nil {
		s.state.Orders = []OrderRecord{}
	}
	return nil
}

func (s *OrderStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil && filepath.Dir(s.path) != "." {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func DedupeKey(signal trading.Signal) string {
	payload := string(signal.Action) + "|" +
		signal.APIID + "|" +
		signal.Coinpair + "|" +
		trading.NormalizeFloat(signal.Price.Value) + "|" +
		signal.SentAt + "|" +
		signal.Ticker + "|" +
		trading.NormalizeFloat(signal.Amount.Value) + "|" +
		signal.CanonicalTokenPayload() + "|" +
		signal.Token
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func NewSignalID(now time.Time, dedupeKey string) string {
	short := dedupeKey
	if len(short) > 12 {
		short = short[:12]
	}
	return now.UTC().Format("20060102T150405.000") + "-" + short
}

func ShortHash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:8])
}

package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pcdogyu/tv_okx_bot/internal/trading"
	_ "modernc.org/sqlite"
)

type OrderStatus string

const (
	StatusAccepted  OrderStatus = "accepted"
	StatusDuplicate OrderStatus = "duplicate"
	StatusSubmitted OrderStatus = "submitted"
	StatusFailed    OrderStatus = "failed"
	StatusRejected  OrderStatus = "rejected"
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
	ErrorCode  string              `json:"error_code,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type OrderStore struct {
	mu    sync.Mutex
	path  string
	state orderState
	db    *sql.DB
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

func NewSQLiteOrderStore(databasePath, legacyJSONPath string) (*OrderStore, error) {
	if databasePath == "" {
		return nil, errors.New("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil && filepath.Dir(databasePath) != "." {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &OrderStore{
		path: legacyJSONPath,
		state: orderState{
			Dedupe: map[string]string{},
			Orders: []OrderRecord{},
		},
		db: db,
	}
	if err := s.migrateSQLite(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.importLegacyJSON(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *OrderStore) RecordAccepted(signal trading.Signal, dedupeKey string, now time.Time) (OrderRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.recordAcceptedSQLiteLocked(signal, dedupeKey, now)
	}
	if id, ok := s.state.Dedupe[dedupeKey]; ok {
		if _, found := s.findLocked(id); found {
			rec := newOrderRecord(signal, dedupeKey, StatusDuplicate, now)
			rec.SignalID = s.newSignalIDLocked(now, dedupeKey+"|duplicate")
			rec.ErrorCode = "duplicate"
			rec.Error = "duplicate signal ignored"
			s.state.Orders = append(s.state.Orders, rec)
			if err := s.saveLocked(); err != nil {
				return OrderRecord{}, false, err
			}
			return rec, true, nil
		}
	}
	rec := newOrderRecord(signal, dedupeKey, StatusAccepted, now)
	rec.SignalID = s.newSignalIDLocked(now, dedupeKey)
	s.state.Dedupe[dedupeKey] = rec.SignalID
	s.state.Orders = append(s.state.Orders, rec)
	if err := s.saveLocked(); err != nil {
		return OrderRecord{}, false, err
	}
	return rec, false, nil
}

func (s *OrderStore) RecordRejected(signal trading.Signal, code string, err error, now time.Time) (OrderRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.recordRejectedSQLiteLocked(signal, code, err, now)
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	dedupeKey := RejectedKey(signal, code, message, now)
	rec := newOrderRecord(signal, dedupeKey, StatusRejected, now)
	rec.SignalID = s.newSignalIDLocked(now, dedupeKey)
	rec.ErrorCode = code
	rec.Error = message
	s.state.Orders = append(s.state.Orders, rec)
	if err := s.saveLocked(); err != nil {
		return OrderRecord{}, err
	}
	return rec, nil
}

func (s *OrderStore) MarkSubmitted(signalID string, result trading.OrderResult, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.markSubmittedSQLiteLocked(signalID, result, now)
	}
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
	if s.db != nil {
		return s.markFailedSQLiteLocked(signalID, err, now)
	}
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
	if s.db != nil {
		records, err := s.listSQLiteLocked(limit)
		if err == nil {
			return records
		}
		return nil
	}
	if limit <= 0 || limit > len(s.state.Orders) {
		limit = len(s.state.Orders)
	}
	out := make([]OrderRecord, 0, limit)
	for i := len(s.state.Orders) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.state.Orders[i])
	}
	return out
}

func (s *OrderStore) Get(signalID string) (OrderRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findLocked(signalID)
}

func (s *OrderStore) findLocked(signalID string) (OrderRecord, bool) {
	if s.db != nil {
		rec, err := s.findSQLiteLocked(signalID)
		return rec, err == nil
	}
	for _, rec := range s.state.Orders {
		if rec.SignalID == signalID {
			return rec, true
		}
	}
	return OrderRecord{}, false
}

func (s *OrderStore) newSignalIDLocked(now time.Time, seed string) string {
	id := NewSignalID(now, seed)
	if !s.hasSignalIDLocked(id) {
		return id
	}
	for i := 1; ; i++ {
		id = NewSignalID(now, collisionSeed(seed, i))
		if !s.hasSignalIDLocked(id) {
			return id
		}
	}
}

func (s *OrderStore) hasSignalIDLocked(signalID string) bool {
	if s.db != nil {
		var exists int
		err := s.db.QueryRow(`SELECT 1 FROM orders WHERE signal_id = ? LIMIT 1`, signalID).Scan(&exists)
		return err == nil
	}
	for _, rec := range s.state.Orders {
		if rec.SignalID == signalID {
			return true
		}
	}
	return false
}

func (s *OrderStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *OrderStore) migrateSQLite() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS orders (
			signal_id TEXT PRIMARY KEY,
			dedupe_key TEXT NOT NULL,
			status TEXT NOT NULL,
			action TEXT,
			api_id TEXT,
			coinpair TEXT,
			ticker TEXT,
			price TEXT,
			leverage INTEGER,
			amount TEXT,
			token_hash TEXT,
			accepted_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			result_json TEXT,
			error_code TEXT,
			error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_dedupe ON orders(dedupe_key)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_accepted_at ON orders(accepted_at)`,
		`CREATE TABLE IF NOT EXISTS okx_fills (
			api_id TEXT NOT NULL,
			inst_type TEXT NOT NULL,
			inst_id TEXT NOT NULL,
			trade_id TEXT NOT NULL,
			ord_id TEXT,
			cl_ord_id TEXT,
			side TEXT,
			fill_px TEXT,
			fill_sz TEXT,
			fill_pnl TEXT,
			fee TEXT,
			fee_ccy TEXT,
			fill_time INTEGER NOT NULL,
			raw_json TEXT,
			fetched_at TEXT NOT NULL,
			PRIMARY KEY(api_id, inst_id, trade_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_okx_fills_time ON okx_fills(api_id, fill_time)`,
		`CREATE TABLE IF NOT EXISTS market_candles (
			inst_id TEXT NOT NULL,
			bar TEXT NOT NULL,
			ts INTEGER NOT NULL,
			open TEXT,
			high TEXT,
			low TEXT,
			close TEXT,
			volume TEXT,
			confirm TEXT,
			fetched_at TEXT NOT NULL,
			PRIMARY KEY(inst_id, bar, ts)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_market_candles_ts ON market_candles(inst_id, bar, ts)`,
		`CREATE TABLE IF NOT EXISTS analysis_cache (
			cache_key TEXT PRIMARY KEY,
			payload_json TEXT NOT NULL,
			refreshed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usdt_balance_snapshots (
			api_id TEXT NOT NULL,
			env TEXT NOT NULL,
			bucket_ts INTEGER NOT NULL,
			observed_at TEXT NOT NULL,
			total_eq TEXT,
			eq TEXT,
			eq_usd TEXT,
			avail_eq TEXT,
			avail_bal TEXT,
			cash_bal TEXT,
			frozen_bal TEXT,
			dis_eq TEXT,
			balance_updated_at TEXT,
			PRIMARY KEY(api_id, env, bucket_ts)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usdt_balance_snapshots_time ON usdt_balance_snapshots(api_id, env, bucket_ts)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *OrderStore) importLegacyJSON() error {
	if s.path == "" {
		return nil
	}
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
	var state orderState
	if err := json.Unmarshal(b, &state); err != nil {
		return err
	}
	for _, rec := range state.Orders {
		if rec.SignalID == "" {
			rec.SignalID = NewSignalID(rec.AcceptedAt, rec.DedupeKey)
		}
		if rec.UpdatedAt.IsZero() {
			rec.UpdatedAt = rec.AcceptedAt
		}
		if err := s.insertOrderSQLiteLocked(rec); err != nil {
			return err
		}
	}
	return nil
}

func (s *OrderStore) recordAcceptedSQLiteLocked(signal trading.Signal, dedupeKey string, now time.Time) (OrderRecord, bool, error) {
	if s.hasPrimaryDedupeSQLiteLocked(dedupeKey) {
		rec := newOrderRecord(signal, dedupeKey, StatusDuplicate, now)
		rec.SignalID = s.newSignalIDLocked(now, dedupeKey+"|duplicate")
		rec.ErrorCode = "duplicate"
		rec.Error = "duplicate signal ignored"
		if err := s.insertOrderSQLiteLocked(rec); err != nil {
			return OrderRecord{}, false, err
		}
		return rec, true, nil
	}
	rec := newOrderRecord(signal, dedupeKey, StatusAccepted, now)
	rec.SignalID = s.newSignalIDLocked(now, dedupeKey)
	if err := s.insertOrderSQLiteLocked(rec); err != nil {
		return OrderRecord{}, false, err
	}
	return rec, false, nil
}

func (s *OrderStore) recordRejectedSQLiteLocked(signal trading.Signal, code string, err error, now time.Time) (OrderRecord, error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	dedupeKey := RejectedKey(signal, code, message, now)
	rec := newOrderRecord(signal, dedupeKey, StatusRejected, now)
	rec.SignalID = s.newSignalIDLocked(now, dedupeKey)
	rec.ErrorCode = code
	rec.Error = message
	if err := s.insertOrderSQLiteLocked(rec); err != nil {
		return OrderRecord{}, err
	}
	return rec, nil
}

func (s *OrderStore) markSubmittedSQLiteLocked(signalID string, result trading.OrderResult, now time.Time) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE orders SET status = ?, updated_at = ?, result_json = ?, error = '' WHERE signal_id = ?`,
		string(StatusSubmitted),
		now.UTC().Format(time.RFC3339Nano),
		string(resultJSON),
		signalID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("signal record not found")
	}
	return nil
}

func (s *OrderStore) markFailedSQLiteLocked(signalID string, failErr error, now time.Time) error {
	message := ""
	if failErr != nil {
		message = failErr.Error()
	}
	res, err := s.db.Exec(`UPDATE orders SET status = ?, updated_at = ?, error = ? WHERE signal_id = ?`,
		string(StatusFailed),
		now.UTC().Format(time.RFC3339Nano),
		message,
		signalID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("signal record not found")
	}
	return nil
}

func (s *OrderStore) insertOrderSQLiteLocked(rec OrderRecord) error {
	if rec.AcceptedAt.IsZero() {
		rec.AcceptedAt = time.Now().UTC()
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.AcceptedAt
	}
	resultJSON := ""
	if rec.Result != (trading.OrderResult{}) {
		b, err := json.Marshal(rec.Result)
		if err != nil {
			return err
		}
		resultJSON = string(b)
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO orders (
		signal_id, dedupe_key, status, action, api_id, coinpair, ticker, price,
		leverage, amount, token_hash, accepted_at, updated_at, result_json, error_code, error
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.SignalID,
		rec.DedupeKey,
		string(rec.Status),
		string(rec.Action),
		rec.APIID,
		rec.Coinpair,
		rec.Ticker,
		rec.Price,
		rec.Leverage,
		rec.Amount,
		rec.TokenHash,
		rec.AcceptedAt.UTC().Format(time.RFC3339Nano),
		rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		resultJSON,
		rec.ErrorCode,
		rec.Error,
	)
	return err
}

func (s *OrderStore) listSQLiteLocked(limit int) ([]OrderRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT signal_id, dedupe_key, status, action, api_id, coinpair, ticker, price,
		leverage, amount, token_hash, accepted_at, updated_at, result_json, error_code, error
		FROM orders ORDER BY accepted_at DESC, signal_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrderRecord
	for rows.Next() {
		rec, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *OrderStore) findSQLiteLocked(signalID string) (OrderRecord, error) {
	row := s.db.QueryRow(`SELECT signal_id, dedupe_key, status, action, api_id, coinpair, ticker, price,
		leverage, amount, token_hash, accepted_at, updated_at, result_json, error_code, error
		FROM orders WHERE signal_id = ?`, signalID)
	return scanOrder(row)
}

func (s *OrderStore) hasPrimaryDedupeSQLiteLocked(dedupeKey string) bool {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM orders WHERE dedupe_key = ? AND status IN (?, ?, ?) LIMIT 1`,
		dedupeKey,
		string(StatusAccepted),
		string(StatusSubmitted),
		string(StatusFailed),
	).Scan(&exists)
	return err == nil
}

type orderScanner interface {
	Scan(dest ...any) error
}

func scanOrder(scanner orderScanner) (OrderRecord, error) {
	var rec OrderRecord
	var status, action, acceptedAt, updatedAt, resultJSON string
	if err := scanner.Scan(
		&rec.SignalID,
		&rec.DedupeKey,
		&status,
		&action,
		&rec.APIID,
		&rec.Coinpair,
		&rec.Ticker,
		&rec.Price,
		&rec.Leverage,
		&rec.Amount,
		&rec.TokenHash,
		&acceptedAt,
		&updatedAt,
		&resultJSON,
		&rec.ErrorCode,
		&rec.Error,
	); err != nil {
		return OrderRecord{}, err
	}
	rec.Status = OrderStatus(status)
	rec.Action = trading.Side(action)
	if acceptedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, acceptedAt)
		if err != nil {
			return OrderRecord{}, fmt.Errorf("parse accepted_at: %w", err)
		}
		rec.AcceptedAt = parsed
	}
	if updatedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return OrderRecord{}, fmt.Errorf("parse updated_at: %w", err)
		}
		rec.UpdatedAt = parsed
	}
	if resultJSON != "" {
		if err := json.Unmarshal([]byte(resultJSON), &rec.Result); err != nil {
			return OrderRecord{}, err
		}
	}
	return rec, nil
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

func RejectedKey(signal trading.Signal, code, message string, now time.Time) string {
	payload := code + "|" +
		message + "|" +
		string(signal.Action) + "|" +
		signal.APIID + "|" +
		signal.Coinpair + "|" +
		trading.NormalizeFloat(signal.Price.Value) + "|" +
		signal.SentAt + "|" +
		signal.Ticker + "|" +
		trading.NormalizeFloat(signal.Amount.Value) + "|" +
		signal.CanonicalTokenPayload() + "|" +
		signal.Token + "|" +
		now.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func RetryKey(sourceSignalID string, signal trading.Signal, now time.Time) string {
	payload := "retry|" +
		sourceSignalID + "|" +
		string(signal.Action) + "|" +
		signal.APIID + "|" +
		signal.Coinpair + "|" +
		trading.NormalizeFloat(signal.Price.Value) + "|" +
		signal.SentAt + "|" +
		signal.Ticker + "|" +
		strconv.Itoa(signal.Leverage) + "|" +
		trading.NormalizeFloat(signal.Amount.Value) + "|" +
		now.UTC().Format(time.RFC3339Nano)
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

func collisionSeed(seed string, index int) string {
	sum := sha256.Sum256([]byte(seed + "|" + strconv.Itoa(index)))
	return hex.EncodeToString(sum[:])
}

func newOrderRecord(signal trading.Signal, dedupeKey string, status OrderStatus, now time.Time) OrderRecord {
	signal.Normalize()
	rec := OrderRecord{
		DedupeKey:  dedupeKey,
		Status:     status,
		Action:     signal.Action,
		APIID:      signal.APIID,
		Coinpair:   signal.Coinpair,
		Ticker:     signal.Ticker,
		Leverage:   signal.Leverage,
		AcceptedAt: now.UTC(),
		UpdatedAt:  now.UTC(),
	}
	if signal.Price.Set {
		rec.Price = trading.NormalizeFloat(signal.Price.Value)
	}
	if signal.Amount.Set {
		rec.Amount = trading.NormalizeFloat(signal.Amount.Value)
	}
	if signal.Token != "" {
		rec.TokenHash = ShortHash(signal.Token)
	}
	return rec
}

package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type MarketCandle struct {
	InstID  string `json:"inst_id"`
	Bar     string `json:"bar"`
	TS      int64  `json:"ts"`
	Open    string `json:"open"`
	High    string `json:"high"`
	Low     string `json:"low"`
	Close   string `json:"close"`
	Volume  string `json:"volume,omitempty"`
	Confirm string `json:"confirm,omitempty"`
}

type OKXFill struct {
	APIID    string `json:"api_id"`
	InstType string `json:"inst_type"`
	InstID   string `json:"inst_id"`
	TradeID  string `json:"trade_id"`
	OrdID    string `json:"ord_id,omitempty"`
	ClOrdID  string `json:"cl_ord_id,omitempty"`
	Side     string `json:"side,omitempty"`
	FillPx   string `json:"fill_px,omitempty"`
	FillSz   string `json:"fill_sz,omitempty"`
	FillPnl  string `json:"fill_pnl,omitempty"`
	Fee      string `json:"fee,omitempty"`
	FeeCcy   string `json:"fee_ccy,omitempty"`
	FillTime int64  `json:"fill_time"`
	RawJSON  string `json:"raw_json,omitempty"`
}

type CachedPayload struct {
	PayloadJSON string
	RefreshedAt time.Time
}

func (s *OrderStore) UpsertMarketCandles(candles []MarketCandle, fetchedAt time.Time) error {
	if s.db == nil {
		return errors.New("sqlite database is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO market_candles
		(inst_id, bar, ts, open, high, low, close, volume, confirm, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(inst_id, bar, ts) DO UPDATE SET
			open=excluded.open,
			high=excluded.high,
			low=excluded.low,
			close=excluded.close,
			volume=excluded.volume,
			confirm=excluded.confirm,
			fetched_at=excluded.fetched_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, candle := range candles {
		if _, err := stmt.Exec(
			candle.InstID,
			candle.Bar,
			candle.TS,
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
			candle.Volume,
			candle.Confirm,
			fetchedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *OrderStore) ListMarketCandles(instID, bar string, since time.Time, limit int) ([]MarketCandle, error) {
	if s.db == nil {
		return nil, errors.New("sqlite database is not configured")
	}
	if limit <= 0 {
		limit = 72
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT inst_id, bar, ts, open, high, low, close, volume, confirm
		FROM market_candles
		WHERE inst_id = ? AND bar = ? AND ts >= ?
		ORDER BY ts ASC LIMIT ?`,
		instID,
		bar,
		since.UTC().UnixMilli(),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MarketCandle
	for rows.Next() {
		var candle MarketCandle
		if err := rows.Scan(&candle.InstID, &candle.Bar, &candle.TS, &candle.Open, &candle.High, &candle.Low, &candle.Close, &candle.Volume, &candle.Confirm); err != nil {
			return nil, err
		}
		out = append(out, candle)
	}
	return out, rows.Err()
}

func (s *OrderStore) UpsertOKXFills(fills []OKXFill, fetchedAt time.Time) error {
	if s.db == nil {
		return errors.New("sqlite database is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO okx_fills
		(api_id, inst_type, inst_id, trade_id, ord_id, cl_ord_id, side, fill_px, fill_sz, fill_pnl, fee, fee_ccy, fill_time, raw_json, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(api_id, inst_id, trade_id) DO UPDATE SET
			inst_type=excluded.inst_type,
			ord_id=excluded.ord_id,
			cl_ord_id=excluded.cl_ord_id,
			side=excluded.side,
			fill_px=excluded.fill_px,
			fill_sz=excluded.fill_sz,
			fill_pnl=excluded.fill_pnl,
			fee=excluded.fee,
			fee_ccy=excluded.fee_ccy,
			fill_time=excluded.fill_time,
			raw_json=excluded.raw_json,
			fetched_at=excluded.fetched_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, fill := range fills {
		if fill.TradeID == "" {
			fill.TradeID = fallbackTradeID(fill, i)
		}
		if _, err := stmt.Exec(
			fill.APIID,
			fill.InstType,
			fill.InstID,
			fill.TradeID,
			fill.OrdID,
			fill.ClOrdID,
			fill.Side,
			fill.FillPx,
			fill.FillSz,
			fill.FillPnl,
			fill.Fee,
			fill.FeeCcy,
			fill.FillTime,
			fill.RawJSON,
			fetchedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *OrderStore) ListOKXFills(apiID string, since time.Time) ([]OKXFill, error) {
	if s.db == nil {
		return nil, errors.New("sqlite database is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT api_id, inst_type, inst_id, trade_id, ord_id, cl_ord_id, side, fill_px, fill_sz, fill_pnl, fee, fee_ccy, fill_time, raw_json
		FROM okx_fills
		WHERE api_id = ? AND fill_time >= ? AND inst_type = 'SWAP' AND inst_id LIKE '%-USDT-SWAP'
		ORDER BY fill_time DESC`,
		apiID,
		since.UTC().UnixMilli(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OKXFill
	for rows.Next() {
		var fill OKXFill
		if err := rows.Scan(&fill.APIID, &fill.InstType, &fill.InstID, &fill.TradeID, &fill.OrdID, &fill.ClOrdID, &fill.Side, &fill.FillPx, &fill.FillSz, &fill.FillPnl, &fill.Fee, &fill.FeeCcy, &fill.FillTime, &fill.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, fill)
	}
	return out, rows.Err()
}

func (s *OrderStore) CachePayload(key string, payload any, refreshedAt time.Time) error {
	if s.db == nil {
		return errors.New("sqlite database is not configured")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO analysis_cache(cache_key, payload_json, refreshed_at)
		VALUES (?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET payload_json=excluded.payload_json, refreshed_at=excluded.refreshed_at`,
		key,
		string(b),
		refreshedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *OrderStore) CachedPayload(key string) (CachedPayload, bool, error) {
	if s.db == nil {
		return CachedPayload{}, false, errors.New("sqlite database is not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload, refreshedAt string
	err := s.db.QueryRow(`SELECT payload_json, refreshed_at FROM analysis_cache WHERE cache_key = ?`, key).Scan(&payload, &refreshedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedPayload{}, false, nil
	}
	if err != nil {
		return CachedPayload{}, false, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, refreshedAt)
	if err != nil {
		return CachedPayload{}, false, err
	}
	return CachedPayload{PayloadJSON: payload, RefreshedAt: parsed}, true, nil
}

func fallbackTradeID(fill OKXFill, index int) string {
	return fill.OrdID + "|" + fill.ClOrdID + "|" + fill.Side + "|" + fill.FillPx + "|" + fill.FillSz + "|" + strconv.FormatInt(fill.FillTime, 10) + "|" + strconv.Itoa(index)
}

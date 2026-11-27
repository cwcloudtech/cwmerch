package woo

import (
	"context"
	"database/sql"
	"errors"
)

// SetOrderMeta upserts one order meta value in the authoritative storage
// (and the other one when the compatibility sync is on).
func (s *Store) SetOrderMeta(ctx context.Context, orderID uint64, key, value string) error {
	hpos, sync := s.HPOS(ctx)
	if hpos || sync {
		res, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("wc_orders_meta")+` SET meta_value = ? WHERE order_id = ? AND meta_key = ?`, value, orderID, key)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var exists int
			_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("wc_orders_meta")+` WHERE order_id = ? AND meta_key = ?`, orderID, key).Scan(&exists)
			if exists == 0 {
				if _, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("wc_orders_meta")+` (order_id, meta_key, meta_value) VALUES (?, ?, ?)`, orderID, key, value); err != nil {
					return err
				}
			}
		}
	}
	if !hpos || sync {
		return s.setPostMeta(ctx, s.DB, orderID, key, value)
	}
	return nil
}

// OrderMeta reads one order meta value ("" when missing).
func (s *Store) OrderMeta(ctx context.Context, orderID uint64, key string) string {
	hpos, _ := s.HPOS(ctx)
	var v sql.NullString
	if hpos {
		_ = s.DB.QueryRowContext(ctx, `SELECT meta_value FROM `+s.T("wc_orders_meta")+` WHERE order_id = ? AND meta_key = ? ORDER BY id DESC LIMIT 1`, orderID, key).Scan(&v)
	} else {
		_ = s.DB.QueryRowContext(ctx, `SELECT meta_value FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = ? ORDER BY meta_id DESC LIMIT 1`, orderID, key).Scan(&v)
	}
	return v.String
}

// FindOrderByMeta answers the order carrying meta key=value.
func (s *Store) FindOrderByMeta(ctx context.Context, key, value string) (uint64, error) {
	hpos, _ := s.HPOS(ctx)
	var id uint64
	var err error
	if hpos {
		err = s.DB.QueryRowContext(ctx, `SELECT order_id FROM `+s.T("wc_orders_meta")+` WHERE meta_key = ? AND meta_value = ? ORDER BY id DESC LIMIT 1`, key, value).Scan(&id)
	} else {
		err = s.DB.QueryRowContext(ctx, `SELECT post_id FROM `+s.T("postmeta")+` WHERE meta_key = ? AND meta_value = ? ORDER BY meta_id DESC LIMIT 1`, key, value).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

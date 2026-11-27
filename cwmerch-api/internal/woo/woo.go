// Package woo reads and writes WooCommerce's data directly in the shared
// WordPress database, the way WooCommerce itself stores it, so a WordPress
// install running WooCommerce on the same database sees every change
// cwmerch makes (and the other way round).
//
// Storage it follows:
//   - products / variations / coupons / attachments: <prefix>posts + postmeta,
//     categories/tags/attributes/types/visibility as terms, with the
//     wc_product_meta_lookup table kept in sync;
//   - orders: HPOS tables (wc_orders, wc_order_addresses, wc_order_operational_data,
//     wc_orders_meta) when `woocommerce_custom_orders_table_enabled` is "yes",
//     else shop_order posts; both when the compatibility sync is on;
//     lines in woocommerce_order_items(+meta), notes in comments;
//   - analytics: wc_order_stats, wc_order_product_lookup, wc_customer_lookup,
//     wc_order_coupon_lookup, wc_order_tax_lookup;
//   - settings: options (PHP-serialized where WooCommerce serializes them).
package woo

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/php"
)

// ErrNotFound is a missing product, order, term...
var ErrNotFound = errors.New("not found")

// Error is a business refusal with an i18n code the frontend translates.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func refuse(status int, code, message string) error {
	return &Error{Status: status, Code: code, Message: message}
}

// Store is the WooCommerce data access, shared by every handler.
type Store struct {
	DB *db.DB

	mu       sync.Mutex
	opts     map[string]optionEntry
	hposAt   time.Time
	hpos     bool
	hposSync bool
	// UploadsDir/UploadsURL locate wp-content/uploads (shared volume) for media.
	UploadsDir string
	UploadsURL string
	// SiteURL is the public shop URL (permalinks, emails, SEO).
	SiteURL string

	events OrderEvents
	fees   FeeProvider
}

type optionEntry struct {
	value string
	found bool
	at    time.Time
}

// optionTTL bounds how stale a cached option may be: WordPress may change it
// behind our back.
const optionTTL = 30 * time.Second

func New(d *db.DB, uploadsDir, uploadsURL, siteURL string) *Store {
	return &Store{DB: d, opts: map[string]optionEntry{}, UploadsDir: uploadsDir,
		UploadsURL: strings.TrimRight(uploadsURL, "/"), SiteURL: strings.TrimRight(siteURL, "/")}
}

func (s *Store) T(name string) string { return s.DB.T(name) }

// ------------------------------------------------------------------ options

// Option is an option value ("" when missing).
func (s *Store) Option(ctx context.Context, name string) string {
	v, _ := s.OptionFound(ctx, name)
	return v
}

// OptionOr is the option, or def when it does not exist.
func (s *Store) OptionOr(ctx context.Context, name, def string) string {
	if v, ok := s.OptionFound(ctx, name); ok {
		return v
	}
	return def
}

// OptionFound is the option and whether it exists.
func (s *Store) OptionFound(ctx context.Context, name string) (string, bool) {
	s.mu.Lock()
	if e, ok := s.opts[name]; ok && time.Since(e.at) < optionTTL {
		s.mu.Unlock()
		return e.value, e.found
	}
	s.mu.Unlock()
	var v sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT option_value FROM `+s.T("options")+` WHERE option_name = ? LIMIT 1`, name).Scan(&v)
	found := err == nil
	s.mu.Lock()
	s.opts[name] = optionEntry{value: v.String, found: found, at: time.Now()}
	s.mu.Unlock()
	return v.String, found
}

// OptionArray is a serialized option decoded (empty array when missing).
func (s *Store) OptionArray(ctx context.Context, name string) *php.Array {
	return php.UnmarshalArray(s.Option(ctx, name))
}

// OptionBool reads a WooCommerce "yes"/"no" option.
func (s *Store) OptionBool(ctx context.Context, name string, def bool) bool {
	v, ok := s.OptionFound(ctx, name)
	if !ok || v == "" {
		return def
	}
	return v == "yes" || v == "1" || v == "true"
}

// OptionInt reads a numeric option.
func (s *Store) OptionInt(ctx context.Context, name string, def int) int {
	v, ok := s.OptionFound(ctx, name)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// SetOption writes an option (insert or update), like update_option.
func (s *Store) SetOption(ctx context.Context, name, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("options")+` (option_name, option_value, autoload)
		VALUES (?, ?, 'on') ON DUPLICATE KEY UPDATE option_value = VALUES(option_value)`, name, value)
	s.mu.Lock()
	delete(s.opts, name)
	s.mu.Unlock()
	return err
}

// DeleteOption removes an option.
func (s *Store) DeleteOption(ctx context.Context, name string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("options")+` WHERE option_name = ?`, name)
	s.mu.Lock()
	delete(s.opts, name)
	s.mu.Unlock()
	return err
}

// ForgetOptions drops the option cache.
func (s *Store) ForgetOptions() {
	s.mu.Lock()
	s.opts = map[string]optionEntry{}
	s.hposAt = time.Time{}
	s.mu.Unlock()
}

// HPOS reports whether orders are authoritative in the HPOS tables, and
// whether the compatibility sync to posts is on.
func (s *Store) HPOS(ctx context.Context) (authoritative bool, sync bool) {
	s.mu.Lock()
	if time.Since(s.hposAt) < optionTTL {
		defer s.mu.Unlock()
		return s.hpos, s.hposSync
	}
	s.mu.Unlock()
	h := s.OptionBool(ctx, "woocommerce_custom_orders_table_enabled", false)
	sy := s.OptionBool(ctx, "woocommerce_custom_orders_table_data_sync_enabled", false)
	s.mu.Lock()
	s.hpos, s.hposSync, s.hposAt = h, sy, time.Now()
	s.mu.Unlock()
	return h, sy
}

// ------------------------------------------------------------------ meta

// Meta is a post's meta, first value per key (WooCommerce keeps one).
type Meta map[string]string

func (m Meta) Get(k string) string { return m[k] }

func (m Meta) Bool(k string) bool { v := m[k]; return v == "yes" || v == "1" }

func (m Meta) Float(k string) float64 { return parseFloat(m[k]) }

func (m Meta) FloatPtr(k string) *float64 {
	v := strings.TrimSpace(m[k])
	if v == "" {
		return nil
	}
	f := parseFloat(v)
	return &f
}

func (m Meta) Int(k string) int64 { return int64(parseFloat(m[k])) }

func (m Meta) IntPtr(k string) *int64 {
	v := strings.TrimSpace(m[k])
	if v == "" {
		return nil
	}
	n := int64(parseFloat(v))
	return &n
}

func (m Meta) Array(k string) *php.Array { return php.UnmarshalArray(m[k]) }

func parseFloat(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(v, ",", ".")), 64)
	if err != nil {
		return 0
	}
	return f
}

// postMetas loads the meta of several posts at once.
func (s *Store) postMetas(ctx context.Context, q db.Querier, ids []uint64) (map[uint64]Meta, error) {
	out := map[uint64]Meta{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		out[id] = Meta{}
	}
	for _, chunk := range chunks(ids, 500) {
		rows, err := q.QueryContext(ctx, `SELECT post_id, meta_key, meta_value FROM `+s.T("postmeta")+
			` WHERE post_id IN (`+inList(chunk)+`) ORDER BY meta_id`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id uint64
			var k string
			var v sql.NullString
			if err := rows.Scan(&id, &k, &v); err != nil {
				rows.Close()
				return nil, err
			}
			if _, seen := out[id][k]; !seen {
				out[id][k] = v.String
			}
		}
		rows.Close()
	}
	return out, nil
}

func (s *Store) postMeta(ctx context.Context, q db.Querier, id uint64) (Meta, error) {
	m, err := s.postMetas(ctx, q, []uint64{id})
	if err != nil {
		return nil, err
	}
	return m[id], nil
}

// setPostMeta is update_post_meta: one row per key.
func (s *Store) setPostMeta(ctx context.Context, q db.Querier, postID uint64, key, value string) error {
	var metaID uint64
	err := q.QueryRowContext(ctx, `SELECT meta_id FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = ? ORDER BY meta_id LIMIT 1`,
		postID, key).Scan(&metaID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.T("postmeta")+` (post_id, meta_key, meta_value) VALUES (?, ?, ?)`, postID, key, value)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+s.T("postmeta")+` SET meta_value = ? WHERE meta_id = ?`, value, metaID)
	return err
}

func (s *Store) setPostMetas(ctx context.Context, q db.Querier, postID uint64, values map[string]string) error {
	for k, v := range values {
		if err := s.setPostMeta(ctx, q, postID, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) deletePostMeta(ctx context.Context, q db.Querier, postID uint64, key string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = ?`, postID, key)
	return err
}

// ------------------------------------------------------------------ helpers

func chunks(ids []uint64, size int) [][]uint64 {
	var out [][]uint64
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

// inList renders numeric ids inline (safe: they are integers).
func inList(ids []uint64) string {
	if len(ids) == 0 {
		return "0"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}
	return strings.Join(parts, ",")
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func strArgs(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func uniqueIDs(ids []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := []uint64{}
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ParseID reads a numeric id.
func ParseID(s string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n
}

func idsFromSerialized(v string) []uint64 {
	out := []uint64{}
	for _, x := range php.UnmarshalArray(v).List() {
		if n := ParseID(php.ToString(x)); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

func serializeIDs(ids []uint64) string {
	a := php.NewArray()
	for _, id := range ids {
		a.Append(int64(id))
	}
	return php.Marshal(a)
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func fmtPrice(f *float64) string {
	if f == nil {
		return ""
	}
	return fmtFloat(*f)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// now is UTC wall time truncated to the second, the precision of WP DATETIMEs.
func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

// localTime is t in the shop's timezone (for the post_date / non-GMT columns).
func (s *Store) localTime(ctx context.Context, t time.Time) time.Time {
	if tz := s.Option(ctx, "timezone_string"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Date(t.In(loc).Year(), t.In(loc).Month(), t.In(loc).Day(), t.In(loc).Hour(),
				t.In(loc).Minute(), t.In(loc).Second(), 0, time.UTC)
		}
	}
	if off := s.Option(ctx, "gmt_offset"); off != "" {
		return t.Add(time.Duration(parseFloat(off) * float64(time.Hour)))
	}
	return t
}

// Paged is a page of a list.
type Paged[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PerPage    int   `json:"perPage"`
	TotalPages int   `json:"totalPages"`
}

func NewPaged[T any](items []T, total int64, page, perPage int) Paged[T] {
	if items == nil {
		items = []T{}
	}
	pages := 0
	if perPage > 0 {
		pages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return Paged[T]{Items: items, Total: total, Page: page, PerPage: perPage, TotalPages: pages}
}

// Page normalizes page/perPage.
func Page(page, perPage int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage, (page - 1) * perPage
}

func round(v float64, decimals int) float64 {
	p := 1.0
	for i := 0; i < decimals; i++ {
		p *= 10
	}
	if v < 0 {
		return -float64(int64(-v*p+0.5)) / p
	}
	return float64(int64(v*p+0.5)) / p
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmtFloat(x)
	case bool:
		if x {
			return "yes"
		}
		return "no"
	}
	return php.ToString(v)
}

func serializeStrings(values []string) string { return php.Marshal(php.FromStrings(values)) }

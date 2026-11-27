// Package dbclone copies cwmerch's data between databases, from one engine to
// the other or between two of the same type (MySQL/MariaDB ↔ PostgreSQL):
// "export" copies the main database to a target one, "import" copies a remote
// database into the main one.
//
// Both databases are expected to be migrated (same Flyway version). The
// destination may already hold data: rows are upserted by primary key (the
// source wins), and destination rows clashing with a source row on another
// unique key (an option name, an invoice number...) are replaced. Tables are
// copied parents first (foreign keys), in batches, keyset-paginated on the
// primary key. Zero dates, table prefixes (<prefix>user_roles,
// <prefix>capabilities...) and PostgreSQL identity sequences are taken care of.
package dbclone

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cwmerch-api/internal/db"

	"github.com/google/uuid"
)

// Directions.
const (
	Export = "export" // main → target
	Import = "import" // remote → main
)

// Conflict modes: what happens to destination rows with the same key.
const (
	Overwrite = "overwrite" // the source wins (default)
	Skip      = "skip"      // the destination's rows are kept, only missing rows are added
)

// children are WordPress/WooCommerce child tables: with Overwrite, the rows
// of an object the source has (a post's meta, an order's items...) are
// replaced by the source's, never mixed with the destination's.
var children = map[string][2]string{ // child table → parent table, child column
	"postmeta":                       {"posts", "post_id"},
	"term_relationships":             {"posts", "object_id"},
	"usermeta":                       {"users", "user_id"},
	"commentmeta":                    {"comments", "comment_id"},
	"termmeta":                       {"terms", "term_id"},
	"wc_orders_meta":                 {"wc_orders", "order_id"},
	"wc_order_addresses":             {"wc_orders", "order_id"},
	"woocommerce_order_items":        {"wc_orders", "order_id"},
	"woocommerce_order_itemmeta":     {"woocommerce_order_items", "order_item_id"},
	"woocommerce_tax_rate_locations": {"woocommerce_tax_rates", "tax_rate_id"},
}

// Remote is the other database.
type Remote struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Prefix   string `json:"prefix"`
	SSLMode  string `json:"sslMode,omitempty"`
}

func (r Remote) config() (db.Config, error) {
	t, err := db.NormalizeType(r.Type)
	if err != nil {
		return db.Config{}, err
	}
	if strings.TrimSpace(r.Host) == "" || strings.TrimSpace(r.Name) == "" {
		return db.Config{}, errors.New("host and database name are required")
	}
	port := strings.TrimSpace(r.Port)
	if port == "" {
		port = db.DefaultPort(t)
	}
	prefix := r.Prefix
	if prefix == "" {
		prefix = "wp_"
	}
	return db.Config{Type: t, Host: strings.TrimSpace(r.Host), Port: port, User: r.User, Password: r.Password,
		Name: strings.TrimSpace(r.Name), Prefix: prefix, SSLMode: r.SSLMode}, nil
}

// Info describes a database.
type Info struct {
	Type      string `json:"type"`
	Version   string `json:"version"`
	Host      string `json:"host,omitempty"`
	Name      string `json:"name,omitempty"`
	Prefix    string `json:"prefix"`
	Migration string `json:"migration"` // latest successful Flyway version ("" = no history)
	Tables    int    `json:"tables"`
	Rows      int64  `json:"rows"`
	Products  int64  `json:"products"`
	Orders    int64  `json:"orders"`
	Customers int64  `json:"customers"`
}

// TestResult is POST /v1/admin/clone/test's answer.
type TestResult struct {
	Main   Info     `json:"main"`
	Remote Info     `json:"remote"`
	Ready  bool     `json:"ready"`
	Issues []string `json:"issues,omitempty"`
	// Missing lists the source tables the destination lacks (not copied).
	Missing []string `json:"missing,omitempty"`
}

// TableProgress is one table's copy.
type TableProgress struct {
	Table        string `json:"table"`
	SourceRows   int64  `json:"sourceRows"`
	Copied       int64  `json:"copied"`
	Replaced     int64  `json:"replaced"` // destination rows removed on a unique-key clash
	BeforeRows   int64  `json:"beforeRows"`
	AfterRows    int64  `json:"afterRows"`
	Status       string `json:"status"` // pending | running | done | failed
	Error        string `json:"error,omitempty"`
	DurationMsec int64  `json:"durationMs"`
}

// Job is a clone run.
type Job struct {
	ID         string          `json:"id"`
	Direction  string          `json:"direction"`
	Conflict   string          `json:"conflict"`
	Status     string          `json:"status"` // running | done | failed
	Source     Info            `json:"source"`
	Target     Info            `json:"target"`
	Tables     []TableProgress `json:"tables"`
	Copied     int64           `json:"copied"`
	Total      int64           `json:"total"`
	Error      string          `json:"error,omitempty"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	StartedBy  uint64          `json:"startedBy"`
}

// Service runs one clone at a time.
type Service struct {
	main *db.DB
	// OnImported runs after an import into the main database (drop caches).
	OnImported func()

	mu  sync.Mutex
	job *Job
}

// New wires the service on the main database.
func New(main *db.DB) *Service { return &Service{main: main} }

var (
	ErrRunning  = errors.New("a clone is already running")
	ErrSameDB   = errors.New("the remote database is the main database")
	ErrNotReady = errors.New("the databases are not ready to be cloned")
)

// side is an open database with its dialect.
type side struct {
	conn   *sql.DB
	typ    string
	prefix string
	info   Info
}

func (s *side) pg() bool { return s.typ == db.Postgres }

func (s *side) quote(id string) string {
	if s.pg() {
		return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(id, "`", "``") + "`"
}

func (s *side) ph(n int) string {
	if s.pg() {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

func (s *Service) open(ctx context.Context, r Remote) (*side, *side, error) {
	rc, err := r.config()
	if err != nil {
		return nil, nil, err
	}
	mc := s.main.Config
	if rc.Type == s.main.Type && strings.EqualFold(rc.Host, mc.Host) && rc.Port == mc.Port && rc.Name == mc.Name && rc.Prefix == mc.Prefix {
		return nil, nil, ErrSameDB
	}
	mainConn, err := db.OpenRaw(ctx, mc)
	if err != nil {
		return nil, nil, fmt.Errorf("main database: %w", err)
	}
	remoteConn, err := db.OpenRaw(ctx, rc)
	if err != nil {
		_ = mainConn.Close()
		return nil, nil, fmt.Errorf("remote database: %w", err)
	}
	m := &side{conn: mainConn, typ: s.main.Type, prefix: mc.Prefix}
	m.info = describe(ctx, m)
	m.info.Host, m.info.Name = mc.Host, mc.Name
	o := &side{conn: remoteConn, typ: rc.Type, prefix: rc.Prefix}
	o.info = describe(ctx, o)
	o.info.Host, o.info.Name = rc.Host, rc.Name
	return m, o, nil
}

func closeBoth(a, b *side) {
	if a != nil {
		_ = a.conn.Close()
	}
	if b != nil {
		_ = b.conn.Close()
	}
}

// tables lists the prefixed tables (suffix → real name).
func (s *side) tables(ctx context.Context) (map[string]string, error) {
	q := `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`
	if s.pg() {
		q = `SELECT table_name::text FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`
	}
	rows, err := s.conn.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil && strings.HasPrefix(strings.ToLower(n), strings.ToLower(s.prefix)) {
			out[strings.ToLower(n[len(s.prefix):])] = n
		}
	}
	return out, rows.Err()
}

func describe(ctx context.Context, s *side) Info {
	in := Info{Type: s.typ, Prefix: s.prefix}
	vq := "SELECT VERSION()"
	if s.pg() {
		vq = "SHOW server_version"
	}
	_ = s.conn.QueryRowContext(ctx, vq).Scan(&in.Version)
	var v sql.NullString
	if s.conn.QueryRowContext(ctx, `SELECT version FROM flyway_schema_history WHERE success = true AND version IS NOT NULL ORDER BY installed_rank DESC LIMIT 1`).Scan(&v) == nil {
		in.Migration = v.String
	}
	tabs, _ := s.tables(ctx)
	in.Tables = len(tabs)
	count := func(q string) int64 {
		var n int64
		_ = s.conn.QueryRowContext(ctx, q).Scan(&n)
		return n
	}
	if t, ok := tabs["posts"]; ok {
		in.Products = count(`SELECT COUNT(*) FROM ` + s.quote(t) + ` WHERE post_type = 'product' AND post_status <> 'trash'`)
	}
	if t, ok := tabs["wc_orders"]; ok {
		in.Orders = count(`SELECT COUNT(*) FROM ` + s.quote(t) + ` WHERE type = 'shop_order'`)
	}
	if t, ok := tabs["users"]; ok {
		in.Customers = count(`SELECT COUNT(*) FROM ` + s.quote(t))
	}
	for _, t := range tabs {
		in.Rows += count(`SELECT COUNT(*) FROM ` + s.quote(t))
	}
	return in
}

func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// check compares source and destination.
func check(ctx context.Context, src, dst *side) (TestResult, map[string]string, map[string]string) {
	res := TestResult{}
	st, err := src.tables(ctx)
	if err != nil {
		res.Issues = append(res.Issues, "source: "+err.Error())
	}
	dt, err := dst.tables(ctx)
	if err != nil {
		res.Issues = append(res.Issues, "destination: "+err.Error())
	}
	for k := range st {
		if _, ok := dt[k]; !ok {
			res.Missing = append(res.Missing, k)
		}
	}
	sort.Strings(res.Missing)
	if _, ok := dt["cwmerch_settings"]; !ok {
		res.Issues = append(res.Issues, "the destination database is not migrated (run cwmerch's Flyway migrations on it first)")
	}
	if dst.info.Migration == "" {
		res.Issues = append(res.Issues, "the destination database has no Flyway history (run the migrations first)")
	} else if src.info.Migration != "" && versionLess(dst.info.Migration, src.info.Migration) {
		res.Issues = append(res.Issues, fmt.Sprintf("the destination database is at migration %s, the source at %s: migrate it first", dst.info.Migration, src.info.Migration))
	}
	res.Ready = len(res.Issues) == 0
	return res, st, dt
}

// Test connects to the remote database and checks both are ready.
func (s *Service) Test(ctx context.Context, r Remote, direction string) (TestResult, error) {
	m, o, err := s.open(ctx, r)
	if err != nil {
		return TestResult{}, err
	}
	defer closeBoth(m, o)
	src, dst := m, o
	if direction == Import {
		src, dst = o, m
	}
	res, _, _ := check(ctx, src, dst)
	res.Main, res.Remote = m.info, o.info
	return res, nil
}

// Current is the running or last job (nil if none).
func (s *Service) Current() *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	cp := *s.job
	cp.Tables = append([]TableProgress{}, s.job.Tables...)
	return &cp
}

func (s *Service) update(fn func(j *Job)) {
	s.mu.Lock()
	fn(s.job)
	s.mu.Unlock()
}

// MainInfo describes the main database.
func (s *Service) MainInfo(ctx context.Context) Info {
	conn, err := db.OpenRaw(ctx, s.main.Config)
	if err != nil {
		return Info{Type: s.main.Type, Prefix: s.main.Prefix}
	}
	defer conn.Close()
	m := &side{conn: conn, typ: s.main.Type, prefix: s.main.Prefix}
	in := describe(ctx, m)
	in.Host, in.Name = s.main.Config.Host, s.main.Config.Name
	return in
}

// Start launches a clone in the background.
func (s *Service) Start(ctx context.Context, r Remote, direction, conflict string, by uint64) (*Job, error) {
	if conflict == "" {
		conflict = Overwrite
	}
	if conflict != Overwrite && conflict != Skip {
		return nil, fmt.Errorf("conflict must be %q or %q", Overwrite, Skip)
	}
	if direction != Export && direction != Import {
		return nil, fmt.Errorf("direction must be %q or %q", Export, Import)
	}
	s.mu.Lock()
	if s.job != nil && s.job.Status == "running" {
		s.mu.Unlock()
		return nil, ErrRunning
	}
	s.mu.Unlock()
	m, o, err := s.open(ctx, r)
	if err != nil {
		return nil, err
	}
	src, dst := m, o
	if direction == Import {
		src, dst = o, m
	}
	res, st, dt := check(ctx, src, dst)
	if !res.Ready {
		closeBoth(m, o)
		return nil, fmt.Errorf("%w: %s", ErrNotReady, strings.Join(res.Issues, "; "))
	}
	order, err := copyOrder(ctx, src, st, dt)
	if err != nil {
		closeBoth(m, o)
		return nil, err
	}
	job := &Job{ID: uuid.NewString(), Direction: direction, Conflict: conflict, Status: "running", Source: src.info, Target: dst.info,
		StartedAt: time.Now().UTC(), StartedBy: by}
	for _, k := range order {
		job.Tables = append(job.Tables, TableProgress{Table: k, Status: "pending"})
	}
	s.mu.Lock()
	if s.job != nil && s.job.Status == "running" {
		s.mu.Unlock()
		closeBoth(m, o)
		return nil, ErrRunning
	}
	s.job = job
	s.mu.Unlock()
	go s.run(src, dst, st, dt, order, direction, conflict)
	return s.Current(), nil
}

func (s *Service) run(src, dst *side, st, dt map[string]string, order []string, direction, conflict string) {
	defer closeBoth(src, dst)
	ctx := context.Background()
	failed := false
	// one session on the destination: MySQL's sql_mode allows WordPress' zero dates
	conn, err := dst.conn.Conn(ctx)
	if err == nil && !dst.pg() {
		_, err = conn.ExecContext(ctx, `SET SESSION sql_mode = REPLACE(REPLACE(REPLACE(REPLACE(@@SESSION.sql_mode, 'NO_ZERO_DATE', ''), 'NO_ZERO_IN_DATE', ''), 'STRICT_TRANS_TABLES', ''), 'STRICT_ALL_TABLES', '')`)
	}
	if err != nil {
		s.finish(err)
		return
	}
	defer conn.Close()
	for i, key := range order {
		start := time.Now()
		s.update(func(j *Job) { j.Tables[i].Status = "running" })
		var tp TableProgress
		var err error
		before := dst.count(ctx, conn, dt[key])
		if conflict == Overwrite {
			if rel, ok := children[key]; ok && st[rel[0]] != "" && dt[rel[0]] != "" {
				tp.Replaced, err = syncChildren(ctx, src, dst, conn, st[rel[0]], dt[key], rel[1])
			}
		}
		replacedChildren := tp.Replaced
		if err == nil {
			tp, err = copyTable(ctx, src, dst, conn, st[key], dt[key], conflict, func(n int64, total int64) {
				s.update(func(j *Job) {
					j.Tables[i].Copied = n
					j.Tables[i].SourceRows = total
				})
			})
			tp.Replaced += replacedChildren
			tp.BeforeRows = before
		}
		s.update(func(j *Job) {
			t := &j.Tables[i]
			t.SourceRows, t.Copied, t.Replaced, t.BeforeRows, t.AfterRows = tp.SourceRows, tp.Copied, tp.Replaced, tp.BeforeRows, tp.AfterRows
			t.DurationMsec = time.Since(start).Milliseconds()
			t.Status = "done"
			if err != nil {
				t.Status, t.Error = "failed", err.Error()
			}
			j.Copied += tp.Copied
			j.Total += tp.SourceRows
		})
		if err != nil {
			failed = true
			slog.Error("database clone: table failed", "table", key, "error", err)
		}
	}
	if direction == Import && s.OnImported != nil {
		s.OnImported()
	}
	if failed {
		s.finish(errors.New("some tables could not be copied"))
		return
	}
	s.finish(nil)
}

func (s *Service) finish(err error) {
	now := time.Now().UTC()
	s.update(func(j *Job) {
		j.FinishedAt = &now
		j.Status = "done"
		if err != nil {
			j.Status, j.Error = "failed", err.Error()
		}
	})
}

// copyOrder sorts the tables both sides have so referenced tables come first.
func copyOrder(ctx context.Context, src *side, st, dt map[string]string) ([]string, error) {
	var keys []string
	for k := range st {
		if _, ok := dt[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	deps := map[string][]string{}
	q := `SELECT table_name, referenced_table_name FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND referenced_table_name IS NOT NULL`
	if src.pg() {
		q = `SELECT conrelid::regclass::text, confrelid::regclass::text FROM pg_constraint WHERE contype = 'f'`
	}
	if rows, err := src.conn.QueryContext(ctx, q); err == nil {
		for rows.Next() {
			var a, b string
			if rows.Scan(&a, &b) == nil {
				a, b = strings.Trim(strings.ToLower(a), `"`), strings.Trim(strings.ToLower(b), `"`)
				p := strings.ToLower(src.prefix)
				deps[strings.TrimPrefix(a, p)] = append(deps[strings.TrimPrefix(a, p)], strings.TrimPrefix(b, p))
			}
		}
		rows.Close()
	}
	var out []string
	seen := map[string]bool{}
	var visit func(k string, depth int)
	visit = func(k string, depth int) {
		if seen[k] || depth > 50 {
			return
		}
		seen[k] = true
		for _, d := range deps[k] {
			if _, ok := dt[d]; ok {
				visit(d, depth+1)
			}
		}
		out = append(out, k)
	}
	for _, k := range keys {
		visit(k, 0)
	}
	return out, nil
}

type column struct {
	name   string // as the side names it
	lower  string
	binary bool
	time   bool
}

func (s *side) columns(ctx context.Context, table string) ([]column, error) {
	q := `SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`
	if s.pg() {
		q = `SELECT column_name::text, data_type::text FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`
	}
	rows, err := s.conn.QueryContext(ctx, q, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []column
	for rows.Next() {
		var n, t string
		if err := rows.Scan(&n, &t); err != nil {
			return nil, err
		}
		t = strings.ToLower(t)
		out = append(out, column{name: n, lower: strings.ToLower(n),
			binary: strings.Contains(t, "blob") || strings.Contains(t, "binary") || t == "bytea",
			time:   strings.Contains(t, "date") || strings.Contains(t, "timestamp")})
	}
	return out, rows.Err()
}

// keys answers the primary key and the other unique keys (lowercase names).
func (s *side) keys(ctx context.Context, table string) ([]string, [][]string, error) {
	q := `SELECT index_name, column_name FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND non_unique = 0 ORDER BY index_name, seq_in_index`
	if s.pg() {
		q = `SELECT ic.relname::text, a.attname::text
			FROM pg_index i JOIN pg_class ic ON ic.oid = i.indexrelid
			JOIN unnest(i.indkey) WITH ORDINALITY AS k(attnum, n) ON true
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
			WHERE i.indrelid = to_regclass($1) AND i.indisunique AND i.indexprs IS NULL
			ORDER BY i.indisprimary DESC, ic.relname, k.n`
		table = `"` + table + `"`
	}
	rows, err := s.conn.QueryContext(ctx, q, table)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	idx := map[string][]string{}
	var names []string
	primary := ""
	for rows.Next() {
		var n, c string
		if err := rows.Scan(&n, &c); err != nil {
			return nil, nil, err
		}
		if _, ok := idx[n]; !ok {
			names = append(names, n)
		}
		idx[n] = append(idx[n], strings.ToLower(c))
		if n == "PRIMARY" || strings.HasSuffix(n, "_pkey") {
			primary = n
		}
	}
	var others [][]string
	for _, n := range names {
		if n != primary {
			others = append(others, idx[n])
		}
	}
	return idx[primary], others, rows.Err()
}

func (s *side) count(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table string) int64 {
	var n int64
	_ = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.quote(table)).Scan(&n)
	return n
}

var zeroMySQL = "0000-00-00 00:00:00"

// value converts a scanned source value for the destination column.
func value(v any, dst *side, col column) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		if x.IsZero() || x.Year() <= 1 {
			if dst.pg() {
				return "0001-01-01 00:00:00"
			}
			return zeroMySQL
		}
		return x.UTC().Format("2006-01-02 15:04:05.999999")
	case []byte:
		if col.binary {
			return x
		}
		return clean(string(x), dst)
	case string:
		if col.binary {
			return []byte(x)
		}
		if col.time && strings.HasPrefix(x, "0000-00-00") && dst.pg() {
			return "0001-01-01 00:00:00"
		}
		return clean(x, dst)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	}
	return fmt.Sprint(v)
}

func clean(s string, dst *side) string {
	if dst.pg() && strings.IndexByte(s, 0) >= 0 {
		return strings.ReplaceAll(s, "\x00", "")
	}
	return s
}

// renamePrefix rewrites prefix-named options/usermeta keys
// (<prefix>user_roles, <prefix>capabilities...) when the prefixes differ.
func renamePrefix(table string, col column, v any, src, dst *side) any {
	if src.prefix == dst.prefix {
		return v
	}
	if !((table == "options" && col.lower == "option_name") || (table == "usermeta" && col.lower == "meta_key")) {
		return v
	}
	if s, ok := v.(string); ok && strings.HasPrefix(s, src.prefix) {
		return dst.prefix + s[len(src.prefix):]
	}
	return v
}

const batchRows = 500

func copyTable(ctx context.Context, src, dst *side, dconn *sql.Conn, srcTable, dstTable, conflict string, progress func(n, total int64)) (TableProgress, error) {
	tp := TableProgress{Table: dstTable}
	key := strings.ToLower(dstTable[len(dst.prefix):])
	scols, err := src.columns(ctx, srcTable)
	if err != nil {
		return tp, err
	}
	dcols, err := dst.columns(ctx, dstTable)
	if err != nil {
		return tp, err
	}
	// common columns, in the source's order
	byLower := map[string]column{}
	for _, c := range dcols {
		byLower[c.lower] = c
	}
	var sc, dc []column
	for _, c := range scols {
		if d, ok := byLower[c.lower]; ok {
			sc, dc = append(sc, c), append(dc, d)
		}
	}
	if len(sc) == 0 {
		return tp, errors.New("no common column")
	}
	pk, uniques, err := dst.keys(ctx, dstTable)
	if err != nil {
		return tp, err
	}
	spk, _, _ := src.keys(ctx, srcTable)
	tp.SourceRows = src.count(ctx, src.conn, srcTable)
	tp.BeforeRows = dst.count(ctx, dconn, dstTable)
	progress(0, tp.SourceRows)

	pos := map[string]int{}
	for i, c := range dc {
		pos[c.lower] = i
	}
	// source column names for SELECT and the keyset
	var sel []string
	for _, c := range sc {
		sel = append(sel, src.quote(c.name))
	}
	srcName := map[string]string{}
	for _, c := range sc {
		srcName[c.lower] = c.name
	}
	pageKey := spk
	for _, k := range pageKey {
		if _, ok := srcName[k]; !ok {
			pageKey = nil
		}
	}
	perBatch := batchRows
	if lim := 30000 / len(dc); lim < perBatch {
		perBatch = max(lim, 1)
	}
	var last []any
	offset := 0
	for {
		q := `SELECT ` + strings.Join(sel, ", ") + ` FROM ` + src.quote(srcTable)
		var args []any
		if len(pageKey) > 0 {
			var kq []string
			for _, k := range pageKey {
				kq = append(kq, src.quote(srcName[k]))
			}
			if last != nil {
				var ph []string
				for i := range last {
					ph = append(ph, src.ph(i+1))
				}
				q += ` WHERE (` + strings.Join(kq, ", ") + `) > (` + strings.Join(ph, ", ") + `)`
				args = append(args, last...)
			}
			q += ` ORDER BY ` + strings.Join(kq, ", ") + ` LIMIT ` + strconv.Itoa(perBatch)
		} else {
			q += ` LIMIT ` + strconv.Itoa(perBatch) + ` OFFSET ` + strconv.Itoa(offset)
		}
		rows, err := src.conn.QueryContext(ctx, q, args...)
		if err != nil {
			return tp, err
		}
		var batch [][]any
		for rows.Next() {
			vals := make([]any, len(sc))
			ptrs := make([]any, len(sc))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return tp, err
			}
			batch = append(batch, vals)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return tp, err
		}
		if len(batch) == 0 {
			break
		}
		// next keyset position (raw source values)
		if len(pageKey) > 0 {
			lastRow := batch[len(batch)-1]
			last = nil
			for _, k := range pageKey {
				for i, c := range sc {
					if c.lower == k {
						last = append(last, keyArg(lastRow[i]))
					}
				}
			}
		}
		offset += len(batch)
		// convert for the destination
		conv := make([][]any, len(batch))
		for r, row := range batch {
			out := make([]any, len(row))
			for i, v := range row {
				out[i] = renamePrefix(key, dc[i], value(v, dst, dc[i]), src, dst)
			}
			conv[r] = out
		}
		replaced, err := writeBatch(ctx, dst, dconn, dstTable, dc, pk, uniques, pos, conv, conflict == Skip)
		if err != nil {
			return tp, err
		}
		tp.Replaced += replaced
		tp.Copied += int64(len(batch))
		progress(tp.Copied, tp.SourceRows)
		if len(batch) < perBatch {
			break
		}
	}
	if dst.pg() {
		resetIdentity(ctx, dst, dconn, dstTable)
	}
	tp.AfterRows = dst.count(ctx, dconn, dstTable)
	return tp, nil
}

// keyArg makes a keyset value bindable again on the source.
func keyArg(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case uint64:
		return strconv.FormatUint(x, 10)
	}
	return v
}

func writeBatch(ctx context.Context, dst *side, conn *sql.Conn, table string, cols []column, pk []string, uniques [][]string, pos map[string]int, rows [][]any, skip bool) (int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var replaced int64
	// 1. destination rows clashing on another unique key with a different primary key
	if len(pk) > 0 && !skip {
		for _, u := range uniques {
			okCols := true
			for _, c := range append(append([]string{}, u...), pk...) {
				if _, ok := pos[c]; !ok {
					okCols = false
				}
			}
			if !okCols {
				continue
			}
			var args []any
			n := 0
			tuple := func(names []string, row []any) string {
				var ph []string
				for _, c := range names {
					n++
					ph = append(ph, dst.ph(n))
					args = append(args, row[pos[c]])
				}
				return "(" + strings.Join(ph, ", ") + ")"
			}
			quoteList := func(names []string) string {
				var q []string
				for _, c := range names {
					q = append(q, dst.quote(colName(cols, c)))
				}
				return "(" + strings.Join(q, ", ") + ")"
			}
			var ut, pt []string
			for _, r := range rows {
				ut = append(ut, tuple(u, r))
			}
			for _, r := range rows {
				pt = append(pt, tuple(pk, r))
			}
			res, err := tx.ExecContext(ctx, `DELETE FROM `+dst.quote(table)+` WHERE `+quoteList(u)+` IN (`+strings.Join(ut, ", ")+`) AND `+
				quoteList(pk)+` NOT IN (`+strings.Join(pt, ", ")+`)`, args...)
			if err != nil {
				return 0, fmt.Errorf("unique key %v: %w", u, err)
			}
			if k, _ := res.RowsAffected(); k > 0 {
				replaced += k
			}
		}
	}
	// 2. upsert by primary key
	var names []string
	for _, c := range cols {
		names = append(names, dst.quote(c.name))
	}
	var values []string
	var args []any
	n := 0
	for _, r := range rows {
		var ph []string
		for _, v := range r {
			n++
			ph = append(ph, dst.ph(n))
			args = append(args, v)
		}
		values = append(values, "("+strings.Join(ph, ", ")+")")
	}
	var sets []string
	for _, c := range cols {
		if !skip && !contains(pk, c.lower) {
			if dst.pg() {
				sets = append(sets, dst.quote(c.name)+" = EXCLUDED."+dst.quote(c.name))
			} else {
				sets = append(sets, dst.quote(c.name)+" = VALUES("+dst.quote(c.name)+")")
			}
		}
	}
	q := `INSERT INTO ` + dst.quote(table) + ` (` + strings.Join(names, ", ") + `) VALUES ` + strings.Join(values, ", ")
	switch {
	case dst.pg() && len(pk) > 0 && len(sets) > 0:
		var pq []string
		for _, c := range pk {
			pq = append(pq, dst.quote(colName(cols, c)))
		}
		q += ` ON CONFLICT (` + strings.Join(pq, ", ") + `) DO UPDATE SET ` + strings.Join(sets, ", ")
	case dst.pg():
		q += ` ON CONFLICT DO NOTHING`
	case len(sets) > 0:
		q += ` ON DUPLICATE KEY UPDATE ` + strings.Join(sets, ", ")
	default:
		q = `INSERT IGNORE` + strings.TrimPrefix(q, `INSERT`)
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return 0, err
	}
	return replaced, tx.Commit()
}

func colName(cols []column, lower string) string {
	for _, c := range cols {
		if c.lower == lower {
			return c.name
		}
	}
	return lower
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// resetIdentity moves a PostgreSQL identity/serial sequence past the copied ids.
func resetIdentity(ctx context.Context, dst *side, conn *sql.Conn, table string) {
	var col sql.NullString
	qt := `"` + table + `"`
	if conn.QueryRowContext(ctx, `SELECT attname::text FROM pg_attribute WHERE attrelid = to_regclass($1) AND attnum > 0 AND NOT attisdropped
		AND pg_get_serial_sequence($1, attname) IS NOT NULL LIMIT 1`, qt).Scan(&col) != nil || !col.Valid {
		return
	}
	_, err := conn.ExecContext(ctx, `SELECT setval(pg_get_serial_sequence($1, $2), GREATEST(COALESCE((SELECT MAX(`+dst.quote(col.String)+`) FROM `+dst.quote(table)+`), 0), 1))`, qt, col.String)
	if err != nil {
		slog.Warn("database clone: sequence not reset", "table", table, "error", err)
	}
}

// syncChildren removes the destination child rows of the parents the source
// has (they are copied right after from the source), in chunks.
func syncChildren(ctx context.Context, src, dst *side, conn *sql.Conn, srcParent, dstChild, fk string) (int64, error) {
	ppk, _, err := src.keys(ctx, srcParent)
	if err != nil || len(ppk) != 1 {
		return 0, err
	}
	scols, err := src.columns(ctx, srcParent)
	if err != nil {
		return 0, err
	}
	pkName := colName(scols, ppk[0])
	dcols, err := dst.columns(ctx, dstChild)
	if err != nil {
		return 0, err
	}
	fkName := colName(dcols, fk)
	rows, err := src.conn.QueryContext(ctx, `SELECT `+src.quote(pkName)+` FROM `+src.quote(srcParent))
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var v any
		if rows.Scan(&v) == nil {
			if s, ok := value(v, dst, column{}).(string); ok {
				ids = append(ids, s)
			}
		}
	}
	rows.Close()
	var removed int64
	for start := 0; start < len(ids); start += 1000 {
		chunk := ids[start:min(start+1000, len(ids))]
		var ph []string
		args := make([]any, len(chunk))
		for i, id := range chunk {
			ph = append(ph, dst.ph(i+1))
			args[i] = id
		}
		res, err := conn.ExecContext(ctx, `DELETE FROM `+dst.quote(dstChild)+` WHERE `+dst.quote(fkName)+` IN (`+strings.Join(ph, ", ")+`)`, args...)
		if err != nil {
			return removed, err
		}
		n, _ := res.RowsAffected()
		removed += n
	}
	return removed, nil
}

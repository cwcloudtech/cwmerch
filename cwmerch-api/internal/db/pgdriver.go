package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// pgConnector wraps pgx's database/sql connector: every query goes through
// Translate (MySQL dialect → PostgreSQL) and arguments are sent as text, so
// PostgreSQL converts them to the column's type like MySQL does ("5" to a
// bigint, 5 to a text column).
type pgConnector struct {
	inner driver.Connector
	cat   *pgCatalog
}

func (c *pgConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	sc, ok := conn.(*stdlib.Conn)
	if !ok {
		return nil, errors.New("unexpected pgx connection type")
	}
	return &pgConn{Conn: sc, cat: c.cat}, nil
}

func (c *pgConnector) Driver() driver.Driver { return c.inner.Driver() }

// openPostgres opens a translated PostgreSQL pool.
func openPostgres(dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.RuntimeParams["timezone"] = "UTC"
	return sql.OpenDB(&pgConnector{inner: stdlib.GetConnector(*cfg), cat: &pgCatalog{keys: map[string]pgKeys{}}}), nil
}

type pgConn struct {
	*stdlib.Conn
	cat *pgCatalog
}

type pgCatalogView struct {
	ctx  context.Context
	conn *pgx.Conn
	cat  *pgCatalog
}

func (v pgCatalogView) UniqueKeys(table string) ([]string, [][]string) {
	k := v.cat.get(v.ctx, v.conn, table)
	return k.primary, k.others
}

func (v pgCatalogView) Identity(table string) string { return v.cat.get(v.ctx, v.conn, table).identity }

func (c *pgConn) translate(ctx context.Context, q string) Translated {
	return Translate(q, pgCatalogView{ctx: ctx, conn: c.Conn.Conn(), cat: c.cat})
}

// textArgs sends every value as text (times in UTC, MySQL-like booleans).
func textArgs(args []driver.NamedValue) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = a
		switch v := a.Value.(type) {
		case nil, []byte, string:
		case int64:
			out[i].Value = strconv.FormatInt(v, 10)
		case float64:
			out[i].Value = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			if v {
				out[i].Value = "1"
			} else {
				out[i].Value = "0"
			}
		case time.Time:
			out[i].Value = v.UTC().Format("2006-01-02 15:04:05.999999")
		}
	}
	return out
}

func (c *pgConn) CheckNamedValue(nv *driver.NamedValue) error {
	v, err := driver.DefaultParameterConverter.ConvertValue(nv.Value)
	if err != nil {
		return err
	}
	nv.Value = v
	return nil
}

func (c *pgConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.Conn.PrepareContext(ctx, c.translate(ctx, query).SQL)
}

func (c *pgConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *pgConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	t := c.translate(ctx, query)
	return c.Conn.QueryContext(ctx, t.SQL, textArgs(args))
}

type pgResult struct {
	id, rows int64
	hasID    bool
}

func (r pgResult) LastInsertId() (int64, error) {
	if !r.hasID {
		return 0, errors.New("no insert id")
	}
	return r.id, nil
}
func (r pgResult) RowsAffected() (int64, error) { return r.rows, nil }

func (c *pgConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	t := c.translate(ctx, query)
	if !t.ReturningID {
		return c.Conn.ExecContext(ctx, t.SQL, textArgs(args))
	}
	rows, err := c.Conn.QueryContext(ctx, t.SQL, textArgs(args))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := pgResult{hasID: true}
	dest := make([]driver.Value, len(rows.Columns()))
	for {
		if err := rows.Next(dest); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		res.rows++
		if len(dest) > 0 {
			switch v := dest[0].(type) {
			case int64:
				res.id = v
			case string:
				res.id, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	return res, nil
}

// pgCatalog caches each table's unique keys and identity column.
type pgCatalog struct {
	mu   sync.Mutex
	keys map[string]pgKeys
}

type pgKeys struct {
	primary  []string
	others   [][]string
	identity string
}

func (c *pgCatalog) get(ctx context.Context, conn *pgx.Conn, table string) pgKeys {
	c.mu.Lock()
	k, ok := c.keys[table]
	c.mu.Unlock()
	if ok {
		return k
	}
	rows, err := conn.Query(ctx, `SELECT i.indisprimary, array_agg(a.attname::text ORDER BY k.n)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN unnest(i.indkey) WITH ORDINALITY AS k(attnum, n) ON true
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		WHERE c.oid = to_regclass($1) AND i.indisunique AND i.indexprs IS NULL
		GROUP BY i.indexrelid, i.indisprimary ORDER BY i.indisprimary DESC, i.indexrelid`, quoteIdent(table))
	if err != nil {
		return k
	}
	for rows.Next() {
		var primary bool
		var cols []string
		if rows.Scan(&primary, &cols) == nil {
			if primary {
				k.primary = cols
			} else {
				k.others = append(k.others, cols)
			}
		}
	}
	rows.Close()
	_ = conn.QueryRow(ctx, `SELECT attname::text FROM pg_attribute WHERE attrelid = to_regclass($1) AND attnum > 0 AND NOT attisdropped
		AND (attidentity <> '' OR pg_get_serial_sequence($1, attname) IS NOT NULL) LIMIT 1`, quoteIdent(table)).Scan(&k.identity)
	c.mu.Lock()
	c.keys[table] = k
	c.mu.Unlock()
	return k
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

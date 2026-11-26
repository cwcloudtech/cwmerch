// Package db opens the MySQL/MariaDB database goocommerce shares with
// WordPress/WooCommerce, and names its tables with the deployment's prefix.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/go-sql-driver/mysql"
)

// DB is the shared connection pool plus the WordPress table prefix ("wp_" by
// default) every query must go through T() to honour.
type DB struct {
	*sql.DB
	Prefix string
}

var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// Config is how to reach the database.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	Prefix   string
	PoolSize int
}

// DSN is the go-sql-driver DSN for cfg. Times are read and written in UTC
// (WordPress keeps both a local and a *_gmt column; goocommerce fills both).
func (c Config) DSN() string {
	mc := mysql.NewConfig()
	mc.User = c.User
	mc.Passwd = c.Password
	mc.Net = "tcp"
	mc.Addr = c.Host + ":" + c.Port
	mc.DBName = c.Name
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Params = map[string]string{"charset": "utf8mb4"}
	mc.Collation = "utf8mb4_unicode_520_ci"
	mc.AllowNativePasswords = true
	mc.MultiStatements = false
	return mc.FormatDSN()
}

// Connect opens and pings the pool.
func Connect(ctx context.Context, cfg Config) (*DB, error) {
	if !prefixPattern.MatchString(cfg.Prefix) {
		return nil, fmt.Errorf("invalid table prefix %q", cfg.Prefix)
	}
	conn, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, err
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 10
	}
	conn.SetMaxOpenConns(cfg.PoolSize)
	conn.SetMaxIdleConns(cfg.PoolSize)
	conn.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &DB{DB: conn, Prefix: cfg.Prefix}, nil
}

// T is a table name with the prefix, back-quoted: T("posts") = "`wp_posts`".
func (d *DB) T(name string) string { return "`" + d.Prefix + name + "`" }

// Raw is the prefixed name without quotes (for information_schema lookups and
// option names such as "<prefix>user_roles").
func (d *DB) Raw(name string) string { return d.Prefix + name }

// Querier is what both *sql.DB and *sql.Tx offer, so store helpers run
// inside or outside a transaction alike.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// InTx runs fn in a transaction, committing on nil and rolling back otherwise.
func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// IsDuplicate reports a unique-key violation (MySQL 1062).
func IsDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// Package db opens cwmerch's database: the MySQL/MariaDB one it shares with
// WordPress/WooCommerce, or a PostgreSQL one (new projects), and names its
// tables with the deployment's prefix. Queries are written in MySQL's dialect;
// on PostgreSQL they are translated at the driver level (translate.go).
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/go-sql-driver/mysql"
)

// DB is the shared connection pool plus the WordPress table prefix ("wp_" by
// default) every query must go through T() to honour.
type DB struct {
	*sql.DB
	Prefix string
	// Type is MySQL or Postgres.
	Type string
	// Config is how the pool was opened (the clone opens raw connections).
	Config Config
}

// Database types.
const (
	MySQL    = "mysql"
	Postgres = "postgres"
)

// NormalizeType maps DB_TYPE's accepted spellings: postgres, postgresql,
// pgsql, pg, psql → "postgres"; mysql, mariadb, my → "mysql".
func NormalizeType(t string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "mysql", "mariadb", "my":
		return MySQL, nil
	case "postgres", "postgresql", "pgsql", "pg", "psql":
		return Postgres, nil
	}
	return "", fmt.Errorf("unsupported database type %q (mysql, mariadb, postgres)", t)
}

// DefaultPort is 5432 for PostgreSQL, 3306 for MySQL/MariaDB.
func DefaultPort(t string) string {
	if t == Postgres {
		return "5432"
	}
	return "3306"
}

// IsPostgres tells the dialect.
func (d *DB) IsPostgres() bool { return d.Type == Postgres }

var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// Config is how to reach the database.
type Config struct {
	Type     string
	SSLMode  string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	Prefix   string
	PoolSize int
}

// DSN is the driver's DSN for cfg (pgx keyword/value string on PostgreSQL).
func (c Config) DSN() string {
	if c.Type == Postgres {
		ssl := c.SSLMode
		if ssl == "" {
			ssl = "disable"
		}
		kv := func(k, v string) string {
			return k + "='" + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), "'", `\'`) + "' "
		}
		return strings.TrimSpace(kv("host", c.Host) + kv("port", c.Port) + kv("user", c.User) + kv("password", c.Password) +
			kv("dbname", c.Name) + kv("sslmode", ssl))
	}
	return c.mysqlDSN()
}

// mysqlDSN is the go-sql-driver DSN for cfg. Times are read and written in UTC
// (WordPress keeps both a local and a *_gmt column; cwmerch fills both).
func (c Config) mysqlDSN() string {
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
	t, err := NormalizeType(cfg.Type)
	if err != nil {
		return nil, err
	}
	cfg.Type = t
	var conn *sql.DB
	if t == Postgres {
		conn, err = openPostgres(cfg.DSN())
	} else {
		conn, err = sql.Open("mysql", cfg.DSN())
	}
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
	return &DB{DB: conn, Prefix: cfg.Prefix, Type: t, Config: cfg}, nil
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

// IsDuplicate reports a unique-key violation (MySQL 1062, PostgreSQL 23505).
func IsDuplicate(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// OpenRaw opens a pool WITHOUT the PostgreSQL translation (each dialect's own
// SQL), pinged; the database clone uses it to copy between engines.
func OpenRaw(ctx context.Context, cfg Config) (*sql.DB, error) {
	t, err := NormalizeType(cfg.Type)
	if err != nil {
		return nil, err
	}
	cfg.Type = t
	var conn *sql.DB
	if t == Postgres {
		pc, perr := pgx.ParseConfig(cfg.DSN())
		if perr != nil {
			return nil, perr
		}
		pc.RuntimeParams["timezone"] = "UTC"
		conn = stdlib.OpenDB(*pc)
	} else {
		conn, err = sql.Open("mysql", cfg.DSN())
		if err != nil {
			return nil, err
		}
	}
	conn.SetMaxOpenConns(4)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

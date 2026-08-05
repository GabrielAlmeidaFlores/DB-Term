// Package db provides database connectivity, query execution, and schema
// introspection for db-term.
//
// Design note: this package is NOT Bubble Tea-aware. It exposes synchronous
// methods only. Callers (internal/app) wrap those methods in tea.Cmd closures
// to keep Bubble Tea message types out of this package and avoid import cycles.
package db

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/gabrielfloresousion/db-term/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

// BuildDSN constructs the driver-specific connection string from a Connection
// config and the plain-text password (never stored in config).
//
// PostgreSQL uses a URL format (postgresql://user:pass@host:port/db?params)
// so that special characters in any field are percent-encoded and empty
// values never bleed into adjacent key=value tokens.
//
// When Database is empty, each driver falls back to a sensible default:
//   - postgres:   "postgres"  (the standard system database)
//   - mysql:      "" (driver connects to the server without selecting a DB)
//   - sqlserver:  "master"
func BuildDSN(c config.Connection, password string) (string, error) {
	switch c.Driver {
	case "postgres":
		dbName := c.Database
		if dbName == "" {
			dbName = "postgres"
		}
		u := url.URL{
			Scheme: "postgresql",
			User:   url.UserPassword(c.User, password),
			Host:   c.Host + ":" + strconv.Itoa(c.Port),
			Path:   "/" + dbName,
		}
		q := url.Values{}
		q.Set("sslmode", sslMode(c.SSLMode))
		if c.SSLCert != "" {
			q.Set("sslcert", c.SSLCert)
		}
		if c.SSLKey != "" {
			q.Set("sslkey", c.SSLKey)
		}
		if c.SSLRootCert != "" {
			q.Set("sslrootcert", c.SSLRootCert)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil

	case "mysql":
		tls := "false"
		if c.SSLMode != "" && c.SSLMode != "disable" {
			tls = "custom"
		}
		// MySQL DSN: user:password@tcp(host:port)/database?params
		// url.QueryEscape the password to handle @, /, and other special chars.
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
			url.QueryEscape(c.User),
			url.QueryEscape(password),
			c.Host,
			c.Port,
			url.QueryEscape(c.Database),
			tls,
		), nil

	case "sqlserver":
		encrypt := "disable"
		if c.SSLMode != "" && c.SSLMode != "disable" {
			encrypt = "true"
		}
		dbName := c.Database
		if dbName == "" {
			dbName = "master"
		}
		u := url.URL{
			Scheme: "sqlserver",
			User:   url.UserPassword(c.User, password),
			Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		}
		q := url.Values{}
		q.Set("database", dbName)
		q.Set("encrypt", encrypt)
		q.Set("TrustServerCertificate", "true")
		q.Set("dial timeout", "10")
		u.RawQuery = q.Encode()
		return u.String(), nil

	default:
		return "", fmt.Errorf("db: unsupported driver %q", c.Driver)
	}
}

// NewDialector returns the GORM dialector for the given Connection and password.
func NewDialector(c config.Connection, password string) (gorm.Dialector, error) {
	dsn, err := BuildDSN(c, password)
	if err != nil {
		return nil, err
	}

	switch c.Driver {
	case "postgres":
		return postgres.Open(dsn), nil
	case "mysql":
		return mysql.Open(dsn), nil
	case "sqlserver":
		return sqlserver.Open(dsn), nil
	default:
		return nil, fmt.Errorf("db: unsupported driver %q", c.Driver)
	}
}

// sslMode normalises a postgres ssl_mode value, defaulting to "disable".
func sslMode(mode string) string {
	if mode == "" {
		return "disable"
	}
	return mode
}

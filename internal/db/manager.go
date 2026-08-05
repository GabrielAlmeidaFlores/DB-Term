package db

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// silentGORMConfig disables GORM's default logger so that slow-query warnings
// and SQL traces are never written to stdout, which would corrupt the TUI
// alt-screen rendered by Bubble Tea.
var silentGORMConfig = &gorm.Config{
	Logger: logger.Default.LogMode(logger.Silent),
}

// ConnEntry holds the runtime state of a single database connection.
type ConnEntry struct {
	Config config.Connection
	DB     *gorm.DB
	State  types.ConnState
	Err    error // last error, cleared on successful reconnect

	mu     sync.Mutex         // protects cancel only
	cancel context.CancelFunc // non-nil while a query is in flight
}

// cancelQuery cancels any in-flight query for this entry.
func (e *ConnEntry) cancelQuery() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
}

// setCancel stores a new cancel function (replacing the previous one).
func (e *ConnEntry) setCancel(fn context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel() // cancel previous query before overwriting
	}
	e.cancel = fn
}

// clearCancel removes the cancel function after a query finishes normally.
func (e *ConnEntry) clearCancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancel = nil
}

// ConnSnapshot is a safe-to-copy view of a ConnEntry.
// It contains only the fields callers need to read — no mutex, no DB handle.
type ConnSnapshot struct {
	Config config.Connection
	State  types.ConnState
	Err    error
}

// QueryResult is the return value of Manager.RunQuery.
type QueryResult struct {
	Columns []string
	Rows    [][]string
	Elapsed time.Duration
	Err     error
}

// SubConnKey returns the connection map key for a database-level sub-connection.
// Sub-connections reuse the parent's credentials but target a specific database.
// The format "parentName\x00dbName" uses a null byte as a separator that cannot
// appear in user-supplied names.
func SubConnKey(parentConnName, dbName string) string {
	return parentConnName + "\x00" + dbName
}

// Manager is a thread-safe pool of named GORM connections.
// It exposes synchronous methods only; callers wrap them in tea.Cmd.
type Manager struct {
	mu    sync.RWMutex
	conns map[string]*ConnEntry
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{conns: make(map[string]*ConnEntry)}
}

// connectTimeout is the maximum time allowed for opening a connection and
// verifying it with a ping. If the server is unreachable, the call returns
// an error after this duration instead of blocking indefinitely.
const connectTimeout = 10 * time.Second

// Connect opens a GORM connection for the given config and password.
// It is synchronous — wrap it in a tea.Cmd for background execution.
// On success the entry's State is set to StateConnected.
func (m *Manager) Connect(c config.Connection, password string) error {
	dialector, err := NewDialector(c, password)
	if err != nil {
		return fmt.Errorf("db: building dialector for %q: %w", c.Name, err)
	}

	gdb, err := gorm.Open(dialector, silentGORMConfig)
	if err != nil {
		return fmt.Errorf("db: opening connection %q: %w", c.Name, err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("db: getting sql.DB for %q: %w", c.Name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("db: pinging %q: %w", c.Name, enrichConnErr(c, err))
	}

	m.mu.Lock()
	// If a connection with the same name already exists, close it first to
	// avoid orphaning the old *sql.DB and leaking its connection pool.
	if old, exists := m.conns[c.Name]; exists {
		old.cancelQuery()
		m.mu.Unlock()
		if oldSQL, err2 := old.DB.DB(); err2 == nil {
			_ = oldSQL.Close() // best-effort; new connection succeeds regardless
		}
		m.mu.Lock()
	}
	m.conns[c.Name] = &ConnEntry{
		Config: c,
		DB:     gdb,
		State:  types.StateConnected,
	}
	m.mu.Unlock()

	return nil
}

// Disconnect closes the underlying sql.DB and removes the entry.
func (m *Manager) Disconnect(connName string) error {
	m.mu.Lock()
	entry, ok := m.conns[connName]
	if ok {
		delete(m.conns, connName)
	}
	m.mu.Unlock()

	if !ok {
		return nil // already gone
	}

	entry.cancelQuery() // stop any in-flight query

	sqlDB, err := entry.DB.DB()
	if err != nil {
		return fmt.Errorf("db: disconnect %q: getting sql.DB: %w", connName, err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("db: disconnect %q: closing: %w", connName, err)
	}
	return nil
}

// RunQuery executes a raw SQL query on the named connection.
// It is cancellable: a concurrent call to CancelQuery will interrupt it.
// It is synchronous — wrap it in a tea.Cmd for background execution.
func (m *Manager) RunQuery(connName, sql string) QueryResult {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()

	if !ok {
		return QueryResult{Err: fmt.Errorf("db: connection %q not found", connName)}
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry.setCancel(cancel)
	defer entry.clearCancel()

	start := time.Now()
	rows, err := entry.DB.WithContext(ctx).Raw(sql).Rows()
	if err != nil {
		return QueryResult{
			Elapsed: time.Since(start),
			Err:     fmt.Errorf("db: executing query on %q: %w", connName, err),
		}
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{
			Elapsed: time.Since(start),
			Err:     fmt.Errorf("db: reading columns from %q: %w", connName, err),
		}
	}

	colTypes, _ := rows.ColumnTypes()
	typeNames := make([]string, len(cols))
	for i, ct := range colTypes {
		typeNames[i] = ct.DatabaseTypeName()
	}

	var result [][]string
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{
				Elapsed: time.Since(start),
				Err:     fmt.Errorf("db: scanning row from %q: %w", connName, err),
			}
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = valueToString(v, typeNames[i])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{
			Elapsed: time.Since(start),
			Err:     fmt.Errorf("db: iterating rows from %q: %w", connName, err),
		}
	}

	return QueryResult{
		Columns: cols,
		Rows:    result,
		Elapsed: time.Since(start),
	}
}

// CancelQuery cancels the in-flight query for connName.
// No-op if no query is running or the connection does not exist.
func (m *Manager) CancelQuery(connName string) {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()
	if ok {
		entry.cancelQuery()
	}
}

// Ping sends a lightweight query to verify the connection is still alive.
// Used by the keepalive ticker.
func (m *Manager) Ping(connName string) error {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("db: connection %q not found", connName)
	}

	sqlDB, err := entry.DB.DB()
	if err != nil {
		return fmt.Errorf("db: ping %q: getting sql.DB: %w", connName, err)
	}
	if err := sqlDB.PingContext(context.Background()); err != nil {
		return fmt.Errorf("db: ping %q: %w", connName, err)
	}
	return nil
}

// Get returns a ConnSnapshot for connName, or nil if not found.
// The snapshot is safe to read without holding a lock.
func (m *Manager) Get(connName string) *ConnSnapshot {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return &ConnSnapshot{
		Config: entry.Config,
		State:  entry.State,
		Err:    entry.Err,
	}
}

// All returns a snapshot of every ConnEntry, safe to iterate.
func (m *Manager) All() []ConnSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ConnSnapshot, 0, len(m.conns))
	for _, e := range m.conns {
		out = append(out, ConnSnapshot{
			Config: e.Config,
			State:  e.State,
			Err:    e.Err,
		})
	}
	return out
}

// DisconnectAll closes every open connection. Called on graceful shutdown.
func (m *Manager) DisconnectAll() {
	m.mu.RLock()
	names := make([]string, 0, len(m.conns))
	for name := range m.conns {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		_ = m.Disconnect(name) // best-effort; ignore individual errors on shutdown
	}
}

// ListDatabases returns the list of databases available on the instance.
// It uses the existing connection identified by connName.
func (m *Manager) ListDatabases(connName, driver string) ([]string, error) {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("db: connection %q not found", connName)
	}
	return LoadDatabases(entry.DB, driver)
}

// LoadForeignKeys returns foreign-key metadata for a table on a named connection.
// It returns an error when the connection does not exist or introspection fails.
func (m *Manager) LoadForeignKeys(connName, driver, schema, table string) (map[string]*types.ForeignKey, error) {
	m.mu.RLock()
	entry, ok := m.conns[connName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("db: connection %q not found", connName)
	}
	return LoadForeignKeys(entry.DB, driver, schema, table)
}

// valueToString converts a database value to its string representation.
// dbTypeName is the driver-reported column type (e.g. "UNIQUEIDENTIFIER", "BYTEA").
// It handles nil (NULL), []byte with driver-aware formatting, and all other
// types via fmt.Sprintf.
func valueToString(v interface{}, dbTypeName string) string {
	if v == nil {
		return "NULL"
	}
	if b, ok := v.([]byte); ok {
		if len(b) == 16 && strings.EqualFold(dbTypeName, "UNIQUEIDENTIFIER") {
			return formatSQLServerGUID(b)
		}
		if utf8.Valid(b) {
			return string(b)
		}
		return "0x" + hex.EncodeToString(b)
	}
	return fmt.Sprintf("%v", v)
}

// formatSQLServerGUID converts a SQL Server uniqueidentifier []byte (16 bytes,
// mixed-endian) into the standard UUID string representation.
// SQL Server stores the first three UUID components in little-endian order and
// the last two in big-endian order.
func formatSQLServerGUID(b []byte) string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		binary.LittleEndian.Uint32(b[0:4]),
		binary.LittleEndian.Uint16(b[4:6]),
		binary.LittleEndian.Uint16(b[6:8]),
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15],
	)
}

// enrichConnErr wraps known low-level errors with human-readable context.
func enrichConnErr(c config.Connection, err error) error {
	if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
		if c.Driver == "sqlserver" {
			return fmt.Errorf(
				"%w — SQL Server closed the connection during handshake. Possible causes: "+
					"wrong host/port, TCP/IP not enabled in SQL Server Configuration Manager, "+
					"or the server firewall blocks this client IP",
				err,
			)
		}
		return fmt.Errorf("%w — server closed the connection unexpectedly", err)
	}
	if strings.Contains(err.Error(), "connection refused") {
		return fmt.Errorf("%w — nothing is listening on %s:%d", err, c.Host, c.Port)
	}
	if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "name resolution") {
		return fmt.Errorf("%w — hostname %q could not be resolved", err, c.Host)
	}
	return err
}

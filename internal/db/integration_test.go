//go:build integration

// Integration tests require a running PostgreSQL instance.
// Run with: go test ./internal/db/... -tags integration
//
// Environment variables:
//   DB_INTEGRATION_DSN  full postgres DSN (default: host=localhost user=postgres dbname=postgres sslmode=disable)

package db

import (
	"os"
	"testing"
	"time"

	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
)

func integrationConn(t *testing.T) config.Connection {
	t.Helper()
	return config.Connection{
		Name:     "integration-test",
		Driver:   "postgres",
		Host:     envOrDefault("DB_HOST", "localhost"),
		Port:     5432,
		User:     envOrDefault("DB_USER", "postgres"),
		Database: envOrDefault("DB_NAME", "postgres"),
		SSLMode:  "disable",
	}
}

func integrationPassword(t *testing.T) string {
	t.Helper()
	return envOrDefault("DB_PASSWORD", "postgres")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestIntegration_Connect(t *testing.T) {
	mgr := NewManager()
	conn := integrationConn(t)

	if err := mgr.Connect(conn, integrationPassword(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Disconnect(conn.Name)

	entry := mgr.Get(conn.Name)
	if entry == nil {
		t.Fatal("Get after Connect returned nil")
	}
	if entry.State != types.StateConnected {
		t.Errorf("State = %v, want StateConnected", entry.State)
	}
}

func TestIntegration_RunQuery(t *testing.T) {
	mgr := NewManager()
	conn := integrationConn(t)
	if err := mgr.Connect(conn, integrationPassword(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Disconnect(conn.Name)

	result := mgr.RunQuery(conn.Name, "SELECT 1 AS val")
	if result.Err != nil {
		t.Fatalf("RunQuery: %v", result.Err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("RunQuery: got %d rows, want 1", len(result.Rows))
	}
	if result.Rows[0][0] != "1" {
		t.Errorf("RunQuery: row[0][0] = %q, want %q", result.Rows[0][0], "1")
	}
	if result.Elapsed <= 0 {
		t.Error("RunQuery: Elapsed <= 0")
	}
}

func TestIntegration_CancelQuery(t *testing.T) {
	mgr := NewManager()
	conn := integrationConn(t)
	if err := mgr.Connect(conn, integrationPassword(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Disconnect(conn.Name)

	done := make(chan QueryResult, 1)
	go func() {
		// pg_sleep(5) will be cancelled before completion.
		done <- mgr.RunQuery(conn.Name, "SELECT pg_sleep(5)")
	}()

	// Give the query a moment to start then cancel it.
	time.Sleep(100 * time.Millisecond)
	mgr.CancelQuery(conn.Name)

	result := <-done
	if result.Err == nil {
		t.Error("CancelQuery: expected error after cancel, got nil")
	}
}

func TestIntegration_LoadSchemas(t *testing.T) {
	mgr := NewManager()
	conn := integrationConn(t)
	if err := mgr.Connect(conn, integrationPassword(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Disconnect(conn.Name)

	// Access the internal entry directly for the gorm.DB handle (integration only).
	mgr.mu.RLock()
	entry := mgr.conns[conn.Name]
	mgr.mu.RUnlock()

	schemas, err := LoadSchemas(entry.DB, "postgres")
	if err != nil {
		t.Fatalf("LoadSchemas: %v", err)
	}
	if len(schemas) == 0 {
		t.Error("LoadSchemas: returned no schemas")
	}
}

func TestIntegration_Ping(t *testing.T) {
	mgr := NewManager()
	conn := integrationConn(t)
	if err := mgr.Connect(conn, integrationPassword(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer mgr.Disconnect(conn.Name)

	if err := mgr.Ping(conn.Name); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

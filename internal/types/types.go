// Package types defines shared data types used across the db and app packages.
// It has no internal imports — only stdlib — to prevent import cycles.
package types

// ForeignKey describes a foreign-key reference from one column to another table.
type ForeignKey struct {
	Schema string
	Table  string
	Column string
}

// Column represents a single column in a database table or view.
type Column struct {
	Name       string
	DataType   string
	IsNullable bool
	IsPK       bool
	FK         *ForeignKey
}

// Table represents a database table or view within a schema.
type Table struct {
	Name    string
	Schema  string
	IsView  bool
	Columns []Column // populated lazily on tree expand
}

// Schema represents a database schema containing tables and views.
type Schema struct {
	Name   string
	Tables []Table
}

// ConnState represents the lifecycle state of a database connection.
type ConnState int

const (
	StateDisconnected ConnState = iota
	StateConnecting
	StateConnected
	StateError
)

// String returns a human-readable label for the connection state.
func (s ConnState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

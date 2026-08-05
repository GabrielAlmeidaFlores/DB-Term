package db

import (
	"strings"
	"testing"

	"github.com/gabrielfloresousion/db-term/internal/config"
)

func TestBuildDSN_Postgres_Basic(t *testing.T) {
	c := config.Connection{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		User:     "admin",
		Database: "mydb",
		SSLMode:  "disable",
	}
	dsn, err := BuildDSN(c, "secret")
	if err != nil {
		t.Fatalf("BuildDSN postgres: %v", err)
	}

	// New format: postgresql://user:pass@host:port/db?sslmode=disable
	checks := []string{
		"postgresql://",
		"admin:",
		"@localhost:5432",
		"/mydb",
		"sslmode=disable",
	}
	for _, want := range checks {
		if !strings.Contains(dsn, want) {
			t.Errorf("BuildDSN postgres: DSN missing %q\nGot: %s", want, dsn)
		}
	}
}

func TestBuildDSN_Postgres_SSLFields(t *testing.T) {
	c := config.Connection{
		Driver:      "postgres",
		Host:        "db.example.com",
		Port:        5432,
		User:        "app",
		Database:    "prod",
		SSLMode:     "verify-full",
		SSLCert:     "/etc/certs/client.crt",
		SSLKey:      "/etc/certs/client.key",
		SSLRootCert: "/etc/certs/ca.crt",
	}
	dsn, err := BuildDSN(c, "pass")
	if err != nil {
		t.Fatalf("BuildDSN postgres SSL: %v", err)
	}

	for _, want := range []string{
		"sslmode=verify-full",
		"sslcert=",
		"sslkey=",
		"sslrootcert=",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("BuildDSN postgres SSL: DSN missing %q\nGot: %s", want, dsn)
		}
	}
}

func TestBuildDSN_Postgres_DefaultsSSLModeToDisable(t *testing.T) {
	c := config.Connection{Driver: "postgres", Host: "localhost", Port: 5432, User: "u", Database: "d"}
	dsn, err := BuildDSN(c, "p")
	if err != nil {
		t.Fatalf("BuildDSN: %v", err)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("BuildDSN postgres: expected default sslmode=disable\nGot: %s", dsn)
	}
}

func TestBuildDSN_Postgres_EmptyDatabaseNotMisparse(t *testing.T) {
	// Regression: key=value DSN with empty dbname caused pgx to interpret
	// the next token ("port=5432") as the database name.
	// URL format fixes this — empty database becomes the URL path "/".
	c := config.Connection{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		User:     "admin",
		Database: "",
		SSLMode:  "disable",
	}
	dsn, err := BuildDSN(c, "pass")
	if err != nil {
		t.Fatalf("BuildDSN postgres empty db: %v", err)
	}
	// Must not contain "port=5432" as a query value or path component.
	if strings.Contains(dsn, "database=port") || strings.Contains(dsn, "dbname=port") {
		t.Errorf("BuildDSN postgres empty db: port misparse detected\nGot: %s", dsn)
	}
}

func TestBuildDSN_MySQL_Basic(t *testing.T) {
	c := config.Connection{
		Driver:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Database: "devdb",
	}
	dsn, err := BuildDSN(c, "rootpass")
	if err != nil {
		t.Fatalf("BuildDSN mysql: %v", err)
	}

	for _, want := range []string{"root:rootpass@tcp(127.0.0.1:3306)/devdb", "charset=utf8mb4", "parseTime=True"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("BuildDSN mysql: DSN missing %q\nGot: %s", want, dsn)
		}
	}
}

func TestBuildDSN_MySQL_SSLTLSCustom(t *testing.T) {
	c := config.Connection{
		Driver:   "mysql",
		Host:     "db",
		Port:     3306,
		User:     "u",
		Database: "d",
		SSLMode:  "require",
	}
	dsn, err := BuildDSN(c, "p")
	if err != nil {
		t.Fatalf("BuildDSN mysql SSL: %v", err)
	}
	if !strings.Contains(dsn, "tls=custom") {
		t.Errorf("BuildDSN mysql SSL: expected tls=custom\nGot: %s", dsn)
	}
}

func TestBuildDSN_SQLServer_Basic(t *testing.T) {
	c := config.Connection{
		Driver:   "sqlserver",
		Host:     "sqlhost",
		Port:     1433,
		User:     "sa",
		Database: "master",
	}
	dsn, err := BuildDSN(c, "P@ssw0rd")
	if err != nil {
		t.Fatalf("BuildDSN sqlserver: %v", err)
	}

	// URL format: special chars in password are percent-encoded.
	// "@" in "P@ssw0rd" becomes "P%40ssw0rd" which is the correct encoding.
	for _, want := range []string{"sqlserver://sa:", "@sqlhost:1433", "database=master"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("BuildDSN sqlserver: DSN missing %q\nGot: %s", want, dsn)
		}
	}
}

func TestBuildDSN_SQLServer_EncryptedWithSSL(t *testing.T) {
	c := config.Connection{
		Driver:   "sqlserver",
		Host:     "sqlhost",
		Port:     1433,
		User:     "sa",
		Database: "master",
		SSLMode:  "require",
	}
	dsn, err := BuildDSN(c, "pass")
	if err != nil {
		t.Fatalf("BuildDSN sqlserver SSL: %v", err)
	}
	if !strings.Contains(dsn, "encrypt=true") {
		t.Errorf("BuildDSN sqlserver SSL: expected encrypt=true\nGot: %s", dsn)
	}
}

func TestBuildDSN_UnknownDriverReturnsError(t *testing.T) {
	c := config.Connection{Driver: "oracle"}
	_, err := BuildDSN(c, "pass")
	if err == nil {
		t.Error("BuildDSN unknown driver: expected error, got nil")
	}
}

func TestGroupIntoSchemas_GroupsCorrectly(t *testing.T) {
	rows := []tableRow{
		{TableSchema: "public", TableName: "users", TableType: "BASE TABLE"},
		{TableSchema: "public", TableName: "orders", TableType: "BASE TABLE"},
		{TableSchema: "public", TableName: "v_active", TableType: "VIEW"},
		{TableSchema: "analytics", TableName: "events", TableType: "BASE TABLE"},
	}

	schemas := groupIntoSchemas(rows)

	if len(schemas) != 2 {
		t.Fatalf("groupIntoSchemas: got %d schemas, want 2", len(schemas))
	}

	pub := schemas[0]
	if pub.Name != "public" {
		t.Errorf("schemas[0].Name = %q, want %q", pub.Name, "public")
	}
	if len(pub.Tables) != 3 {
		t.Errorf("public: got %d tables, want 3", len(pub.Tables))
	}

	// View should be marked correctly.
	var viewFound bool
	for _, tbl := range pub.Tables {
		if tbl.Name == "v_active" && tbl.IsView {
			viewFound = true
		}
	}
	if !viewFound {
		t.Error("groupIntoSchemas: v_active not marked as view")
	}

	ana := schemas[1]
	if ana.Name != "analytics" {
		t.Errorf("schemas[1].Name = %q, want %q", ana.Name, "analytics")
	}
	if len(ana.Tables) != 1 {
		t.Errorf("analytics: got %d tables, want 1", len(ana.Tables))
	}
}

func TestGroupIntoSchemas_EmptyInput(t *testing.T) {
	schemas := groupIntoSchemas(nil)
	if schemas != nil {
		t.Errorf("groupIntoSchemas(nil): expected nil, got %v", schemas)
	}
}

func TestManager_GetReturnsNilForMissing(t *testing.T) {
	mgr := NewManager()
	if mgr.Get("nonexistent") != nil {
		t.Error("Manager.Get: expected nil for unknown connection, got entry")
	}
}

func TestManager_AllReturnsEmptySliceInitially(t *testing.T) {
	mgr := NewManager()
	entries := mgr.All()
	if len(entries) != 0 {
		t.Errorf("Manager.All: expected empty, got %d entries", len(entries))
	}
}

func TestManager_CancelQueryNoopOnMissing(t *testing.T) {
	mgr := NewManager()
	// Should not panic.
	mgr.CancelQuery("nonexistent")
}

func TestManager_DisconnectNoopOnMissing(t *testing.T) {
	mgr := NewManager()
	if err := mgr.Disconnect("nonexistent"); err != nil {
		t.Errorf("Manager.Disconnect(missing): expected nil error, got %v", err)
	}
}

func TestManager_RunQueryErrorOnMissing(t *testing.T) {
	mgr := NewManager()
	result := mgr.RunQuery("nonexistent", "SELECT 1")
	if result.Err == nil {
		t.Error("Manager.RunQuery(missing): expected error, got nil")
	}
}

func TestManager_PingErrorOnMissing(t *testing.T) {
	mgr := NewManager()
	if err := mgr.Ping("nonexistent"); err == nil {
		t.Error("Manager.Ping(missing): expected error, got nil")
	}
}

func TestManager_LoadForeignKeysErrorOnMissing(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.LoadForeignKeys("nonexistent", "postgres", "public", "orders"); err == nil {
		t.Error("Manager.LoadForeignKeys(missing): expected error, got nil")
	}
}

func TestSubConnKey_ContainsParentAndDatabase(t *testing.T) {
	key := SubConnKey("MAM DEV", "myapp")
	if !strings.Contains(key, "MAM DEV") {
		t.Errorf("SubConnKey: missing parent name, got %q", key)
	}
	if !strings.Contains(key, "myapp") {
		t.Errorf("SubConnKey: missing database name, got %q", key)
	}
}

func TestSubConnKey_UniquePerDatabase(t *testing.T) {
	k1 := SubConnKey("conn", "db1")
	k2 := SubConnKey("conn", "db2")
	if k1 == k2 {
		t.Error("SubConnKey: different databases must produce different keys")
	}
}

func TestSubConnKey_UniquePerParent(t *testing.T) {
	k1 := SubConnKey("conn-a", "db")
	k2 := SubConnKey("conn-b", "db")
	if k1 == k2 {
		t.Error("SubConnKey: different parents must produce different keys")
	}
}

func TestValueToString_SQLServerGUID(t *testing.T) {
	// Regression: SQL Server uniqueidentifier columns were returned as []byte
	// and rendered as garbage because string(b) on 16 non-UTF-8 bytes produces
	// invalid Unicode. formatSQLServerGUID must apply mixed-endian GUID decoding.
	b := []byte{
		0x78, 0x56, 0x34, 0x12,
		0x34, 0x12,
		0x34, 0x12,
		0x12, 0x34,
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc,
	}
	got := valueToString(b, "UNIQUEIDENTIFIER")
	want := "12345678-1234-1234-1234-123456789abc"
	if got != want {
		t.Errorf("GUID: got %q, want %q", got, want)
	}
}

func TestValueToString_ValidUTF8Bytes(t *testing.T) {
	// Valid UTF-8 []byte (JSON, text columns) must still be returned as string.
	b := []byte(`{"key":"value"}`)
	got := valueToString(b, "NVARCHAR")
	if got != `{"key":"value"}` {
		t.Errorf("UTF-8 bytes: got %q", got)
	}
}

func TestValueToString_NonUTF8BinaryHex(t *testing.T) {
	// Non-UTF-8 binary that is not a GUID must be shown as 0x<hex>.
	b := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	got := valueToString(b, "VARBINARY")
	if !strings.HasPrefix(got, "0x") {
		t.Errorf("non-UTF-8 binary: expected 0x prefix, got %q", got)
	}
}

func TestValueToString_NULL(t *testing.T) {
	if got := valueToString(nil, "UNIQUEIDENTIFIER"); got != "NULL" {
		t.Errorf("nil: got %q, want NULL", got)
	}
}

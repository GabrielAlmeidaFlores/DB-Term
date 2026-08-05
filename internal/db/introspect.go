package db

import (
	"fmt"

	"github.com/gabrielfloresousion/db-term/internal/types"
	"gorm.io/gorm"
)

// tableRow is used internally for schema introspection queries.
type tableRow struct {
	TableSchema string
	TableName   string
	TableType   string
}

// LoadDatabases returns the list of user-accessible database names on the server.
// The current connection only needs read access to the system catalog.
func LoadDatabases(db *gorm.DB, driver string) ([]string, error) {
	var names []string

	switch driver {
	case "postgres":
		if err := db.Raw(
			`SELECT datname FROM pg_database
			 WHERE datistemplate = false AND datallowconn = true
			 ORDER BY datname`,
		).Scan(&names).Error; err != nil {
			return nil, fmt.Errorf("db: listing databases (%s): %w", driver, err)
		}

	case "mysql":
		if err := db.Raw(
			`SELECT schema_name
			 FROM information_schema.schemata
			 ORDER BY schema_name`,
		).Scan(&names).Error; err != nil {
			return nil, fmt.Errorf("db: listing databases (%s): %w", driver, err)
		}

	case "sqlserver":
		if err := db.Raw(
			`SELECT name FROM sys.databases
			 WHERE database_id > 4
			 ORDER BY name`,
		).Scan(&names).Error; err != nil {
			return nil, fmt.Errorf("db: listing databases (%s): %w", driver, err)
		}

	default:
		return nil, fmt.Errorf("db: unsupported driver %q", driver)
	}

	return names, nil
}

// LoadSchemas queries the database and returns all schemas with their tables
// and views. Called immediately after a successful connection is established.
func LoadSchemas(db *gorm.DB, driver string) ([]types.Schema, error) {
	var rows []tableRow
	var err error

	switch driver {
	case "postgres":
		err = db.Raw(`
			SELECT table_schema, table_name, table_type
			FROM information_schema.tables
			WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
			ORDER BY table_schema, table_name
		`).Scan(&rows).Error

	case "mysql":
		err = db.Raw(`
			SELECT table_schema, table_name, table_type
			FROM information_schema.tables
			WHERE table_schema = DATABASE()
			ORDER BY table_name
		`).Scan(&rows).Error

	case "sqlserver":
		err = db.Raw(`
			SELECT s.name AS table_schema, t.name AS table_name,
			       'BASE TABLE' AS table_type
			FROM sys.tables t
			JOIN sys.schemas s ON t.schema_id = s.schema_id
			ORDER BY s.name, t.name
		`).Scan(&rows).Error

	default:
		return nil, fmt.Errorf("db: introspect: unsupported driver %q", driver)
	}

	if err != nil {
		return nil, fmt.Errorf("db: introspect: loading schemas (%s): %w", driver, err)
	}

	return groupIntoSchemas(rows), nil
}

// groupIntoSchemas converts a flat list of table rows into a slice of Schema,
// each containing its tables. Views are identified by table_type == "VIEW".
func groupIntoSchemas(rows []tableRow) []types.Schema {
	index := map[string]int{} // schema name → position in result
	var schemas []types.Schema

	for _, r := range rows {
		pos, exists := index[r.TableSchema]
		if !exists {
			pos = len(schemas)
			index[r.TableSchema] = pos
			schemas = append(schemas, types.Schema{Name: r.TableSchema})
		}
		schemas[pos].Tables = append(schemas[pos].Tables, types.Table{
			Name:   r.TableName,
			Schema: r.TableSchema,
			IsView: r.TableType == "VIEW",
		})
	}

	return schemas
}

// LoadColumns returns column metadata for a specific table or view.
// Called lazily when the user expands a table node in the sidebar.
func LoadColumns(db *gorm.DB, driver, schema, table string) ([]types.Column, error) {
	type row struct {
		ColumnName string
		DataType   string
		IsNullable string // "YES" or "NO"
	}

	var rows []row
	var err error

	switch driver {
	case "postgres":
		err = db.Raw(`
			SELECT column_name, data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = ? AND table_name = ?
			ORDER BY ordinal_position
		`, schema, table).Scan(&rows).Error

	case "mysql":
		err = db.Raw(`
			SELECT column_name, data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = ? AND table_name = ?
			ORDER BY ordinal_position
		`, schema, table).Scan(&rows).Error

	case "sqlserver":
		err = db.Raw(`
			SELECT c.name AS column_name,
			       tp.name AS data_type,
			       CASE WHEN c.is_nullable = 1 THEN 'YES' ELSE 'NO' END AS is_nullable
			FROM sys.columns c
			JOIN sys.types tp ON c.user_type_id = tp.user_type_id
			JOIN sys.objects o ON c.object_id = o.object_id
			JOIN sys.schemas s ON o.schema_id = s.schema_id
			WHERE s.name = ? AND o.name = ?
			ORDER BY c.column_id
		`, schema, table).Scan(&rows).Error

	default:
		return nil, fmt.Errorf("db: introspect: unsupported driver %q", driver)
	}

	if err != nil {
		return nil, fmt.Errorf("db: introspect: loading columns for %s.%s (%s): %w",
			schema, table, driver, err)
	}

	cols := make([]types.Column, len(rows))
	for i, r := range rows {
		cols[i] = types.Column{
			Name:       r.ColumnName,
			DataType:   r.DataType,
			IsNullable: r.IsNullable == "YES",
		}
	}

	pks, err := loadPrimaryKeys(db, driver, schema, table)
	if err == nil {
		for i := range cols {
			cols[i].IsPK = pks[cols[i].Name]
		}
	}

	return cols, nil
}

// LoadForeignKeys returns a map of column name to ForeignKey for every FK
// defined on the given table. Returns an empty map (not an error) when no
// foreign keys exist or the catalog query is unsupported for the driver.
func LoadForeignKeys(db *gorm.DB, driver, schema, table string) (map[string]*types.ForeignKey, error) {
	type fkRow struct {
		ColumnName string
		FKSchema   string
		FKTable    string
		FKColumn   string
	}

	var rows []fkRow
	var err error

	switch driver {
	case "postgres":
		err = db.Raw(`
			SELECT kcu.column_name,
			       ref_kcu.table_schema AS fk_schema,
			       ref_kcu.table_name   AS fk_table,
			       ref_kcu.column_name  AS fk_column
			FROM information_schema.table_constraints AS tc
			JOIN information_schema.key_column_usage AS kcu
			  ON tc.constraint_catalog = kcu.constraint_catalog
			 AND tc.constraint_schema  = kcu.constraint_schema
			 AND tc.constraint_name    = kcu.constraint_name
			JOIN information_schema.referential_constraints AS rc
			  ON tc.constraint_catalog = rc.constraint_catalog
			 AND tc.constraint_schema  = rc.constraint_schema
			 AND tc.constraint_name    = rc.constraint_name
			JOIN information_schema.key_column_usage AS ref_kcu
			  ON ref_kcu.constraint_catalog = rc.unique_constraint_catalog
			 AND ref_kcu.constraint_schema  = rc.unique_constraint_schema
			 AND ref_kcu.constraint_name    = rc.unique_constraint_name
			 AND ref_kcu.ordinal_position   = kcu.position_in_unique_constraint
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema    = ?
			  AND tc.table_name      = ?
		`, schema, table).Scan(&rows).Error

	case "mysql":
		err = db.Raw(`
			SELECT kcu.column_name,
			       kcu.referenced_table_schema AS fk_schema,
			       kcu.referenced_table_name   AS fk_table,
			       kcu.referenced_column_name  AS fk_column
			FROM information_schema.key_column_usage AS kcu
			JOIN information_schema.table_constraints AS tc
			  ON kcu.constraint_name = tc.constraint_name
			 AND kcu.table_schema    = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND kcu.table_schema   = ?
			  AND kcu.table_name     = ?
		`, schema, table).Scan(&rows).Error

	case "sqlserver":
		err = db.Raw(`
			SELECT col.name        AS column_name,
			       ref_sch.name    AS fk_schema,
			       ref_tab.name    AS fk_table,
			       ref_col.name    AS fk_column
			FROM sys.foreign_key_columns fkc
			JOIN sys.foreign_keys   fk      ON fkc.constraint_object_id = fk.object_id
			JOIN sys.tables         tab     ON fkc.parent_object_id      = tab.object_id
			JOIN sys.schemas        sch     ON tab.schema_id             = sch.object_id
			JOIN sys.columns        col     ON fkc.parent_object_id      = col.object_id
			                               AND fkc.parent_column_id      = col.column_id
			JOIN sys.tables         ref_tab ON fkc.referenced_object_id  = ref_tab.object_id
			JOIN sys.schemas        ref_sch ON ref_tab.schema_id         = ref_sch.object_id
			JOIN sys.columns        ref_col ON fkc.referenced_object_id  = ref_col.object_id
			                               AND fkc.referenced_column_id  = ref_col.column_id
			WHERE sch.name = ? AND tab.name = ?
		`, schema, table).Scan(&rows).Error

	default:
		return map[string]*types.ForeignKey{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("db: introspect: loading foreign keys for %s.%s (%s): %w",
			schema, table, driver, err)
	}

	fks := make(map[string]*types.ForeignKey, len(rows))
	for _, r := range rows {
		fks[r.ColumnName] = &types.ForeignKey{
			Schema: r.FKSchema,
			Table:  r.FKTable,
			Column: r.FKColumn,
		}
	}
	return fks, nil
}

func loadPrimaryKeys(db *gorm.DB, driver, schema, table string) (map[string]bool, error) {
	var colNames []string

	switch driver {
	case "postgres":
		db.Raw(`
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON tc.constraint_name = kcu.constraint_name
			 AND tc.table_schema    = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
			  AND tc.table_schema    = ?
			  AND tc.table_name      = ?
		`, schema, table).Scan(&colNames)

	case "mysql":
		db.Raw(`
			SELECT column_name
			FROM information_schema.key_column_usage
			WHERE constraint_name = 'PRIMARY'
			  AND table_schema    = ?
			  AND table_name      = ?
		`, schema, table).Scan(&colNames)

	case "sqlserver":
		db.Raw(`
			SELECT c.name
			FROM sys.indexes i
			JOIN sys.index_columns ic ON i.object_id = ic.object_id
			                          AND i.index_id = ic.index_id
			JOIN sys.columns c ON ic.object_id = c.object_id
			                   AND ic.column_id = c.column_id
			JOIN sys.objects o ON i.object_id = o.object_id
			JOIN sys.schemas s ON o.schema_id = s.schema_id
			WHERE i.is_primary_key = 1 AND s.name = ? AND o.name = ?
		`, schema, table).Scan(&colNames)
	}

	pks := make(map[string]bool, len(colNames))
	for _, name := range colNames {
		pks[name] = true
	}
	return pks, nil
}

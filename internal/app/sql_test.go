package app

import (
	"strings"
	"testing"

	"github.com/gabrielfloresousion/db-term/internal/types"
)

func TestParseGoTime_FullPrecision(t *testing.T) {
	// Regression: Go time.Time.String() format must parse without error.
	_, ok := parseGoTime("2026-05-29 12:17:57.305508 +0000 UTC")
	if !ok {
		t.Error("parseGoTime: failed to parse full-precision Go timestamp")
	}
}

func TestParseGoTime_SecondPrecision(t *testing.T) {
	_, ok := parseGoTime("2026-05-29 12:17:57 +0000 UTC")
	if !ok {
		t.Error("parseGoTime: failed to parse second-precision Go timestamp")
	}
}

func TestParseGoTime_NonTimestamp(t *testing.T) {
	_, ok := parseGoTime("hello world")
	if ok {
		t.Error("parseGoTime: should return false for non-timestamp string")
	}
}

func TestParseGoTime_PlainInteger(t *testing.T) {
	_, ok := parseGoTime("42")
	if ok {
		t.Error("parseGoTime: should return false for plain integer")
	}
}

func TestBuildUpdateSQL_TimestampNormalisedInWhere(t *testing.T) {
	// Regression: timestamps must NOT use CAST in WHERE because CAST(col AS TEXT)
	// in PostgreSQL produces "+00" notation while Go produces "+0000 UTC".
	// After fix, the predicate should compare the column directly using the
	// normalised timestamp literal (no CAST wrapper on the column).
	sql := buildUpdateSQL(
		"postgres",
		"users",
		"name",
		"Bob",
		[]string{"id", "name", "created_at"},
		[]string{"1", "Alice", "2026-05-29 12:17:57.305508 +0000 UTC"},
	)

	if strings.Contains(sql, `CAST("created_at"`) {
		t.Errorf("timestamp column should not use CAST in WHERE; got:\n%s", sql)
	}
	if !strings.Contains(sql, `"created_at" = '2026-05-29 12:17:57.305508+00'`) {
		t.Errorf("timestamp should be normalised to +00 notation; got:\n%s", sql)
	}
}

func TestBuildUpdateSQL_PlainStringUsesCASTInWhere(t *testing.T) {
	sql := buildUpdateSQL(
		"postgres",
		"users",
		"name",
		"Bob",
		[]string{"id", "name"},
		[]string{"1", "Alice"},
	)

	if !strings.Contains(sql, `CAST("id" AS TEXT) = '1'`) {
		t.Errorf("plain string should use CAST; got:\n%s", sql)
	}
}

func TestBuildUpdateSQL_NullValueUsesIsNull(t *testing.T) {
	sql := buildUpdateSQL(
		"postgres",
		"users",
		"name",
		"Bob",
		[]string{"deleted_at"},
		[]string{"NULL"},
	)

	if !strings.Contains(sql, `"deleted_at" IS NULL`) {
		t.Errorf("NULL value should produce IS NULL predicate; got:\n%s", sql)
	}
}

func TestBuildUpdateSQL_SingleQuoteEscaped(t *testing.T) {
	sql := buildUpdateSQL(
		"postgres",
		"notes",
		"body",
		"it's fine",
		[]string{"id"},
		[]string{"5"},
	)

	if !strings.Contains(sql, `= 'it''s fine'`) {
		t.Errorf("single quote in new value must be escaped; got:\n%s", sql)
	}
}

func TestBuildUpdateSQL_MySQLBacktickQuoting(t *testing.T) {
	sql := buildUpdateSQL(
		"mysql",
		"users",
		"name",
		"Bob",
		[]string{"id"},
		[]string{"1"},
	)

	if !strings.Contains(sql, "UPDATE `users`") {
		t.Errorf("mysql should use backtick quoting; got:\n%s", sql)
	}
}

func TestInjectOrderBy_AppendsAsc(t *testing.T) {
	sql := injectOrderBy("postgres", `SELECT * FROM "users"`, "name", true)
	if !strings.HasSuffix(strings.TrimSpace(sql), `ORDER BY "name" ASC;`) {
		t.Errorf("unexpected sql: %s", sql)
	}
}

func TestInjectOrderBy_AppendsDesc(t *testing.T) {
	sql := injectOrderBy("postgres", `SELECT * FROM "users"`, "created_at", false)
	if !strings.HasSuffix(strings.TrimSpace(sql), `ORDER BY "created_at" DESC;`) {
		t.Errorf("unexpected sql: %s", sql)
	}
}

func TestInjectOrderBy_ReplacesExistingOrderBy(t *testing.T) {
	base := `SELECT * FROM "users" ORDER BY "id" ASC`
	sql := injectOrderBy("postgres", base, "name", false)
	if strings.Count(sql, "ORDER BY") != 1 {
		t.Errorf("should have exactly one ORDER BY; got:\n%s", sql)
	}
	if !strings.Contains(sql, `ORDER BY "name" DESC`) {
		t.Errorf("should replace old ORDER BY; got:\n%s", sql)
	}
}

func TestInjectOrderBy_StripsSemicolon(t *testing.T) {
	sql := injectOrderBy("postgres", `SELECT * FROM "t";`, "id", true)
	if strings.Count(sql, ";") != 1 {
		t.Errorf("should have exactly one semicolon; got: %s", sql)
	}
}

func TestInjectOrderBy_MySQLBackticks(t *testing.T) {
	sql := injectOrderBy("mysql", "SELECT * FROM `users`", "name", true)
	if !strings.Contains(sql, "ORDER BY `name` ASC") {
		t.Errorf("mysql should use backtick quoting; got: %s", sql)
	}
}

func TestInjectOrderBy_SQLServerBrackets(t *testing.T) {
	sql := injectOrderBy("sqlserver", "SELECT * FROM [users]", "name", false)
	if !strings.Contains(sql, "ORDER BY [name] DESC") {
		t.Errorf("sqlserver should use bracket quoting; got: %s", sql)
	}
}

func TestInjectOrderBy_LimitPreservedAfterOrderBy(t *testing.T) {
	// Regression: SELECT with LIMIT produced "... LIMIT 200 ORDER BY" which
	// is invalid SQL. ORDER BY must precede LIMIT.
	base := `SELECT * FROM public.rastro LIMIT 200;`
	sql := injectOrderBy("postgres", base, "name", true)

	orderIdx := strings.Index(strings.ToLower(sql), "order by")
	limitIdx := strings.Index(strings.ToLower(sql), "limit")
	if orderIdx < 0 {
		t.Fatalf("ORDER BY not found in: %s", sql)
	}
	if limitIdx < 0 {
		t.Fatalf("LIMIT not found in: %s", sql)
	}
	if orderIdx > limitIdx {
		t.Errorf("ORDER BY must precede LIMIT; got:\n%s", sql)
	}
}

func TestInjectOrderBy_LimitOffsetPreserved(t *testing.T) {
	base := `SELECT * FROM t LIMIT 50 OFFSET 100`
	sql := injectOrderBy("postgres", base, "id", false)
	if !strings.Contains(sql, "LIMIT 50 OFFSET 100") {
		t.Errorf("LIMIT/OFFSET should be preserved; got: %s", sql)
	}
	orderIdx := strings.Index(strings.ToLower(sql), "order by")
	limitIdx := strings.Index(strings.ToLower(sql), "limit")
	if orderIdx > limitIdx {
		t.Errorf("ORDER BY must precede LIMIT; got:\n%s", sql)
	}
}

func TestInjectOrderBy_ExistingOrderByWithLimitReplaced(t *testing.T) {
	base := `SELECT * FROM t ORDER BY "old" ASC LIMIT 200`
	sql := injectOrderBy("postgres", base, "new_col", false)
	if strings.Contains(sql, `"old"`) {
		t.Errorf("old ORDER BY column should be replaced; got: %s", sql)
	}
	if !strings.Contains(sql, `ORDER BY "new_col" DESC`) {
		t.Errorf("new ORDER BY not found; got: %s", sql)
	}
	orderIdx := strings.Index(strings.ToLower(sql), "order by")
	limitIdx := strings.Index(strings.ToLower(sql), "limit")
	if orderIdx > limitIdx {
		t.Errorf("ORDER BY must precede LIMIT; got:\n%s", sql)
	}
}

func TestInjectWhere_ExactMatch(t *testing.T) {
	sql := injectWhere("postgres", `SELECT * FROM "users" LIMIT 200`, "name", "Alice")
	if !strings.Contains(sql, `WHERE "name" = 'Alice'`) {
		t.Errorf("expected exact match predicate; got: %s", sql)
	}
}

func TestInjectWhere_LikeWhenPercentPresent(t *testing.T) {
	sql := injectWhere("postgres", `SELECT * FROM "users" LIMIT 200`, "name", "%ali%")
	if !strings.Contains(sql, `WHERE "name" LIKE '%ali%'`) {
		t.Errorf("expected LIKE predicate; got: %s", sql)
	}
}

func TestInjectWhere_OrderByAndLimitPreserved(t *testing.T) {
	base := `SELECT * FROM "t" ORDER BY "id" ASC LIMIT 200`
	sql := injectWhere("postgres", base, "name", "Alice")
	whereIdx := strings.Index(strings.ToLower(sql), "where")
	orderIdx := strings.Index(strings.ToLower(sql), "order by")
	limitIdx := strings.Index(strings.ToLower(sql), "limit")
	if whereIdx < 0 || orderIdx < 0 || limitIdx < 0 {
		t.Fatalf("missing clause in: %s", sql)
	}
	if !(whereIdx < orderIdx && orderIdx < limitIdx) {
		t.Errorf("expected WHERE < ORDER BY < LIMIT; got:\n%s", sql)
	}
}

func TestInjectWhere_ReplacesExistingWhere(t *testing.T) {
	base := `SELECT * FROM "t" WHERE "old" = 'x' LIMIT 200`
	sql := injectWhere("postgres", base, "name", "Alice")
	if strings.Contains(sql, `"old"`) {
		t.Errorf("old WHERE should be replaced; got: %s", sql)
	}
	if !strings.Contains(sql, `WHERE "name" = 'Alice'`) {
		t.Errorf("new WHERE not found; got: %s", sql)
	}
}

func TestInjectWhere_MySQLBackticks(t *testing.T) {
	sql := injectWhere("mysql", "SELECT * FROM `users` LIMIT 200", "name", "Alice")
	if !strings.Contains(sql, "WHERE `name` = 'Alice'") {
		t.Errorf("mysql should use backtick quoting; got: %s", sql)
	}
}

func TestStripWhere_RemovesWhereClause(t *testing.T) {
	sql := stripWhere(`SELECT * FROM "t" WHERE "name" = 'Alice' LIMIT 200`)
	if strings.Contains(strings.ToLower(sql), "where") {
		t.Errorf("WHERE should be removed; got: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT 200") {
		t.Errorf("LIMIT should be preserved; got: %s", sql)
	}
}

func TestStripWhere_PreservesOrderByAndLimit(t *testing.T) {
	sql := stripWhere(`SELECT * FROM "t" WHERE "x" = '1' ORDER BY "id" ASC LIMIT 200`)
	whereIdx := strings.Index(strings.ToLower(sql), "where")
	orderIdx := strings.Index(strings.ToLower(sql), "order by")
	limitIdx := strings.Index(strings.ToLower(sql), "limit")
	if whereIdx >= 0 {
		t.Errorf("WHERE should be removed; got: %s", sql)
	}
	if orderIdx < 0 || limitIdx < 0 {
		t.Errorf("ORDER BY and LIMIT should be preserved; got: %s", sql)
	}
	if orderIdx > limitIdx {
		t.Errorf("ORDER BY must precede LIMIT; got: %s", sql)
	}
}

func TestStripWhere_NoOpWhenNoWhere(t *testing.T) {
	base := `SELECT * FROM "t" LIMIT 200`
	sql := stripWhere(base)
	if strings.Contains(strings.ToLower(sql), "where") {
		t.Errorf("no WHERE to remove, should be clean; got: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT 200") {
		t.Errorf("LIMIT should still be present; got: %s", sql)
	}
}

func TestPreviewSQL_DriverSyntax(t *testing.T) {
	// Regression: SQL Server does not support LIMIT — must use SELECT TOP N.
	cases := []struct {
		driver, schema, table string
		wantContains          string
		wantNotContains       string
	}{
		{"sqlserver", "dbo", "users", "SELECT TOP 200", "LIMIT"},
		{"sqlserver", "dbo", "users", "[dbo].[users]", ""},
		{"postgres", "public", "users", `"public"."users"`, "TOP"},
		{"postgres", "public", "users", "LIMIT 200", ""},
		{"mysql", "mydb", "orders", "`mydb`.`orders`", "TOP"},
		{"mysql", "mydb", "orders", "LIMIT 200", ""},
		{"sqlserver", "dbo", "tab]le", "[tab]]le]", ""},
	}
	for _, tc := range cases {
		sql := previewSQL(tc.driver, tc.schema, tc.table)
		if tc.wantContains != "" && !strings.Contains(sql, tc.wantContains) {
			t.Errorf("previewSQL(%q,%q,%q): want %q in %q", tc.driver, tc.schema, tc.table, tc.wantContains, sql)
		}
		if tc.wantNotContains != "" && strings.Contains(sql, tc.wantNotContains) {
			t.Errorf("previewSQL(%q,%q,%q): must not contain %q in %q", tc.driver, tc.schema, tc.table, tc.wantNotContains, sql)
		}
	}
}

func TestForeignKeySQL_DriverSyntaxAndEscaping(t *testing.T) {
	cases := []struct {
		driver string
		fk     types.ForeignKey
		value  string
		wants  []string
	}{
		{"postgres", types.ForeignKey{Schema: `pub"lic`, Table: "customers", Column: "external_id"}, "it's-42", []string{`"pub""lic"."customers"`, `WHERE "external_id" = 'it''s-42'`, "LIMIT 200"}},
		{"mysql", types.ForeignKey{Schema: "shop", Table: "customers", Column: "external`id"}, "42", []string{"`shop`.`customers`", "WHERE `external``id` = '42'", "LIMIT 200"}},
		{"sqlserver", types.ForeignKey{Schema: "dbo", Table: "customers", Column: "external]id"}, "42", []string{"SELECT TOP 200", "[dbo].[customers]", "WHERE [external]]id] = '42'"}},
	}

	for _, tc := range cases {
		sql := foreignKeySQL(tc.driver, tc.fk, tc.value)
		for _, want := range tc.wants {
			if !strings.Contains(sql, want) {
				t.Errorf("foreignKeySQL(%q) = %q, want %q", tc.driver, sql, want)
			}
		}
	}
}

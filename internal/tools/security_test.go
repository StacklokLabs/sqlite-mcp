package tools

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
)

// newReadOnlyDB seeds a database file with a known row and reopens it through
// database.New in read-only mode, the server's default posture.
func newReadOnlyDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "protected.db")

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
		INSERT INTO users (id, name) VALUES (1, 'ORIGINAL')`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	db, err := database.New(dbPath, true) // read-only
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, dbPath
}

func readName(t *testing.T, dbPath string) string {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()
	var name string
	require.NoError(t, conn.QueryRow("SELECT name FROM users WHERE id = 1").Scan(&name))
	return name
}

func isError(result *mcp.CallToolResult) bool {
	return result != nil && result.IsError
}

// TestReadOnlyMode_ExecuteQueryRejectsStatementChaining covers GHSA-q433-mp6c-8v4f
// vector 1: SELECT-prefixed statement chaining through execute_query.
func TestReadOnlyMode_ExecuteQueryRejectsStatementChaining(t *testing.T) {
	db, dbPath := newReadOnlyDB(t)
	qt := New(db)
	ctx := context.Background()
	proof := filepath.Join(filepath.Dir(dbPath), "PROOF_execquery.db")

	cases := []struct {
		name  string
		query string
	}{
		{"attach and create arbitrary file", "SELECT 1; ATTACH '" + proof + "' AS p; CREATE TABLE p.t(x)"},
		{"self-attach and modify protected db", "SELECT 1; ATTACH '" + dbPath + "' AS w; UPDATE w.users SET name='PWNED' WHERE id=1"},
		{"attach and read an unexposed db", "SELECT 1; ATTACH '" + dbPath + "' AS s; SELECT name FROM s.users"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{Params: mcp.CallToolParams{
				Name:      toolExecuteQuery,
				Arguments: map[string]interface{}{argQuery: tc.query},
			}}
			result, err := qt.HandleTool(ctx, req)
			require.NoError(t, err)
			assert.True(t, isError(result), "chained statement must be rejected")
		})
	}

	assert.NoFileExists(t, proof, "no arbitrary file may be created")
	assert.Equal(t, "ORIGINAL", readName(t, dbPath), "protected row must be unchanged")
}

// TestReadOnlyMode_DescribeTableRejectsInjection covers GHSA-q433-mp6c-8v4f
// vector 2: SQL injection via the describe_table table_name argument.
func TestReadOnlyMode_DescribeTableRejectsInjection(t *testing.T) {
	db, dbPath := newReadOnlyDB(t)
	qt := New(db)
	ctx := context.Background()
	proof := filepath.Join(filepath.Dir(dbPath), "PROOF_describe.db")

	injection := "x); ATTACH '" + proof + "' AS q; CREATE TABLE q.t(x); --"
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      toolDescribeTable,
		Arguments: map[string]interface{}{argTableName: injection},
	}}
	result, err := qt.HandleTool(ctx, req)
	require.NoError(t, err)
	// The crafted name is not a real table, so the handler reports it as not
	// found; crucially, the injected ATTACH/CREATE must not have executed.
	assert.True(t, isError(result))
	assert.NoFileExists(t, proof, "injected statement must not create a file")

	// A legitimate table name still returns its schema.
	ok := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      toolDescribeTable,
		Arguments: map[string]interface{}{argTableName: "users"},
	}}
	okResult, err := qt.HandleTool(ctx, ok)
	require.NoError(t, err)
	assert.False(t, isError(okResult), "describe_table must still work for real tables")
}

// TestReadOnlyMode_LegitimateQueriesStillWork guards against over-blocking.
func TestReadOnlyMode_LegitimateQueriesStillWork(t *testing.T) {
	db, _ := newReadOnlyDB(t)
	qt := New(db)
	ctx := context.Background()

	for _, q := range []string{
		"SELECT * FROM users",
		"SELECT name FROM users WHERE id = 1;",      // single trailing semicolon
		"SELECT name FROM users WHERE name = 'a;b'", // semicolon inside a literal
		"SELECT 1 -- trailing comment\n",            // trailing comment
	} {
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{
			Name:      toolExecuteQuery,
			Arguments: map[string]interface{}{argQuery: q},
		}}
		result, err := qt.HandleTool(ctx, req)
		require.NoError(t, err)
		assert.False(t, isError(result), "legitimate single-statement query must be allowed: %q", q)
	}
}

func TestHasMultipleStatements(t *testing.T) {
	multi := []string{
		"SELECT 1; ATTACH 'x' AS p",
		"SELECT 1;SELECT 2",
		"SELECT 1; -- c\nSELECT 2",
	}
	single := []string{
		"SELECT 1",
		"SELECT 1;",
		"SELECT 1;   ",
		"SELECT 1; -- trailing comment",
		"SELECT ';' ; ",
		"SELECT \"a;b\" FROM t",
	}
	for _, s := range multi {
		assert.True(t, hasMultipleStatements(s), "want multiple: %q", s)
	}
	for _, s := range single {
		assert.False(t, hasMultipleStatements(s), "want single: %q", s)
	}
}

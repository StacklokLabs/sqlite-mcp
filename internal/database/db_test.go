package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a test database with sample data
	db, err := New(InMemoryDB, false)
	require.NoError(t, err)
	defer db.Close()

	// Create test table and insert sample data
	_, err = db.Execute(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			age INTEGER
		)
	`)
	require.NoError(t, err)

	_, err = db.Execute(`
		INSERT INTO users (name, email, age) VALUES 
		('Alice', 'alice@example.com', 30),
		('Bob', 'bob@example.com', 25)
	`)
	require.NoError(t, err)

	// Save to file
	_, err = db.Execute("ATTACH DATABASE ? AS disk", dbPath)
	require.NoError(t, err)

	_, err = db.Execute("CREATE TABLE disk.users AS SELECT * FROM users")
	require.NoError(t, err)

	return dbPath
}

func TestNew(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		dbPath := createTestDB(t)

		db, err := New(dbPath, false)
		require.NoError(t, err)
		assert.NotNil(t, db)
		assert.Equal(t, dbPath, db.Path())

		err = db.Close()
		assert.NoError(t, err)
	})

	t.Run("non-existent file", func(t *testing.T) {
		db, err := New("/non/existent/path.db", false)
		assert.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "database file does not exist")
	})
}

func TestQuery(t *testing.T) {
	dbPath := createTestDB(t)
	db, err := New(dbPath, false)
	require.NoError(t, err)
	defer db.Close()

	t.Run("select all users", func(t *testing.T) {
		results, err := db.Query("SELECT * FROM users ORDER BY id")
		require.NoError(t, err)
		assert.Len(t, results, 2)

		assert.Equal(t, "Alice", results[0]["name"])
		assert.Equal(t, "alice@example.com", results[0]["email"])
		assert.Equal(t, int64(30), results[0]["age"])
	})

	t.Run("select with parameters", func(t *testing.T) {
		results, err := db.Query("SELECT * FROM users WHERE age > ?", 27)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Alice", results[0]["name"])
	})

	t.Run("invalid query", func(t *testing.T) {
		_, err := db.Query("SELECT * FROM non_existent_table")
		assert.Error(t, err)
	})
}

func TestExecute(t *testing.T) {
	dbPath := createTestDB(t)
	db, err := New(dbPath, false)
	require.NoError(t, err)
	defer db.Close()

	t.Run("insert new user", func(t *testing.T) {
		rowsAffected, err := db.Execute(
			"INSERT INTO users (name, email, age) VALUES (?, ?, ?)",
			"Charlie", "charlie@example.com", 35,
		)
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)

		// Verify insertion
		results, err := db.Query("SELECT COUNT(*) as count FROM users")
		require.NoError(t, err)
		assert.Equal(t, int64(3), results[0]["count"])
	})

	t.Run("update user", func(t *testing.T) {
		rowsAffected, err := db.Execute("UPDATE users SET age = ? WHERE name = ?", 31, "Alice")
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)
	})

	t.Run("delete user", func(t *testing.T) {
		rowsAffected, err := db.Execute("DELETE FROM users WHERE name = ?", "Bob")
		require.NoError(t, err)
		assert.Equal(t, int64(1), rowsAffected)
	})
}

func TestGetTables(t *testing.T) {
	dbPath := createTestDB(t)
	db, err := New(dbPath, false)
	require.NoError(t, err)
	defer db.Close()

	tables, err := db.GetTables()
	require.NoError(t, err)
	assert.Contains(t, tables, "users")
}

func TestGetTableSchema(t *testing.T) {
	dbPath := createTestDB(t)
	db, err := New(dbPath, false)
	require.NoError(t, err)
	defer db.Close()

	schema, err := db.GetTableSchema("users")
	require.NoError(t, err)
	assert.Len(t, schema, 4) // id, name, email, age

	// Check first column (id)
	assert.Equal(t, "id", schema[0]["name"])
	assert.Contains(t, []string{"INTEGER", "INT"}, schema[0]["type"]) // SQLite may return either
	assert.Equal(t, int64(0), schema[0]["pk"])                        // Primary key flag
}

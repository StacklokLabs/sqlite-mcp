// Package testutil provides common test utilities for the SQLite MCP server
package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
)

// CreateTestDB creates a test SQLite database with sample data
func CreateTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a test database with sample data directly to file
	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer conn.Close()

	// Create test table and insert sample data
	_, err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE,
			age INTEGER
		)
	`)
	require.NoError(t, err)

	_, err = conn.Exec(`
		INSERT INTO users (name, email, age) VALUES 
		('Alice', 'alice@example.com', 30),
		('Bob', 'bob@example.com', 25)
	`)
	require.NoError(t, err)

	// Create another test table
	_, err = conn.Exec(`
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			price REAL
		)
	`)
	require.NoError(t, err)

	_, err = conn.Exec(`
		INSERT INTO products (name, price) VALUES 
		('Widget', 9.99),
		('Gadget', 19.99)
	`)
	require.NoError(t, err)

	// Open with our database wrapper
	db, err := database.New(dbPath, false)
	require.NoError(t, err)

	return db
}

// GetTextContent is a helper function to extract text from MCP Content
func GetTextContent(t *testing.T, content mcp.Content) string {
	t.Helper()
	textContent, ok := mcp.AsTextContent(content)
	require.True(t, ok, "Expected TextContent")
	return textContent.Text
}

// GetTextResourceContents is a helper function to extract text from MCP ResourceContents
func GetTextResourceContents(t *testing.T, content mcp.ResourceContents) string {
	t.Helper()
	textContent, ok := mcp.AsTextResourceContents(content)
	require.True(t, ok, "Expected TextResourceContents")
	return textContent.Text
}

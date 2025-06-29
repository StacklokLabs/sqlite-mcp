// Package database provides SQLite database connection and query functionality
package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

const (
	// InMemoryDB represents the SQLite in-memory database identifier
	InMemoryDB = ":memory:"
)

// DB wraps a SQLite database connection with common operations
type DB struct {
	conn *sql.DB
	path string
}

// New creates a new database connection
func New(dbPath string, readOnly bool) (*DB, error) {
	// Check if database file exists (skip check for in-memory databases)
	if dbPath != InMemoryDB {
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("database file does not exist: %s", dbPath)
		}
	}

	// Construct the database URI with appropriate parameters
	var dsn string
	if dbPath == InMemoryDB {
		dsn = InMemoryDB
	} else {
		// Convert file path to URI format
		dsn = fmt.Sprintf("file:%s", dbPath)

		// Add query parameters based on mode
		if readOnly {
			dsn += "?mode=ro"
		} else {
			dsn += "?mode=rwc"
		}

		// Configure SQLite for container environments with restricted filesystem access
		dsn += "&_journal_mode=off&_temp_store=memory&_synchronous=off&_cache_size=-64000&_mmap_size=0"
	}

	fmt.Printf("Connecting to database: %s\n", dsn)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := conn.Ping(); err != nil {
		if err := conn.Close(); err != nil {
			return nil, fmt.Errorf("failed to close database connection after ping error: %w", err)
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{
		conn: conn,
		path: dbPath,
	}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Query executes a SELECT query and returns the results
func (db *DB) Query(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// Execute runs an INSERT, UPDATE, or DELETE statement
func (db *DB) Execute(statement string, args ...interface{}) (int64, error) {
	result, err := db.conn.Exec(statement, args...)
	if err != nil {
		return 0, fmt.Errorf("execution failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// GetTables returns a list of all tables in the database
func (db *DB) GetTables() ([]string, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}

	var tables []string
	for _, row := range rows {
		if name, ok := row["name"].(string); ok {
			tables = append(tables, name)
		}
	}

	return tables, nil
}

// GetTableSchema returns the schema information for a specific table
func (db *DB) GetTableSchema(tableName string) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	return db.Query(query)
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

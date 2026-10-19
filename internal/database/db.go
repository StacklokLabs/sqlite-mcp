// Package database provides SQLite database connection and query functionality
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	sqlite "modernc.org/sqlite" // Pure Go SQLite driver
)

// sqliteLimitAttached is SQLITE_LIMIT_ATTACHED, the run-time limit on the
// number of attached databases. Setting it to zero makes the engine refuse
// ATTACH. See https://www.sqlite.org/c3ref/c_limit_attached.html.
const sqliteLimitAttached = 7

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
			// Defense in depth: query_only blocks every write on the
			// connection, including writes to databases reached via ATTACH,
			// which mode=ro alone does not prevent.
			dsn += "&_pragma=query_only(1)"
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

	// Block ATTACH DATABASE at the engine level. The server never needs it,
	// and disabling it stops a crafted statement from opening another file,
	// which is the core of the read-only bypass and the cross-database read in
	// GHSA-q433-mp6c-8v4f, independently of any SQL-text checks. The
	// attached-databases limit is bound to a single connection, so the pool is
	// pinned to one connection and the limit is set on it. Pinning to one
	// connection is also what an in-memory database requires to stay coherent.
	conn.SetMaxOpenConns(1)
	if err := disableAttach(conn); err != nil {
		if cerr := conn.Close(); cerr != nil {
			return nil, fmt.Errorf("%w (and failed to close: %v)", err, cerr)
		}
		return nil, err
	}

	return &DB{
		conn: conn,
		path: dbPath,
	}, nil
}

// disableAttach sets SQLITE_LIMIT_ATTACHED to zero on the pool's connection so
// the engine refuses ATTACH DATABASE. The caller must have pinned the pool to a
// single connection, since the limit applies only to the connection it is set
// on.
func disableAttach(conn *sql.DB) error {
	c, err := conn.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := sqlite.Limit(c, sqliteLimitAttached, 0); err != nil {
		return fmt.Errorf("failed to disable ATTACH: %w", err)
	}
	return nil
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

// GetTableSchema returns the schema information for a specific table.
//
// The table name is passed as a bound parameter to the pragma_table_info
// table-valued function rather than interpolated into the SQL text, so a
// crafted table name cannot break out of the query (CWE-89).
func (db *DB) GetTableSchema(tableName string) ([]map[string]interface{}, error) {
	return db.Query("SELECT * FROM pragma_table_info(?)", tableName)
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

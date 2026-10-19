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

	// Pinning to one connection is what an in-memory database requires to stay
	// coherent.
	conn.SetMaxOpenConns(1)

	return &DB{
		conn: conn,
		path: dbPath,
	}, nil
}

// withConn runs fn on a pooled connection that has ATTACH DATABASE disabled.
//
// Blocking ATTACH at the engine level stops a crafted statement from opening
// another file, which is the core of the read-only bypass and the
// cross-database read in GHSA-q433-mp6c-8v4f, independently of any SQL-text
// checks. The server never needs ATTACH. SQLITE_LIMIT_ATTACHED is bound to a
// single connection and the pool can replace a connection at any time (the
// driver discards one after an interrupted query), so the limit is applied to
// the connection that runs each statement rather than once at open. Setting it
// is a single C call and idempotent.
func (db *DB) withConn(fn func(*sql.Conn) error) error {
	ctx := context.Background()
	c, err := db.conn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := sqlite.Limit(c, sqliteLimitAttached, 0); err != nil {
		return fmt.Errorf("failed to disable ATTACH: %w", err)
	}
	return fn(c)
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
	var results []map[string]interface{}
	err := db.withConn(func(c *sql.Conn) error {
		var err error
		results, err = queryRows(c, query, args...)
		return err
	})
	return results, err
}

func queryRows(c *sql.Conn, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := c.QueryContext(context.Background(), query, args...)
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
	var rowsAffected int64
	err := db.withConn(func(c *sql.Conn) error {
		result, err := c.ExecContext(context.Background(), statement, args...)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}
		rowsAffected, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		return nil
	})
	return rowsAffected, err
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

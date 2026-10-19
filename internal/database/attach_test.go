package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestAttachIsDisabled checks the engine-level defense from GHSA-q433-mp6c-8v4f:
// ATTACH DATABASE must be refused by the connection itself, so a statement that
// slips past the tool-layer checks still cannot open another file. Covers both
// read-only and read-write modes, since the server never needs ATTACH.
func TestAttachIsDisabled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "main.db")
	seed, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = seed.Exec("CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	target := filepath.Join(dir, "attached.db")

	for _, readOnly := range []bool{true, false} {
		mode := "read-write"
		if readOnly {
			mode = "read-only"
		}
		t.Run(mode, func(t *testing.T) {
			db, err := New(dbPath, readOnly)
			require.NoError(t, err)
			defer db.Close()

			_, err = db.Query("ATTACH ? AS evil", target)
			assert.Error(t, err, "ATTACH must be refused by the engine")
			assert.NoFileExists(t, target, "a refused ATTACH must not create the file")

			// A normal query must still work.
			_, err = db.Query("SELECT 1")
			assert.NoError(t, err)
		})
	}
}

// TestAttachIsDisabledAfterReconnect checks that the ATTACH limit survives the
// pool replacing its connection. SQLITE_LIMIT_ATTACHED is bound to a single
// connection, and the driver discards a connection after an interrupted query,
// so a limit set only once at open would be lost on the replacement.
func TestAttachIsDisabledAfterReconnect(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "main.db")
	seed, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = seed.Exec("CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	target := filepath.Join(dir, "attached.db")

	for _, readOnly := range []bool{true, false} {
		mode := "read-write"
		if readOnly {
			mode = "read-only"
		}
		t.Run(mode, func(t *testing.T) {
			db, err := New(dbPath, readOnly)
			require.NoError(t, err)
			defer db.Close()

			// Keep no idle connections, so every call runs on a freshly
			// opened connection, as after the pool discards a bad one.
			db.conn.SetMaxIdleConns(0)

			_, err = db.Query("SELECT 1")
			require.NoError(t, err)

			_, err = db.Execute("ATTACH ? AS evil", target)
			assert.Error(t, err, "ATTACH must be refused on a replacement connection")
			assert.NoFileExists(t, target, "a refused ATTACH must not create the file")
		})
	}
}

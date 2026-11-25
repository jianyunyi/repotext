package control

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteMigratesLegacyTasksTableForWorkKeyAndOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE tasks(id TEXT PRIMARY KEY,status TEXT NOT NULL,task_json BLOB NOT NULL,created_at INTEGER NOT NULL,finished_at INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var columns int
	if err = store.db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('tasks') WHERE name='work_key'").Scan(&columns); err != nil || columns != 1 {
		t.Fatalf("work_key column count=%d err=%v", columns, err)
	}
	var tables int
	if err = store.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='outbox'").Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("outbox table count=%d err=%v", tables, err)
	}
}

package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupDatabaseCreatesIntegrityCheckedCopy(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	output := filepath.Join(dir, "backup.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE items(value TEXT); INSERT INTO items(value) VALUES ('ok')"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = backupDatabase(source, output); err != nil {
		t.Fatal(err)
	}
	check, err := sql.Open("sqlite", output)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var value string
	if err = check.QueryRow("SELECT value FROM items").Scan(&value); err != nil || value != "ok" {
		t.Fatalf("backup value=%q err=%v", value, err)
	}
	if info, err := os.Stat(output); err != nil {
		t.Fatalf("backup stat err=%v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("backup permissions info=%v", info)
	}
}

func TestBackupDatabaseRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	output := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupDatabase(source, output); err == nil {
		t.Fatal("backup overwrote an existing destination")
	}
	body, err := os.ReadFile(output)
	if err != nil || string(body) != "keep" {
		t.Fatalf("destination changed: %q err=%v", body, err)
	}
}

func TestConcurrentBackupDatabasePreservesOneCompleteBackup(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	output := filepath.Join(dir, "backup.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE items(value BLOB);
		WITH RECURSIVE rows(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM rows WHERE n<4096)
		INSERT INTO items(value) SELECT zeroblob(1024) FROM rows`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(source, 0o600); err != nil {
		t.Fatal(err)
	}

	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			results <- backupDatabase(source, output)
		}()
	}
	close(start)
	successes := 0
	var failures []error
	for i := 0; i < callers; i++ {
		if err := <-results; err == nil {
			successes++
		} else {
			failures = append(failures, err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful backups=%d, want 1; failures=%v", successes, failures)
	}
	check, err := sql.Open("sqlite", readonlySQLiteDSN(output))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	if err = checkIntegrity(check); err != nil {
		t.Fatalf("surviving backup integrity: %v", err)
	}
	var rows int
	if err = check.QueryRow("SELECT count(*) FROM items WHERE length(value)=1024").Scan(&rows); err != nil || rows != 4096 {
		t.Fatalf("surviving backup rows=%d, want 4096; err=%v", rows, err)
	}
}

func TestBackupDatabasePreservesDestinationCreatedAfterValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	output := filepath.Join(dir, "backup.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE items(value TEXT); INSERT INTO items(value) VALUES ('keep')"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(source, 0o600); err != nil {
		t.Fatal(err)
	}
	validatedOutput, err := validateDestination(output)
	if err != nil {
		t.Fatal(err)
	}
	// Another caller finishes its backup after this caller validates the path.
	if err = backupDatabase(source, output); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err = backupValidatedDatabase(source, validatedOutput); err == nil {
		t.Fatal("backup accepted a destination created after validation")
	}
	after, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("another caller's completed backup changed: err=%v", err)
	}
}

func TestBackupDatabaseRemovesFailedOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	output := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupDatabase(source, output); err == nil {
		t.Fatal("corrupt source accepted")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("failed backup output remains: err=%v", err)
	}
}

func TestBackupDatabaseRejectsRelativePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path semantics are covered by the POSIX deployment check")
	}
	if err := backupDatabase("relative.db", "/tmp/repotest-backup-test.db"); err == nil {
		t.Fatal("relative source path accepted")
	}
}

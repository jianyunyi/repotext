package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "source SQLite database")
	outPath := flag.String("out", "", "new backup database")
	flag.Parse()
	if err := backupDatabase(*dbPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "backup failed:", err)
		os.Exit(1)
	}
	fmt.Println("BACKUP_INTEGRITY_PASS")
}

func backupDatabase(source, destination string) (err error) {
	source, err = validateSource(source)
	if err != nil {
		return err
	}
	destination, err = validateDestination(destination)
	if err != nil {
		return err
	}

	return backupValidatedDatabase(source, destination)
}

func backupValidatedDatabase(source, destination string) (err error) {
	// Reserve the empty output atomically; SQLite permits VACUUM INTO it.
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	if err = output.Close(); err != nil {
		return err
	}

	readOnly, err := sql.Open("sqlite", readonlySQLiteDSN(source))
	if err != nil {
		return err
	}
	readOnly.SetMaxOpenConns(1)
	if err = checkIntegrity(readOnly); err != nil {
		_ = readOnly.Close()
		return err
	}
	if err = readOnly.Close(); err != nil {
		return err
	}
	// SQLite requires a writable connection for VACUUM INTO even though the
	// source database itself is not modified. Integrity was checked read-only
	// immediately before opening this isolated connection.
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	literal := strings.ReplaceAll(destination, "'", "''")
	if _, err = db.Exec("VACUUM INTO '" + literal + "'"); err != nil {
		_ = db.Close()
		return fmt.Errorf("vacuum into backup: %w", err)
	}
	if err = db.Close(); err != nil {
		return err
	}
	if err = os.Chmod(destination, 0o600); err != nil {
		return err
	}
	check, openErr := sql.Open("sqlite", destination)
	if openErr != nil {
		return openErr
	}
	check.SetMaxOpenConns(1)
	checkErr := checkIntegrity(check)
	closeErr := check.Close()
	if checkErr != nil || closeErr != nil {
		return errors.Join(checkErr, closeErr)
	}
	return nil
}

func readonlySQLiteDSN(path string) string {
	return "file:" + path + "?mode=ro"
}

func validateSource(path string) (string, error) {
	path, err := absolutePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("source database must be a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("source database must not be group/world writable")
	}
	return path, nil
}

func validateDestination(path string) (string, error) {
	path, err := absolutePath(path)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(path); err == nil {
		return "", errors.New("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("backup destination parent must be a real directory")
	}
	if runtime.GOOS != "windows" && parent.Mode().Perm()&0o077 != 0 {
		return "", errors.New("backup destination parent must be private")
	}
	return path, nil
}

func absolutePath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute")
	}
	return filepath.Clean(path), nil
}

func checkIntegrity(db *sql.DB) error {
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("sqlite integrity_check: %s", result)
	}
	return nil
}

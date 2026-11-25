package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessStartupFailureLeavesNoOtherListener(t *testing.T) {
	t.Run("LAN bind fails before loopback is served", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		localAddr := freeAddress(t)
		output := runServiceExpectFailure(t, localAddr, occupied.Addr().String())
		if !strings.Contains(output, "listen tcp") {
			t.Fatalf("expected bind error, output=%s", output)
		}
		assertUnavailable(t, localAddr)
	})
	t.Run("loopback bind fails before LAN is served", func(t *testing.T) {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		lanAddr := freeAddress(t)
		output := runServiceExpectFailure(t, occupied.Addr().String(), lanAddr)
		if !strings.Contains(output, "listen tcp") {
			t.Fatalf("expected bind error, output=%s", output)
		}
		assertUnavailable(t, lanAddr)
	})
}

func runServiceExpectFailure(t *testing.T, localAddr, lanAddr string) string {
	return runServiceExpectFailureWithDB(t, filepath.Join(t.TempDir(), "start.db"), localAddr, lanAddr)
}

func runServiceExpectFailureWithDB(t *testing.T, dbPath, localAddr, lanAddr string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Env = serviceEnv(dbPath, localAddr, lanAddr)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("service did not exit on startup failure: %s", out)
	}
	if err == nil {
		t.Fatalf("service unexpectedly started: %s", out)
	}
	return string(out)
}

func freeAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err = ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func assertUnavailable(t *testing.T, addr string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatalf("unexpected listener remains open at %s", addr)
	}
}

func serviceEnv(db, local, lan string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "REPOTEST_DB=") && !strings.HasPrefix(entry, "REPOTEST_ADDR=") && !strings.HasPrefix(entry, "REPOTEST_LAN_ADDR=") {
			env = append(env, entry)
		}
	}
	return append(env, "REPOTEST_DB="+db, "REPOTEST_ADDR="+local, "REPOTEST_LAN_ADDR="+lan)
}

func TestProcessMigrationFailureStartsNoListener(t *testing.T) {
	localAddr := freeAddress(t)
	lanAddr := freeAddress(t)
	dbPath := filepath.Join(t.TempDir(), "missing", "repo.db")
	output := runServiceExpectFailureWithDB(t, dbPath, localAddr, lanAddr)
	if strings.Contains(output, "pairing code") {
		t.Fatalf("pairing code logged before storage initialization: %s", output)
	}
	assertUnavailable(t, localAddr)
	assertUnavailable(t, lanAddr)
}

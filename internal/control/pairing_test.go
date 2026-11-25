package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPairingCodeIsSingleUseAndSessionRevocable(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	pair, code, err := NewPairing(store, now)
	if err != nil {
		t.Fatal(err)
	}
	token, err := pair.Consume(code, now)
	if err != nil {
		t.Fatal(err)
	}
	if !pair.Valid(token, now) {
		t.Fatal("new token invalid")
	}
	if _, err = pair.Consume(code, now); err == nil {
		t.Fatal("replayed code accepted")
	}
	if err = pair.Revoke(token); err != nil {
		t.Fatal(err)
	}
	if pair.Valid(token, now) {
		t.Fatal("revoked token valid")
	}
	_ = context.Background()
}
func TestPairingLocksAfterFiveAttempts(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	pair, code, err := NewPairing(store, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, _ = pair.Consume("wrong", now)
	}
	if _, err = pair.Consume(code, now); err == nil {
		t.Fatal("accepted code after attempt limit")
	}
}
